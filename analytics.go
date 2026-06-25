// Package analytics is an app-internal product-analytics plugin for togo —
// event tracking with volume, top-events, funnels, retention and active-user
// metrics (a Mixpanel-lite you self-host inside your togo app).
//
//	a, _ := analytics.FromKernel(k)
//	a.Track(ctx, analytics.Event{Name: "signup", UserID: "u1", Properties: map[string]any{"plan": "pro"}})
//	vol := a.Volume("signup", from, to, analytics.Daily)
package analytics

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/togo-framework/togo"
)

// Event is a tracked product event.
type Event struct {
	Name       string         `json:"name"`
	UserID     string         `json:"user_id,omitempty"`
	AnonID     string         `json:"anon_id,omitempty"`
	Properties map[string]any `json:"properties,omitempty"`
	Timestamp  time.Time      `json:"timestamp"`
}

// actor returns the best identity for a user-scoped metric.
func (e Event) actor() string {
	if e.UserID != "" {
		return e.UserID
	}
	return e.AnonID
}

// Bucket granularity for time-series volume.
type Bucket string

const (
	Hourly Bucket = "hour"
	Daily  Bucket = "day"
)

// Store is the pluggable event backend (in-memory default; DB-backed later).
type Store interface {
	Append(Event)
	Range(from, to time.Time) []Event
}

// memStore is a bounded in-memory ring of events.
type memStore struct {
	mu     sync.RWMutex
	events []Event
	max    int
}

func (m *memStore) Append(e Event) {
	m.mu.Lock()
	m.events = append(m.events, e)
	if len(m.events) > m.max {
		m.events = m.events[len(m.events)-m.max:]
	}
	m.mu.Unlock()
}

func (m *memStore) Range(from, to time.Time) []Event {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Event, 0, len(m.events))
	for _, e := range m.events {
		if (from.IsZero() || !e.Timestamp.Before(from)) && (to.IsZero() || e.Timestamp.Before(to)) {
			out = append(out, e)
		}
	}
	return out
}

// Service is the analytics runtime stored on the kernel (k.Get("analytics")).
type Service struct {
	store  Store
	mu     sync.RWMutex
	traits map[string]map[string]any // userID → traits (Identify)
}

func init() {
	togo.RegisterProviderFunc("analytics", togo.PriorityService, func(k *togo.Kernel) error {
		s := &Service{store: &memStore{max: 100000}, traits: map[string]map[string]any{}}
		k.Set("analytics", s)
		if k.Router != nil {
			s.mountRoutes(k.Router)
		}
		return nil
	})
}

// FromKernel returns the analytics Service registered on the kernel.
func FromKernel(k *togo.Kernel) (*Service, bool) {
	v, ok := k.Get("analytics")
	if !ok {
		return nil, false
	}
	s, ok := v.(*Service)
	return s, ok
}

// New builds a standalone Service (handy for tests / a custom store).
func New(store Store) *Service {
	if store == nil {
		store = &memStore{max: 100000}
	}
	return &Service{store: store, traits: map[string]map[string]any{}}
}

// Track records an event (timestamp defaults to now).
func (s *Service) Track(ctx context.Context, e Event) {
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now()
	}
	s.store.Append(e)
}

// Identify attaches traits to a user.
func (s *Service) Identify(userID string, traits map[string]any) {
	if userID == "" {
		return
	}
	s.mu.Lock()
	t := s.traits[userID]
	if t == nil {
		t = map[string]any{}
		s.traits[userID] = t
	}
	for k, v := range traits {
		t[k] = v
	}
	s.mu.Unlock()
}

// Traits returns a user's identified traits.
func (s *Service) Traits(userID string) map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := map[string]any{}
	for k, v := range s.traits[userID] {
		out[k] = v
	}
	return out
}

// VolumePoint is one time bucket of an event-volume series.
type VolumePoint struct {
	Time  time.Time `json:"time"`
	Count int       `json:"count"`
}

func truncTo(t time.Time, b Bucket) time.Time {
	if b == Hourly {
		return t.Truncate(time.Hour)
	}
	return t.Truncate(24 * time.Hour)
}

