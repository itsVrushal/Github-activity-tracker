package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Handler serves the page and the JSON endpoint.
type Handler struct {
	gh    *Client
	cache *Cache
}

var tmpl = template.Must(template.ParseFS(templateFiles, "index.html"))

//go:embed index.html
var templateFiles embed.FS

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

// index renders the shell; the feed itself is fetched client side.
func (h *Handler) index(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "index.html", nil); err != nil {
		log.Printf("template: %v", err)
		http.Error(w, "template error", http.StatusInternalServerError)
	}
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

// activity answers GET /api/activity?username=<name>&page=<n>.
// Page 1 is the newest 30 events; each further page reaches one page deeper
// into GitHub's memory (it holds at most 10 pages, 90 days).
func (h *Handler) activity(w http.ResponseWriter, r *http.Request) {
	username := strings.TrimSpace(r.URL.Query().Get("username"))
	if !validUsername(username) {
		writeJSON(w, http.StatusBadRequest, apiError{
			Error: "That is not a valid GitHub username. Letters, numbers and hyphens only, up to 39 characters.",
		})
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

	user, events, pages, done, cached := h.cache.Get(username)
	if !cached {
		u, err := h.gh.FetchProfile(ctx, username)
		if err != nil {
			h.apiErr(w, err, username)
			return
		}
		h.cache.PutProfile(username, u)
		user, events, pages, done = u, nil, 0, false
	}

	// Grow the cache one page at a time up to what was asked for.
	for p := pages + 1; p <= page && !done; p++ {
		evs, err := h.gh.FetchEventsPage(ctx, username, p)
		if err != nil {
			h.apiErr(w, err, username)
			return
		}
		h.cache.Append(username, evs, p)
		events = append(events, evs...)
		pages = p
		if len(evs) < pageSize {
			done = true
		}
	}

	h.respond(w, user, events, pages, done)
}

func (h *Handler) apiErr(w http.ResponseWriter, err error, username string) {
	switch {
	case errors.Is(err, ErrBadUser):
		writeJSON(w, http.StatusBadRequest, apiError{Error: err.Error() + " Letters, numbers and hyphens only, up to 39 characters."})
	case errors.Is(err, ErrNotFound):
		writeJSON(w, http.StatusNotFound, apiError{
			Error: "No GitHub user is named " + username + ". Check the spelling and try again.",
		})
	case errors.Is(err, ErrNetwork):
		log.Printf("network: %v", err)
		writeJSON(w, http.StatusBadGateway, apiError{
			Error: "Could not reach GitHub. Check the connection and try again.",
		})
	default:
		var rl *RateLimitError
		if errors.As(err, &rl) {
			writeJSON(w, http.StatusTooManyRequests, apiError{
				Error: "GitHub's request budget is spent. It fills again after " + rl.Reset.Local().Format("3:04PM") + ".",
				Reset: rl.Reset.Format(time.RFC3339),
			})
			return
		}
		log.Printf("github: %v", err)
		writeJSON(w, http.StatusBadGateway, apiError{
			Error: "GitHub answered something unexpected. Try again in a minute.",
		})
	}
}

func (h *Handler) respond(w http.ResponseWriter, user *User, events []Event, pages int, done bool) {
	items, stats := BuildFeed(events)
	writeJSON(w, http.StatusOK, apiResponse{
		User: apiUser{
			Login:     user.Login,
			Name:      user.Name,
			AvatarURL: user.AvatarURL,
			URL:       user.HTMLURL,
		},
		FetchedAt: time.Now().Format(time.RFC3339),
		Page:      pages,
		HasMore:   !done && pages < 10 && len(events) > 0,
		Events:    items,
		Stats:     stats,
	})
}
