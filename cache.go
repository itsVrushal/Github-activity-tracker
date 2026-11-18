package main

import (
	"sync"
	"time"
)

// pageSize matches GitHub's events page size.
const pageSize = 30

// entry is one cached feed, grown page by page.
type entry struct {
	user    *User
	events  []Event
	pages   int  // highest page fetched so far
	done    bool // true once a page came back short: the history is exhausted
	expires time.Time
}

// Cache keeps feed lookups for a short TTL so one name browsed a few times
// does not burn the GitHub budget.
type Cache struct {
	mu   sync.Mutex
	ttl  time.Duration
	data map[string]entry
}

// NewCache builds a cache with the given time to live.
func NewCache(ttl time.Duration) *Cache {
	return &Cache{ttl: ttl, data: map[string]entry{}}
}

// Get returns the cached user, the events fetched so far, and how deep the
// cache reaches.
func (c *Cache) Get(username string) (*User, []Event, int, bool, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.data[username]
	if !ok || time.Now().After(e.expires) {
		return nil, nil, 0, false, false
	}
	return e.user, e.events, e.pages, e.done, true
}

// PutProfile stores a fresh profile lookup.
func (c *Cache) PutProfile(username string, user *User) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e := c.data[username]
	e.user = user
	e.expires = time.Now().Add(c.ttl)
	c.data[username] = e
}

// Append adds one fetched page. A page shorter than the page size marks the
// history as exhausted.
func (c *Cache) Append(username string, events []Event, page int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e := c.data[username]
	if page <= e.pages || len(e.events)+len(events) > 10*pageSize {
		return
	}
	e.events = append(e.events, events...)
	e.pages = page
	if len(events) < pageSize {
		e.done = true
	}
	e.expires = time.Now().Add(c.ttl)
	c.data[username] = e
}

// CalendarCache keeps the year-long contribution graph longer than the feed:
// one GraphQL call per user per hour is plenty for a square that changes once
// a day.
type CalendarCache struct {
	mu   sync.Mutex
	ttl  time.Duration
	data map[string]calendarEntry
}

type calendarEntry struct {
	calendar *ContributionCalendar
	expires  time.Time
}

// NewCalendarCache builds a cache with the given time to live.
func NewCalendarCache(ttl time.Duration) *CalendarCache {
	return &CalendarCache{ttl: ttl, data: map[string]calendarEntry{}}
}

// Get returns the cached calendar, if it is still fresh.
func (c *CalendarCache) Get(username string) (*ContributionCalendar, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.data[username]
	if !ok || time.Now().After(e.expires) {
		return nil, false
	}
	return e.calendar, true
}

// Put stores a freshly fetched calendar.
func (c *CalendarCache) Put(username string, cal *ContributionCalendar) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[username] = calendarEntry{calendar: cal, expires: time.Now().Add(c.ttl)}
}
