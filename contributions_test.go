package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestBuildCalendarMondayStart(t *testing.T) {
	// 5 Jan 2026 is a Monday: one Sunday blank pads the first column.
	days := []day{
		{date: time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC), level: 1},
		{date: time.Date(2026, 1, 6, 0, 0, 0, 0, time.UTC), level: 2},
	}
	cal := buildCalendar(days, 3)
	if !cal.Cells[0].Blank {
		t.Fatal("first cell should pad Sunday")
	}
	if got := cal.Cells[1].date.Format("2 Jan 2006"); got != "5 Jan 2026" {
		t.Fatalf("first real cell = %q", got)
	}
	if cal.Total != 3 {
		t.Fatalf("total = %d, want 3", cal.Total)
	}
}

func TestBuildCalendarLevels(t *testing.T) {
	// Input is column-major (GitHub's DOM order: all Sundays first), so the
	// calendar must sort it back into chronological row order.
	days := []day{
		{date: time.Date(2026, 1, 8, 0, 0, 0, 0, time.UTC), level: 1},
		{date: time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC), level: 4},
		{date: time.Date(2026, 1, 7, 0, 0, 0, 0, time.UTC), level: 2},
		{date: time.Date(2026, 1, 6, 0, 0, 0, 0, time.UTC), level: 3},
		{date: time.Date(2026, 1, 9, 0, 0, 0, 0, time.UTC), level: 0},
	}
	cal := buildCalendar(days, 10)
	if cal == nil {
		t.Fatal("buildCalendar returned nil")
	}
	if cal.Total != 10 {
		t.Fatalf("total = %d, want 10", cal.Total)
	}
	if !cal.Cells[0].Blank {
		t.Fatal("first cell should pad Sunday")
	}
	// 5 Jan 2026 is a Monday: one Sunday blank, then the sorted week.
	gotDates := []string{cal.Cells[1].date.Format("2 Jan"), cal.Cells[2].date.Format("2 Jan"), cal.Cells[3].date.Format("2 Jan"), cal.Cells[4].date.Format("2 Jan"), cal.Cells[5].date.Format("2 Jan")}
	wantDates := []string{"5 Jan", "6 Jan", "7 Jan", "8 Jan", "9 Jan"}
	for i, w := range wantDates {
		if got := gotDates[i]; got != w {
			t.Fatalf("cell %d date = %q, want %q", i+1, got, w)
		}
	}
	want := []int{4, 3, 2, 1, 0}
	for i, w := range want {
		if got := cal.Cells[i+1].Level; got != w {
			t.Fatalf("cell %d level = %d, want %d", i+1, got, w)
		}
	}
	if got := cal.Cells[1].Title; got != "4 contributions on 5 Jan 2026" {
		t.Fatalf("title = %q", got)
	}
	if len(cal.Cells)%7 != 0 {
		t.Fatalf("%d cells do not fill whole weeks", len(cal.Cells))
	}
	if got := len(cal.Months); got != len(cal.Cells)/7 {
		t.Fatalf("%d month labels for %d columns", got, len(cal.Cells)/7)
	}
	if cal.Months[0] != "Jan" {
		t.Fatalf("first month = %q", cal.Months[0])
	}
}

func TestBuildCalendarMonthLabelsSparse(t *testing.T) {
	// A contiguous week range crossing the month boundary: Jan 25 (Sun) to
	// Feb 1 (Sun). Only the column where each month starts carries a label.
	days := []day{}
	for d := time.Date(2026, 1, 25, 0, 0, 0, 0, time.UTC); !d.After(time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)); d = d.AddDate(0, 0, 1) {
		days = append(days, day{date: d, level: 1})
	}
	cal := buildCalendar(days, 8)
	if cal == nil {
		t.Fatal("buildCalendar returned nil")
	}
	labels := 0
	for _, m := range cal.Months {
		if m != "" {
			labels++
		}
	}
	if labels != 2 {
		t.Fatalf("%d month labels, want 2", labels)
	}
	if cal.Months[0] != "Jan" || cal.Months[1] != "Feb" {
		t.Fatalf("labels = %v, want [Jan Feb]", cal.Months[:2])
	}
}

func TestBuildCalendarEmpty(t *testing.T) {
	if cal := buildCalendar(nil, 0); cal != nil {
		t.Fatal("expected nil for no days")
	}
}

func TestFetchContributions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/users/octocat/contributions" {
			t.Errorf("request = %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`<h2 class="f4 text-normal mb-2">3 contributions in the last year</h2>
			<table>
				<tr><td data-date="2026-01-05" data-level="3" class="ContributionCalendar-day"></td></tr>
				<tr><td data-date="2026-01-06" data-level="0" class="ContributionCalendar-day"></td></tr>
				<tr><td data-date="2026-01-07" data-level="1" class="ContributionCalendar-day"></td></tr>
			</table>`))
	}))
	defer server.Close()

	client := NewGitHubClient("")
	client.base, _ = url.Parse(server.URL + "/")
	client.web, _ = url.Parse(server.URL + "/")
	cal, err := client.FetchContributions(context.Background(), "octocat")
	if err != nil {
		t.Fatalf("FetchContributions: %v", err)
	}
	if cal == nil || cal.Total != 3 {
		t.Fatalf("calendar = %+v", cal)
	}
	if len(cal.Cells) != 7 {
		t.Fatalf("expected 7 cells, got %d", len(cal.Cells))
	}
	if got := cal.Cells[1].Level; got != 3 {
		t.Fatalf("level = %d", got)
	}
	if got := cal.Cells[2].Level; got != 0 {
		t.Fatalf("zero level = %d", got)
	}
	if got := cal.Cells[3].Level; got != 1 {
		t.Fatalf("level = %d", got)
	}
}

func TestBoardRendersCalendar(t *testing.T) {
	cal := &ContributionCalendar{
		Total: 1234,
		Cells: []ContributionCell{
			{Blank: true},
			{Level: 3, Title: "3 contributions on 5 Jan 2026", date: time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)},
		},
		Months: []string{"Jan"},
	}
	user := &User{Login: "octocat", Name: "The Octocat"}
	var out bytes.Buffer
	if err := Board(user, cal, "", Stats{}, nil, 1, false, "").Render(context.Background(), &out); err != nil {
		t.Fatalf("render board: %v", err)
	}
	html := out.String()
	for _, want := range []string{`class="cal"`, `cal__grid`, `cal__cell--l3`, `1,234`, `3 contributions on 5 Jan 2026`} {
		if !strings.Contains(html, want) {
			t.Errorf("rendered board is missing %q", want)
		}
	}
}
