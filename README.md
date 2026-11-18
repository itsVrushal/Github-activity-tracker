# Activity Arcade

A Go webapp that fetches a GitHub user's recent public activity and displays it
as a feed: type a username, press fetch, and see their last 30 public events
with a score line and filters by event type. It also draws the contribution
grid from their GitHub profile — a full year of daily squares in the Arcade
theme's ember orange.

The feed uses the [public GitHub events API](https://docs.github.com/en/rest/activity/events);
the contribution grid uses GitHub's public contributions graph
(`github.com/users/<user>/contributions`), the same one a public profile shows.
Both use nothing but the Go standard library.
The UI is an app screen in the Arcade theme (aubergine CRT ground, Bungee
display, Silkscreen figures, ember and acid inks).

## Run

```sh
go build -o arcade-tracker .
PORT=8899 ./arcade-tracker
```

Then open http://localhost:8899/.

`go run .` works too. `PORT` defaults to 8080.

## Rate limits and the token

Without a token GitHub allows 60 requests per hour per IP. The app uses one
request per page (plus one profile lookup per user), caches each result for
two minutes, and shows the reset time when the budget is spent.

For 5,000 requests per hour, set a token before starting:

```sh
GITHUB_TOKEN=ghp_yourtokenhere ./arcade-tracker
```

A fine-grained token with **no** repository permissions is enough; the public
events endpoint only needs read access to public data.

## Contribution grid

The year-long grid shows for any public username, with or without a token
(`GITHUB_TOKEN` only raises the REST API rate-limit budget). The grid is
cached for an hour (the feed for two minutes), and each square has its date
and count in a tooltip.

GitHub only exposes **public** contributions for another user, so the grid
matches their public profile, not private activity.

## What it maps

| Event | Shown as |
| --- | --- |
| `PushEvent` | pushed N commits to (or pushed to, when GitHub sends no commit list) |
| `WatchEvent` | starred |
| `PullRequestEvent` | opened / closed / merged a pull request in |
| `IssuesEvent` | opened / reopened / closed an issue in |
| `IssueCommentEvent`, `PullRequestReviewEvent`, `PullRequestReviewCommentEvent`, `CommitCommentEvent` | commented on / reviewed |
| `ReleaseEvent` | released \<tag\> in |
| `CreateEvent`, `DeleteEvent`, `ForkEvent`, `PublicEvent`, `MemberEvent`, `GollumEvent` | created / deleted / forked / made public / added a collaborator / edited the wiki |
| anything else | did a \<Type\> in (never dropped) |

## API

`GET /api/activity?username=<name>&page=<n>` returns JSON. Page 1 is the
newest 30 events; each further page reaches one page deeper into GitHub's
memory, which holds at most 10 pages (300 events) and only for 90 days. The
`has_more` field says whether another page exists.

```json
{
  "user": { "login": "octocat", "name": "The Octocat", "avatar_url": "...", "url": "..." },
  "fetched_at": "2026-11-17T14:07:37+05:30",
  "page": 3,
  "has_more": true,
  "events": [ { "time": "10:03PM", "date": "13 Nov", "category": "push", "sentence": "pushed to", "repo": "octocat/hello-world", "url": "..." } ],
  "stats": { "events": 90, "pushes": 14, "commits": 0, "stars": 0, "pulls": 1, "issues": 0, "repos": 4 }
}
```

Errors: 400 (bad username), 404 (unknown user), 429 (rate limited, with a
`reset` field), 502 (network or unexpected GitHub answer).

## Deploy

A `Dockerfile` is included; the binary is self-contained (templates and static
assets are embedded with `go:embed`).

```sh
docker build -t activity-arcade .
docker run -p 8080:8080 -e GITHUB_TOKEN=ghp_... activity-arcade
```

## Layout

```
main.go             server, routes, graceful shutdown, .env loader
github.go           GitHub REST client (profile + public events, error mapping)
contributions.go    GraphQL contribution calendar, grid layout, formatting
activity.go         event -> sentence mapping, categories, stats
cache.go            two-minute feed cache + one-hour calendar cache
handlers.go         Templ page/fragment handlers and JSON endpoint
templates.templ     type-safe Templ components (Page, Board, Grid, Stats, Chips, Feed)
static/             styles.css (Arcade theme), htmx.min.js
```
