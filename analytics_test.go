package analytics

import (
	"context"
	"testing"
	"time"
)

func ev(name, user string, ts time.Time, props map[string]any) Event {
	return Event{Name: name, UserID: user, Properties: props, Timestamp: ts}
}

func TestTrackAndActiveUsers(t *testing.T) {
	s := New(nil)
	base := time.Date(2026, 6, 25, 10, 0, 0, 0, time.UTC)
	s.Track(context.Background(), ev("login", "u1", base, nil))
	s.Track(context.Background(), ev("login", "u1", base.Add(time.Minute), nil)) // same user
	s.Track(context.Background(), ev("login", "u2", base.Add(2*time.Minute), nil))
	if n := s.ActiveUsers(base.Add(-time.Hour), base.Add(time.Hour)); n != 2 {
		t.Fatalf("active users = %d, want 2 (deduped)", n)
	}
}

func TestVolumeBucketing(t *testing.T) {
	s := New(nil)
	d1 := time.Date(2026, 6, 24, 9, 0, 0, 0, time.UTC)
	d2 := time.Date(2026, 6, 25, 9, 0, 0, 0, time.UTC)
	s.Track(context.Background(), ev("pageview", "u1", d1, nil))
	s.Track(context.Background(), ev("pageview", "u2", d1.Add(time.Hour), nil))
	s.Track(context.Background(), ev("pageview", "u3", d2, nil))
	v := s.Volume("pageview", time.Time{}, time.Time{}, Daily)
	if len(v) != 2 {
		t.Fatalf("expected 2 day buckets, got %d (%+v)", len(v), v)
	}
	if v[0].Count != 2 || v[1].Count != 1 {
		t.Errorf("bucket counts = %d,%d want 2,1", v[0].Count, v[1].Count)
	}
}

func TestTopEventsAndProperties(t *testing.T) {
	s := New(nil)
	now := time.Now()
	for i := 0; i < 3; i++ {
		s.Track(context.Background(), ev("click", "u", now, map[string]any{"button": "buy"}))
	}
	s.Track(context.Background(), ev("click", "u", now, map[string]any{"button": "cancel"}))
	s.Track(context.Background(), ev("view", "u", now, nil))

	top := s.TopEvents(time.Time{}, time.Time{}, 10)
	if len(top) == 0 || top[0].Key != "click" || top[0].Count != 4 {
		t.Fatalf("top events = %+v, want click:4 first", top)
	}
	props := s.TopProperties("click", "button", time.Time{}, time.Time{}, 10)
	if len(props) != 2 || props[0].Key != "buy" || props[0].Count != 3 {
		t.Fatalf("top properties = %+v, want buy:3 first", props)
	}
}

func TestFunnelConversion(t *testing.T) {
	s := New(nil)
	now := time.Now()
	// u1, u2, u3 visit; u1+u2 sign up; only u1 purchases — in order.
	for _, u := range []string{"u1", "u2", "u3"} {
		s.Track(context.Background(), ev("visit", u, now, nil))
	}
	s.Track(context.Background(), ev("signup", "u1", now.Add(time.Minute), nil))
	s.Track(context.Background(), ev("signup", "u2", now.Add(time.Minute), nil))
	s.Track(context.Background(), ev("purchase", "u1", now.Add(2*time.Minute), nil))

	f := s.Funnel([]string{"visit", "signup", "purchase"}, time.Time{}, time.Time{})
	if len(f) != 3 {
		t.Fatalf("funnel len %d", len(f))
	}
	if f[0].Count != 3 || f[1].Count != 2 || f[2].Count != 1 {
		t.Fatalf("funnel counts = %d,%d,%d want 3,2,1", f[0].Count, f[1].Count, f[2].Count)
	}
	if f[2].Conversion < 0.33 || f[2].Conversion > 0.34 {
		t.Errorf("final conversion = %.3f, want ~0.333", f[2].Conversion)
	}
}

func TestFunnelRespectsOrder(t *testing.T) {
	s := New(nil)
	now := time.Now()
	// purchase BEFORE signup must not count as converted through the funnel.
	s.Track(context.Background(), ev("visit", "u1", now, nil))
	s.Track(context.Background(), ev("purchase", "u1", now.Add(time.Minute), nil))
	s.Track(context.Background(), ev("signup", "u1", now.Add(2*time.Minute), nil))
	f := s.Funnel([]string{"visit", "signup", "purchase"}, time.Time{}, time.Time{})
	if f[2].Count != 0 {
		t.Fatalf("out-of-order purchase counted: %+v", f)
	}
}

func TestRetentionCohort(t *testing.T) {
	s := New(nil)
	from := time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC)
	// u1 active day 0,1,7 ; u2 active day 0 only.
	s.Track(context.Background(), ev("open", "u1", from.Add(time.Hour), nil))
	s.Track(context.Background(), ev("open", "u1", from.Add(25*time.Hour), nil))
	s.Track(context.Background(), ev("open", "u1", from.Add(7*24*time.Hour+time.Hour), nil))
	s.Track(context.Background(), ev("open", "u2", from.Add(2*time.Hour), nil))

	rows := s.Retention(from, []int{0, 1, 7})
	want := map[int]int{0: 2, 1: 1, 7: 1}
	for _, r := range rows {
		if r.Returned != want[r.OffsetDays] {
			t.Errorf("offset %d returned %d, want %d", r.OffsetDays, r.Returned, want[r.OffsetDays])
		}
	}
}

func TestIdentifyAndDashboard(t *testing.T) {
	s := New(nil)
	s.Identify("u1", map[string]any{"plan": "pro"})
	s.Identify("u1", map[string]any{"country": "EG"})
	tr := s.Traits("u1")
	if tr["plan"] != "pro" || tr["country"] != "EG" {
		t.Fatalf("traits merge failed: %+v", tr)
	}
	s.Track(context.Background(), ev("x", "u1", time.Now(), nil))
	d := s.DashboardSummary(7)
	if d.TotalEvents != 1 || d.DAU != 1 {
		t.Errorf("dashboard = %+v", d)
	}
}
