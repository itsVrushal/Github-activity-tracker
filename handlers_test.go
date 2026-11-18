package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestActivityIncludesCalendar(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/users/octocat/contributions":
			_, _ = w.Write([]byte(`<h2 class="f4 text-normal mb-2">7 contributions in the last year</h2>
				<table>
					<tr><td data-date="2026-01-05" data-level="4" class="ContributionCalendar-day"></td></tr>
					<tr><td data-date="2026-01-06" data-level="0" class="ContributionCalendar-day"></td></tr>
				</table>`))
		case "/users/octocat":
			_, _ = w.Write([]byte(`{"login":"octocat","name":"The Octocat","avatar_url":"","html_url":"https://github.com/octocat"}`))
		default:
			if strings.HasSuffix(r.URL.Path, "/events/public") {
				_, _ = w.Write([]byte(`[]`))
				return
			}
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewGitHubClient("")
	client.base, _ = url.Parse(server.URL + "/")
	client.web, _ = url.Parse(server.URL + "/")
	h := &Handler{gh: client, cache: NewCache(time.Minute), calCache: NewCalendarCache(time.Hour)}

	req := httptest.NewRequest(http.MethodGet, "/activity?username=octocat", nil)
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()
	h.activity(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `class="cal"`) || !strings.Contains(body, `cal__cell--l4`) {
		t.Fatalf("fragment is missing the contribution grid: %s", body)
	}

	// The second request comes from the calendar cache and makes no GitHub call.
	rec = httptest.NewRecorder()
	h.activity(rec, req)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte(`class="cal"`)) {
		t.Fatalf("cached request failed: %d %s", rec.Code, rec.Body.String())
	}
}
