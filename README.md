<div align="center">
  <img src=".github/assets/togo-mark.svg" alt="togo" height="64" />
  <h1>togo-framework/analytics</h1>
  <p>
    <a href="https://to-go.dev/marketplace"><img src="https://img.shields.io/badge/marketplace-to--go.dev-1FC7DC" alt="marketplace" /></a>
    <a href="https://pkg.go.dev/github.com/togo-framework/analytics"><img src="https://pkg.go.dev/badge/github.com/togo-framework/analytics.svg" alt="pkg.go.dev" /></a>
    <img src="https://img.shields.io/badge/license-MIT-blue" alt="MIT" />
  </p>
  <p><strong>App-internal product analytics for <a href="https://to-go.dev">togo</a> — event tracking, funnels, retention & dashboards.</strong></p>
</div>

## Install

```bash
togo install togo-framework/analytics
```

A self-hosted, Mixpanel-lite analytics layer inside your togo app: **track events**, then query **volume over time**, **top events/properties**, **funnels**, **retention cohorts**, and **active users (DAU/WAU)** — over a Go API and REST. Ships with a tiny JS snippet to post events from the browser.

## Track

```go
import "github.com/togo-framework/analytics"

a, _ := analytics.FromKernel(k)
a.Track(ctx, analytics.Event{
	Name:       "order_completed",
	UserID:     "u_123",
	Properties: map[string]any{"plan": "pro", "amount": 49},
})
a.Identify("u_123", map[string]any{"country": "EG"})
```

Or POST batches to `POST /api/analytics/events` from the [JS snippet](docs/usage.md#js-snippet-client).

## Metrics

```go
a.Volume("order_completed", from, to, analytics.Daily)            // time series
a.TopEvents(from, to, 10)                                         // most frequent events
a.TopProperties("order_completed", "plan", from, to, 10)          // value distribution
a.Funnel([]string{"visit", "signup", "order_completed"}, from, to) // ordered conversion
a.Retention(from, []int{0, 1, 7})                                 // cohort retention
a.ActiveUsers(from, to)                                           // distinct users (DAU/WAU)
a.DashboardSummary(7)                                             // overview snapshot
```

### REST

| Method | Path | Purpose |
|---|---|---|
| `POST` | `/api/analytics/events` | batch ingest `{events:[…]}` |
| `POST` | `/api/analytics/identify` | `{user_id, traits}` |
| `GET` | `/api/analytics/volume` | volume time series (`event`, `bucket=day\|hour`, `from`, `to`) |
| `GET` | `/api/analytics/top` | top events, or top values of `property` |
| `GET` | `/api/analytics/funnel` | conversion (`?step=a&step=b&…`) |
| `GET` | `/api/analytics/retention` | cohort retention |
| `GET` | `/api/analytics/active` | active users |
| `GET` | `/api/analytics/dashboard` | overview |

See **[docs/usage.md](docs/usage.md)** for the full API + the client snippet.

## Configuration

No required env. The default event **store is in-memory and bounded** (great for dev + recent-window analytics). For durable history, implement the `analytics.Store` interface against your database (or stream events to a warehouse) and pass it to `analytics.New(store)`.

---

<div align="center">
  <h3>Premium sponsors</h3>
  <p>
    <a href="https://id8media.com"><strong>ID8 Media</strong></a> &nbsp;·&nbsp;
    <a href="https://one-studio.co"><strong>One Studio</strong></a>
  </p>
  <p><sub>Support togo — <a href="https://github.com/sponsors/fadymondy">become a sponsor</a>.</sub></p>
</div>
