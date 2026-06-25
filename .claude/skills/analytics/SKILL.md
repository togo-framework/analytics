---
name: analytics
description: Add product analytics to a togo app with the analytics plugin — track events, build funnels and retention cohorts, and query volume/top-events/active-users.
---

# togo analytics

Use the `togo-framework/analytics` plugin to add app-internal product analytics.

## Track
```go
a, _ := analytics.FromKernel(k)
a.Track(ctx, analytics.Event{Name: "<event>", UserID: "<id>", Properties: map[string]any{...}})
a.Identify("<id>", map[string]any{"plan": "pro"})
```
Or POST batches to `/api/analytics/events` from a JS SDK (see the README snippet).

## Query
- `a.Volume(event, from, to, analytics.Daily)` — time series
- `a.TopEvents(from, to, n)` / `a.TopProperties(event, prop, from, to, n)`
- `a.Funnel([]string{"visit","signup","purchase"}, from, to)` — ordered conversion
- `a.Retention(from, []int{0,1,7})` — cohort retention
- `a.ActiveUsers(from, to)` — DAU/WAU (pick the window)
- `a.DashboardSummary(7)` — overview

## Tips
- Name events consistently (`object_action`, e.g. `order_completed`).
- Funnels are time-ordered per user — list steps in the order they should occur.
- The default store is in-memory + bounded; swap a DB-backed `Store` for durable history.
