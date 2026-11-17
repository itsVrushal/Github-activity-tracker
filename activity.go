package main

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Categories group events behind the filter chips.
const (
	CatPush    = "push"
	CatStar    = "star"
	CatPull    = "pull"
	CatIssue   = "issue"
	CatRelease = "release"
	CatComment = "comment"
	CatRepo    = "repo"
	CatOther   = "other"
)

// Item is one row of the feed.
type Item struct {
	Time     string `json:"time"`
	Date     string `json:"date"`
	Category string `json:"category"`
	Sentence string `json:"sentence"`
	Repo     string `json:"repo"`
	URL      string `json:"url"`
}

// Stats are the score-line figures, computed from the same response.
type Stats struct {
	Events  int `json:"events"`
	Pushes  int `json:"pushes"`
	Commits int `json:"commits"`
	Stars   int `json:"stars"`
	Pulls   int `json:"pulls"`
	Issues  int `json:"issues"`
	Repos   int `json:"repos"`
}

func repoShort(full string) string {
	_, short, ok := strings.Cut(full, "/")
	if !ok {
		return full
	}
	return short
}

// BuildFeed maps raw events to rows and computes the score line.
func BuildFeed(events []Event) ([]Item, Stats) {
	items := make([]Item, 0, len(events))
	repos := map[string]bool{}
	stats := Stats{Events: len(events)}

	// GitHub's pages can interleave slightly; the feed reads in one order.
	sorted := make([]Event, len(events))
	copy(sorted, events)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].CreatedAt > sorted[j].CreatedAt
	})

	for _, e := range sorted {
		repo := e.Repo.Name
		short := repoShort(repo)
		repos[repo] = true
		link := "https://github.com/" + repo

		var cat, sentence string
		switch e.Type {
		case "PushEvent":
			n := e.Payload.Size
			if n == 0 {
				n = len(e.Payload.Commits)
			}
			cat = CatPush
			stats.Pushes++
			stats.Commits += n
			// Some pushes (rebases, large pushes) carry no commit list;
			// say "pushed to" rather than a zero.
			switch {
			case n == 0:
				sentence = "pushed to"
			case n == 1:
				sentence = "pushed 1 commit to"
			default:
				sentence = fmt.Sprintf("pushed %d commits to", n)
			}
		case "WatchEvent":
			cat = CatStar
			stats.Stars++
			sentence = "starred"
		case "PullRequestEvent":
			cat = CatPull
			stats.Pulls++
			switch e.Payload.Action {
			case "closed":
				sentence = "closed a pull request in"
			case "merged":
				sentence = "merged a pull request in"
			default:
				sentence = "opened a pull request in"
			}
			if e.Payload.PullRequest.Title != "" {
				link = e.Payload.PullRequest.HTMLURL
			}
		case "IssuesEvent":
			cat = CatIssue
			stats.Issues++
			switch e.Payload.Action {
			case "closed":
				sentence = "closed an issue in"
			case "reopened":
				sentence = "reopened an issue in"
			default:
				sentence = "opened an issue in"
			}
			if e.Payload.Issue.Title != "" {
				link = e.Payload.Issue.HTMLURL
			}
		case "IssueCommentEvent":
			cat = CatComment
			sentence = "commented on an issue in"
			if e.Payload.Issue.HTMLURL != "" {
				link = e.Payload.Issue.HTMLURL
			}
		case "PullRequestReviewEvent":
			cat = CatComment
			sentence = "reviewed a pull request in"
			if e.Payload.PullRequest.HTMLURL != "" {
				link = e.Payload.PullRequest.HTMLURL
			}
		case "PullRequestReviewCommentEvent":
			cat = CatComment
			sentence = "commented on a review in"
			if e.Payload.PullRequest.HTMLURL != "" {
				link = e.Payload.PullRequest.HTMLURL
			}
		case "CommitCommentEvent":
			cat = CatComment
			sentence = "commented on a commit in"
		case "ReleaseEvent":
			cat = CatRelease
			tag := e.Payload.Release.TagName
			if tag != "" {
				sentence = "released " + tag + " in"
			} else {
				sentence = "published a release in"
			}
		case "CreateEvent":
			cat = CatRepo
			switch e.Payload.RefType {
			case "repository":
				sentence = "created the repository"
			case "branch":
				sentence = "opened the branch " + e.Payload.Ref + " in"
			case "tag":
				sentence = "tagged " + e.Payload.Ref + " in"
			default:
				sentence = "created " + e.Payload.RefType + " in"
			}
		case "DeleteEvent":
			cat = CatRepo
			switch e.Payload.RefType {
			case "branch":
				sentence = "deleted the branch " + e.Payload.Ref + " in"
			case "tag":
				sentence = "deleted the tag " + e.Payload.Ref + " in"
			default:
				sentence = "deleted " + e.Payload.RefType + " in"
			}
		case "ForkEvent":
			cat = CatRepo
			sentence = "forked"
		case "PublicEvent":
			cat = CatRepo
			sentence = "made the source of this repository public, it is"
		case "MemberEvent":
			cat = CatRepo
			sentence = "added a collaborator to"
		case "GollumEvent":
			cat = CatRepo
			sentence = "edited the wiki of"
		default:
			cat = CatOther
			sentence = "did a " + strings.TrimSuffix(e.Type, "Event") + " in"
		}

		when, err := time.Parse(time.RFC3339, e.CreatedAt)
		if err != nil {
			when = time.Now()
		}

		items = append(items, Item{
			Time:     when.UTC().Format("3:04PM"),
			Date:     when.UTC().Format("2 Jan"),
			Category: cat,
			Sentence: sentence,
			Repo:     short,
			URL:      link,
		})
	}

	stats.Repos = len(repos)
	return items, stats
}
