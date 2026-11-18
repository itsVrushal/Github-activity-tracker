package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ContributionCell is one square of the rendered grid. Blank cells pad the
// first and last weeks so the seven rows line up with Sunday through Saturday.
type ContributionCell struct {
	Blank bool
	Level int
	Title string
	date  time.Time
}

// Class returns the CSS classes for a filled square.
func (c ContributionCell) Class() string {
	return fmt.Sprintf("cal__cell cal__cell--l%d", c.Level)
}

// ContributionCalendar is the year of contribution squares, ready to render.
type ContributionCalendar struct {
	Total  int
	Cells  []ContributionCell
	Months []string // one label per column, empty when the month does not change
}

// day is one square as GitHub publishes it, with its level already computed.
type day struct {
	date  time.Time
	level int
}

// FetchContributions loads the public contribution graph from GitHub's own
// contributions endpoint — the same one a public profile shows — so it works
// for any entered user, with or without a token.
func (c *Client) FetchContributions(ctx context.Context, username string) (*ContributionCalendar, error) {
	if !validUsername(username) {
		return nil, ErrBadUser
	}

	u := c.web.ResolveReference(&url.URL{Path: "/users/" + url.PathEscape(username) + "/contributions"})
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("%w: bad request", ErrNetwork)
	}
	req.Header.Set("Accept", "text/html, */*; q=0.1")
	req.Header.Set("User-Agent", "github-activity-tracker")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, ErrNetwork
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return nil, ErrNotFound
	case http.StatusForbidden, http.StatusTooManyRequests:
		return nil, c.rateError(resp)
	default:
		return nil, fmt.Errorf("%w: GitHub answered %d", ErrNetwork, resp.StatusCode)
	}

	days, total, err := readContributions(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	return buildCalendar(days, total), nil
}

// dayCell matches one table cell of GitHub's contribution graph:
//
//	<td ... data-date="2026-01-05" ... data-level="3" ... class="ContributionCalendar-day">
//
// The attribute order is stable in GitHub's markup, so a single regexp is
// enough without pulling in an HTML parser.
var dayCell = regexp.MustCompile(`<td[^>]*\bdata-date="(\d{4}-\d{2}-\d{2})"[^>]*\bdata-level="([0-4])"[^>]*>`)

// totalRe matches the total figure in the graph's heading:
//
//	<h2 ...>2,837 contributions in the last year</h2>
var totalRe = regexp.MustCompile(`<h2[^>]*>\s*([\d,]+)`)

// readContributions decodes GitHub's contributions HTML table and returns one
// day per cell, in document order (chronological), plus the year total shown
// beside the graph.
func readContributions(r io.Reader) ([]day, int, error) {
	body, err := io.ReadAll(r)
	if err != nil {
		return nil, 0, fmt.Errorf("%w: unreadable contributions graph", ErrNetwork)
	}
	total := 0
	if m := totalRe.FindStringSubmatch(string(body)); m != nil {
		total, _ = strconv.Atoi(strings.ReplaceAll(m[1], ",", ""))
	}
	matches := dayCell.FindAllStringSubmatch(string(body), -1)
	days := make([]day, 0, len(matches))
	for _, m := range matches {
		date, err := time.Parse("2006-01-02", m[1])
		if err != nil {
			continue
		}
		level, _ := strconv.Atoi(m[2])
		days = append(days, day{date: date, level: level})
	}
	return days, total, nil
}

// buildCalendar lays days out into seven-row columns beginning on Sunday,
// matching GitHub's graph, and works out where each month label belongs.
//
// GitHub's markup lists the days column by column (every Sunday, then every
// Monday, ...), so the days are sorted by date before being laid out row by
// row into the grid.
func buildCalendar(days []day, total int) *ContributionCalendar {
	if len(days) == 0 {
		return nil
	}
	sort.Slice(days, func(i, j int) bool { return days[i].date.Before(days[j].date) })

	first := days[0].date

	// GitHub starts weeks on Sunday; the grid's first column is Sunday too.
	offset := int(first.Weekday())
	cells := make([]ContributionCell, 0, len(days)+offset+7)
	for i := 0; i < offset; i++ {
		cells = append(cells, ContributionCell{Blank: true})
	}

	for _, d := range days {
		cells = append(cells, ContributionCell{
			Level: d.level,
			Title: contributionPhrase(d.level) + " on " + d.date.Format("2 Jan 2006"),
			date:  d.date,
		})
	}
	for len(cells)%7 != 0 {
		cells = append(cells, ContributionCell{Blank: true})
	}

	// Label the column where each month first appears, matching GitHub.
	months := make([]string, len(cells)/7)
	lastMonth := time.Month(0)
	for i, cell := range cells {
		if cell.Blank || cell.date.Month() == lastMonth {
			continue
		}
		months[i/7] = cell.date.Format("Jan")
		lastMonth = cell.date.Month()
	}

	return &ContributionCalendar{Total: total, Cells: cells, Months: months}
}

// contributionPhrase describes one square's level for the tooltip.
func contributionPhrase(level int) string {
	switch level {
	case 0:
		return "No contributions"
	case 1:
		return "1 contribution"
	default:
		return fmt.Sprintf("%d contributions", level)
	}
}

// formatNumber renders an integer with thousands separators.
func formatNumber(n int) string {
	s := strconv.Itoa(n)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	return b.String()
}