// Volume returns the event count per time bucket for an event name ("" = all).
func (s *Service) Volume(name string, from, to time.Time, b Bucket) []VolumePoint {
	counts := map[time.Time]int{}
	for _, e := range s.store.Range(from, to) {
		if name != "" && e.Name != name {
			continue
		}
		counts[truncTo(e.Timestamp, b)]++
	}
	out := make([]VolumePoint, 0, len(counts))
	for t, c := range counts {
		out = append(out, VolumePoint{Time: t, Count: c})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Time.Before(out[j].Time) })
	return out
}

// Count is a name/value with a count (top-events, top-properties).
type Count struct {
	Key   string `json:"key"`
	Count int    `json:"count"`
}

func topN(counts map[string]int, limit int) []Count {
	out := make([]Count, 0, len(counts))
	for k, c := range counts {
		out = append(out, Count{Key: k, Count: c})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Key < out[j].Key
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// TopEvents ranks event names by volume.
func (s *Service) TopEvents(from, to time.Time, limit int) []Count {
	counts := map[string]int{}
	for _, e := range s.store.Range(from, to) {
		counts[e.Name]++
	}
	return topN(counts, limit)
}

// TopProperties ranks values of a property for a given event.
func (s *Service) TopProperties(event, prop string, from, to time.Time, limit int) []Count {
	counts := map[string]int{}
	for _, e := range s.store.Range(from, to) {
		if e.Name != event {
			continue
		}
		if v, ok := e.Properties[prop]; ok {
			counts[toStr(v)]++
		}
	}
	return topN(counts, limit)
}

// FunnelStep is one step's conversion in a funnel.
type FunnelStep struct {
	Event     string  `json:"event"`
	Count     int     `json:"count"`
	Conversion float64 `json:"conversion"` // fraction of step 0 that reached here
}

// Funnel computes ordered conversion: of users who did step 0, how many went on
// to do each subsequent step (in order, by time) within [from,to).
func (s *Service) Funnel(steps []string, from, to time.Time) []FunnelStep {
	if len(steps) == 0 {
		return nil
	}
	// per-user ordered events
	perUser := map[string][]Event{}
	for _, e := range s.store.Range(from, to) {
		u := e.actor()
		if u == "" {
			continue
		}
		perUser[u] = append(perUser[u], e)
	}
	reached := make([]map[string]bool, len(steps))
	for i := range reached {
		reached[i] = map[string]bool{}
	}
	for u, evs := range perUser {
		sort.Slice(evs, func(i, j int) bool { return evs[i].Timestamp.Before(evs[j].Timestamp) })
		idx := 0 // next step to satisfy
		for _, e := range evs {
			if idx < len(steps) && e.Name == steps[idx] {
				reached[idx][u] = true
				idx++
			}
		}
	}
	out := make([]FunnelStep, len(steps))
	base := len(reached[0])
	for i, step := range steps {
		c := len(reached[i])
		conv := 0.0
		if base > 0 {
			conv = float64(c) / float64(base)
		}
		out[i] = FunnelStep{Event: step, Count: c, Conversion: conv}
	}
	return out
}

// RetentionRow is a cohort retention row (returning users at an offset).
type RetentionRow struct {
	OffsetDays int `json:"offset_days"`
	Returned   int `json:"returned"`
}

// Retention: of users first seen in [from,from+1d), how many returned on day
// offsets (0,1,7 by default) — simple activity-based retention.
func (s *Service) Retention(from time.Time, offsets []int) []RetentionRow {
	if len(offsets) == 0 {
		offsets = []int{0, 1, 7}
	}
	cohortEnd := from.Add(24 * time.Hour)
	all := s.store.Range(from, time.Time{})
	cohort := map[string]bool{}
	active := map[string]map[int]bool{} // user → day offsets active
	for _, e := range all {
		u := e.actor()
		if u == "" {
			continue
		}
		if !e.Timestamp.Before(from) && e.Timestamp.Before(cohortEnd) {
			cohort[u] = true
		}
		off := int(e.Timestamp.Sub(from).Hours() / 24)
		if active[u] == nil {
			active[u] = map[int]bool{}
		}
		active[u][off] = true
	}
	out := make([]RetentionRow, 0, len(offsets))
	for _, off := range offsets {
		n := 0
		for u := range cohort {
			if active[u][off] {
				n++
			}
		}
		out = append(out, RetentionRow{OffsetDays: off, Returned: n})
	}
	return out
}

// ActiveUsers counts distinct users active in [from,to).
func (s *Service) ActiveUsers(from, to time.Time) int {
	seen := map[string]bool{}
	for _, e := range s.store.Range(from, to) {
		if u := e.actor(); u != "" {
			seen[u] = true
		}
	}
	return len(seen)
}

// Dashboard is a summary snapshot for an overview UI.
type Dashboard struct {
	TotalEvents int     `json:"total_events"`
	DAU         int     `json:"dau"`
	WAU         int     `json:"wau"`
	TopEvents   []Count `json:"top_events"`
}

// Dashboard returns a summary over the last `days` days (default 7).
func (s *Service) DashboardSummary(days int) Dashboard {
	if days <= 0 {
		days = 7
	}
	now := time.Now()
	weekAgo := now.Add(-7 * 24 * time.Hour)
	dayAgo := now.Add(-24 * time.Hour)
	window := now.Add(-time.Duration(days) * 24 * time.Hour)
	return Dashboard{
		TotalEvents: len(s.store.Range(window, time.Time{})),
		DAU:         s.ActiveUsers(dayAgo, time.Time{}),
		WAU:         s.ActiveUsers(weekAgo, time.Time{}),
		TopEvents:   s.TopEvents(window, time.Time{}, 10),
	}
}

func toStr(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case bool:
		return strconv.FormatBool(t)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case int:
		return strconv.Itoa(t)
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func qtime(r *http.Request, key string) time.Time {
	if v := r.URL.Query().Get(key); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			return t
		}
	}
	return time.Time{}
}

func qint(r *http.Request, key string, def int) int {
	if v := r.URL.Query().Get(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func (s *Service) mountRoutes(r chi.Router) {
	r.Route("/api/analytics", func(r chi.Router) {
		// Batch ingest from a JS/mobile SDK.
		r.Post("/events", func(w http.ResponseWriter, req *http.Request) {
			var body struct {
				Events []Event `json:"events"`
			}
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				writeJSON(w, 400, map[string]string{"error": "invalid body"})
				return
			}
			for _, e := range body.Events {
				s.Track(req.Context(), e)
			}
			writeJSON(w, 202, map[string]any{"accepted": len(body.Events)})
		})
		r.Post("/identify", func(w http.ResponseWriter, req *http.Request) {
			var body struct {
				UserID string         `json:"user_id"`
				Traits map[string]any `json:"traits"`
			}
			_ = json.NewDecoder(req.Body).Decode(&body)
			s.Identify(body.UserID, body.Traits)
			writeJSON(w, 200, map[string]string{"status": "ok"})
		})
		r.Get("/volume", func(w http.ResponseWriter, req *http.Request) {
			b := Daily
			if req.URL.Query().Get("bucket") == "hour" {
				b = Hourly
			}
			writeJSON(w, 200, s.Volume(req.URL.Query().Get("event"), qtime(req, "from"), qtime(req, "to"), b))
		})
		r.Get("/top", func(w http.ResponseWriter, req *http.Request) {
			if p := req.URL.Query().Get("property"); p != "" {
				writeJSON(w, 200, s.TopProperties(req.URL.Query().Get("event"), p, qtime(req, "from"), qtime(req, "to"), qint(req, "limit", 10)))
				return
			}
			writeJSON(w, 200, s.TopEvents(qtime(req, "from"), qtime(req, "to"), qint(req, "limit", 10)))
		})
		r.Get("/funnel", func(w http.ResponseWriter, req *http.Request) {
			steps := req.URL.Query()["step"]
			writeJSON(w, 200, s.Funnel(steps, qtime(req, "from"), qtime(req, "to")))
		})
		r.Get("/retention", func(w http.ResponseWriter, req *http.Request) {
			writeJSON(w, 200, s.Retention(qtime(req, "from"), nil))
		})
		r.Get("/active", func(w http.ResponseWriter, req *http.Request) {
			writeJSON(w, 200, map[string]int{"active_users": s.ActiveUsers(qtime(req, "from"), qtime(req, "to"))})
		})
		r.Get("/dashboard", func(w http.ResponseWriter, req *http.Request) {
			writeJSON(w, 200, s.DashboardSummary(qint(req, "days", 7)))
		})
	})
}
