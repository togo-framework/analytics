# analytics — usage

## Track events (Go)
```go
a, _ := analytics.FromKernel(k)
a.Track(ctx, analytics.Event{Name: "signup", UserID: "u1", Properties: map[string]any{"plan": "pro"}})
a.Identify("u1", map[string]any{"country": "EG"})
```

## Metrics (Go)
```go
a.Volume("signup", from, to, analytics.Daily)        // []VolumePoint (time series)
a.TopEvents(from, to, 10)                             // []Count
a.TopProperties("signup", "plan", from, to, 10)       // []Count
a.Funnel([]string{"visit","signup","purchase"}, from, to) // []FunnelStep (ordered conversion)
a.Retention(from, []int{0,1,7})                       // []RetentionRow
a.ActiveUsers(from, to)                               // distinct users (DAU/WAU via window)
a.DashboardSummary(7)                                 // overview snapshot
```

## REST
| Method | Path | Purpose |
|---|---|---|
| POST | `/api/analytics/events` | batch ingest `{events:[...]}` from an SDK |
| POST | `/api/analytics/identify` | `{user_id, traits}` |
| GET | `/api/analytics/volume?event=&bucket=day&from=&to=` | volume time series |
| GET | `/api/analytics/top?event=&property=&limit=` | top events or property values |
| GET | `/api/analytics/funnel?step=visit&step=signup&...` | funnel conversion |
| GET | `/api/analytics/retention?from=` | cohort retention (0/1/7) |
| GET | `/api/analytics/active?from=&to=` | active users |
| GET | `/api/analytics/dashboard?days=7` | overview |

Time params are RFC3339.

## JS snippet (client)
```html
<script>
const A = (() => {
  const url = "/api/analytics/events"; let buf = [];
  const flush = () => { if (!buf.length) return; const b = buf; buf = [];
    navigator.sendBeacon(url, JSON.stringify({events:b})); };
  setInterval(flush, 5000); addEventListener("beforeunload", flush);
  return {
    track: (name, properties={}) => buf.push({name, properties, timestamp:new Date().toISOString(), user_id: A.uid}),
    identify: (uid) => { A.uid = uid; },
  };
})();
A.track("page_view", { path: location.pathname });
</script>
```
