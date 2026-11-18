package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Handler serves the page and the JSON endpoint.
type Handler struct {
	gh       *Client
	cache    *Cache
	calCache *CalendarCache
}

type apiError struct {
	Error string `json:"error"`
	Reset string `json:"reset,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("write json: %v", err)
	}
}

// index renders the shell or full board if username is provided.
func (h *Handler) index(w http.ResponseWriter, r *http.Request) {
	username := strings.TrimSpace(r.URL.Query().Get("username"))
	if username == "" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := Page("", EmptyBoard()).Render(r.Context(), w); err != nil {
			log.Printf("render index: %v", err)
		}
		return
	}
	h.activity(w, r)
}

type apiUser struct {
	Login     string `json:"login"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
	URL       string `json:"url"`
}

type apiResponse struct {
	User      apiUser `json:"user"`
	FetchedAt string  `json:"fetched_at"`
	Page      int     `json:"page"`
	HasMore   bool    `json:"has_more"`
	Events    []Item  `json:"events"`
	Stats     Stats   `json:"stats"`
}

// activity answers requests for user events, rendering either Templ components
// (for HTMX and browser direct loads) or JSON (for API clients).
func (h *Handler) activity(w http.ResponseWriter, r *http.Request) {
	username := strings.TrimSpace(r.URL.Query().Get("username"))
	filter := strings.TrimSpace(r.URL.Query().Get("filter"))
	isHX := r.Header.Get("HX-Request") == "true"
	isJSON := (strings.Contains(r.Header.Get("Accept"), "application/json") || strings.HasPrefix(r.URL.Path, "/api/")) && !isHX

	if !validUsername(username) {
		h.handleErr(w, r, ErrBadUser, username)
		return
	}

	page := 1
	if p := r.URL.Query().Get("page"); p != "" {
		if n, err := strconv.Atoi(p); err == nil && n >= 1 {
			page = n
		}
	}
	if page > 10 {
		page = 10
	}

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	cal, calNote := h.contributions(ctx, username)

	user, events, pages, done, cached := h.cache.Get(username)
	if !cached {
		u, err := h.gh.FetchProfile(ctx, username)
		if err != nil {
			h.handleErr(w, r, err, username)
			return
		}
		h.cache.PutProfile(username, u)
		user, events, pages, done = u, nil, 0, false
	}

	// Grow the cache one page at a time up to what was asked for.
	for p := pages + 1; p <= page && !done; p++ {
		evs, err := h.gh.FetchEventsPage(ctx, username, p)
		if err != nil {
			h.handleErr(w, r, err, username)
			return
		}
		h.cache.Append(username, evs, p)
		events = append(events, evs...)
		pages = p
		if len(evs) < pageSize {
			done = true
		}
	}

	items, stats := BuildFeed(events)
	hasMore := !done && pages < 10 && len(events) > 0

	if isJSON {
		writeJSON(w, http.StatusOK, apiResponse{
			User: apiUser{
				Login:     user.Login,
				Name:      user.Name,
				AvatarURL: user.AvatarURL,
				URL:       user.HTMLURL,
			},
			FetchedAt: time.Now().Format(time.RFC3339),
			Page:      pages,
			HasMore:   hasMore,
			Events:    items,
			Stats:     stats,
		})
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	boardComp := Board(user, cal, calNote, stats, items, pages, hasMore, filter)
	if isHX {
		if err := boardComp.Render(r.Context(), w); err != nil {
			log.Printf("render board fragment: %v", err)
		}
	} else {
		if err := Page(username, boardComp).Render(r.Context(), w); err != nil {
			log.Printf("render page: %v", err)
		}
	}
}

// contributions loads the calendar from cache or GitHub. A failed lookup only
// costs the chart: the activity feed still renders.
func (h *Handler) contributions(ctx context.Context, username string) (*ContributionCalendar, string) {
	if cal, ok := h.calCache.Get(username); ok {
		return cal, ""
	}
	cal, err := h.gh.FetchContributions(ctx, username)
	if err != nil {
		log.Printf("contribution calendar for %s: %v", username, err)
		return nil, "The contribution graph is unavailable right now. The activity feed below is unchanged."
	}
	if cal == nil {
		return nil, "No public contributions to graph for this user."
	}
	h.calCache.Put(username, cal)
	return cal, ""
}

func (h *Handler) handleErr(w http.ResponseWriter, r *http.Request, err error, username string) {
	isHX := r.Header.Get("HX-Request") == "true"
	isJSON := (strings.Contains(r.Header.Get("Accept"), "application/json") || strings.HasPrefix(r.URL.Path, "/api/")) && !isHX

	var errMsg string
	var status int

	switch {
	case errors.Is(err, ErrBadUser):
		errMsg = "That is not a valid GitHub username. Letters, numbers and hyphens only, up to 39 characters."
		status = http.StatusBadRequest
	case errors.Is(err, ErrNotFound):
		errMsg = "No GitHub user is named " + username + ". Check the spelling and try again."
		status = http.StatusNotFound
	case errors.Is(err, ErrNetwork):
		errMsg = "Could not reach GitHub. Check the connection and try again."
		status = http.StatusBadGateway
	default:
		var rl *RateLimitError
		if errors.As(err, &rl) {
			errMsg = "GitHub's request budget is spent. It fills again after " + rl.Reset.Local().Format("3:04PM") + "."
			status = http.StatusTooManyRequests
			if isJSON {
				writeJSON(w, status, apiError{Error: errMsg, Reset: rl.Reset.Format(time.RFC3339)})
				return
			}
		} else {
			errMsg = "GitHub answered something unexpected. Try again in a minute."
			status = http.StatusBadGateway
		}
	}

	if isJSON {
		writeJSON(w, status, apiError{Error: errMsg})
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if isHX {
		if err := ErrorBoard(errMsg).Render(r.Context(), w); err != nil {
			log.Printf("render error fragment: %v", err)
		}
	} else {
		w.WriteHeader(status)
		if err := Page(username, ErrorBoard(errMsg)).Render(r.Context(), w); err != nil {
			log.Printf("render error page: %v", err)
		}
	}
}
