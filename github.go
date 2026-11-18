package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// ErrBadUser means the username failed local validation.
var ErrBadUser = errors.New("that is not a valid GitHub username")

// ErrNotFound means GitHub answered 404 for the user.
var ErrNotFound = errors.New("no GitHub user with that name")

// ErrNetwork means the request to GitHub never completed.
var ErrNetwork = errors.New("could not reach GitHub")

// RateLimitError means GitHub answered 403 with the rate-limit budget spent.
type RateLimitError struct {
	Reset time.Time
}

func (e *RateLimitError) Error() string {
	return "GitHub rate limit reached"
}

// Client talks to the public GitHub API and website.
type Client struct {
	base   *url.URL // api.github.com
	web    *url.URL // github.com (public contribution graph)
	http   *http.Client
	token  string
	events chan<- struct{}
}

// NewGitHubClient builds a client; an empty token uses the unauthenticated
// budget of 60 requests per hour per IP.
func NewGitHubClient(token string) *Client {
	base, _ := url.Parse("https://api.github.com/")
	web, _ := url.Parse("https://github.com/")
	return &Client{
		base:  base,
		web:   web,
		http:  &http.Client{Timeout: 15 * time.Second},
		token: token,
	}
}

// User is the profile slice the feed needs.
type User struct {
	Login     string `json:"login"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
	HTMLURL   string `json:"html_url"`
}

// Event is one entry of /users/{user}/events/public.
type Event struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	CreatedAt string `json:"created_at"`
	Repo      struct {
		Name string `json:"name"`
	} `json:"repo"`
	Payload struct {
		Size    int    `json:"size"`
		Ref     string `json:"ref"`
		RefType string `json:"ref_type"`
		Action  string `json:"action"`
		Commits []struct {
			Sha string `json:"sha"`
		} `json:"commits"`
		PullRequest struct {
			Title   string `json:"title"`
			HTMLURL string `json:"html_url"`
		} `json:"pull_request"`
		Issue struct {
			Title   string `json:"title"`
			HTMLURL string `json:"html_url"`
		} `json:"issue"`
		Release struct {
			TagName string `json:"tag_name"`
		} `json:"release"`
	} `json:"payload"`
}

func validUsername(u string) bool {
	if len(u) == 0 || len(u) > 39 {
		return false
	}
	if u[0] == '-' || u[len(u)-1] == '-' {
		return false
	}
	for i := 0; i < len(u); i++ {
		c := u[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-':
		default:
			return false
		}
	}
	return true
}

// FetchProfile returns the profile, or one of ErrNotFound / *RateLimitError /
// ErrNetwork / ErrBadUser.
func (c *Client) FetchProfile(ctx context.Context, username string) (*User, error) {
	if !validUsername(username) {
		return nil, ErrBadUser
	}
	return c.getUser(ctx, username)
}

// FetchEventsPage returns one page (30 items) of the public events feed.
// GitHub keeps at most 300 events (10 pages) and only for 90 days.
func (c *Client) FetchEventsPage(ctx context.Context, username string, page int) ([]Event, error) {
	resp, err := c.get(ctx, fmt.Sprintf("/users/%s/events/public?per_page=30&page=%d", url.PathEscape(username), page))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return nil, ErrNotFound
	case http.StatusForbidden, http.StatusTooManyRequests:
		return nil, c.rateError(resp)
	case http.StatusUnprocessableEntity:
		return nil, ErrBadUser
	default:
		return nil, fmt.Errorf("%w: GitHub answered %d", ErrNetwork, resp.StatusCode)
	}

	var events []Event
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&events); err != nil {
		return nil, fmt.Errorf("%w: unreadable events", ErrNetwork)
	}
	return events, nil
}

func (c *Client) get(ctx context.Context, path string) (*http.Response, error) {
	ref, err := url.Parse(path)
	if err != nil {
		return nil, ErrNetwork
	}
	u := c.base.ResolveReference(ref).String()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, ErrNetwork
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "github-activity-tracker")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, ErrNetwork
	}
	return resp, nil
}

func (c *Client) getUser(ctx context.Context, username string) (*User, error) {
	resp, err := c.get(ctx, "/users/"+url.PathEscape(username))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return nil, ErrNotFound
	case http.StatusForbidden, http.StatusTooManyRequests:
		return nil, c.rateError(resp)
	case http.StatusUnprocessableEntity:
		return nil, ErrBadUser
	default:
		return nil, fmt.Errorf("%w: GitHub answered %d", ErrNetwork, resp.StatusCode)
	}

	var user User
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&user); err != nil {
		return nil, fmt.Errorf("%w: unreadable profile", ErrNetwork)
	}
	return &user, nil
}

func (c *Client) getEvents(ctx context.Context, username string) ([]Event, error) {
	resp, err := c.get(ctx, "/users/"+url.PathEscape(username)+"/events/public?per_page=30")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return nil, ErrNotFound
	case http.StatusForbidden, http.StatusTooManyRequests:
		return nil, c.rateError(resp)
	case http.StatusUnprocessableEntity:
		return nil, ErrBadUser
	default:
		return nil, fmt.Errorf("%w: GitHub answered %d", ErrNetwork, resp.StatusCode)
	}

	var events []Event
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&events); err != nil {
		return nil, fmt.Errorf("%w: unreadable events", ErrNetwork)
	}
	return events, nil
}

func (c *Client) rateError(resp *http.Response) error {
	e := &RateLimitError{Reset: time.Now().Add(time.Hour)}
	if v := resp.Header.Get("X-RateLimit-Reset"); v != "" {
		if sec, err := strconv.ParseInt(v, 10, 64); err == nil {
			e.Reset = time.Unix(sec, 0)
		}
	}
	return e
}
