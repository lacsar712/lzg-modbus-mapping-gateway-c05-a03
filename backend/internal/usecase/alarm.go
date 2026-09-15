package usecase

import (
	"fmt"
	"log"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/bytecode/modbus-mapping-gateway/internal/domain"
	"github.com/bytecode/modbus-mapping-gateway/internal/port"
)

// AlarmService maintains threshold rules and the alarm events generated
// from snapshots. Alarms are a read-side concern: they never block writes
// (write range validation is domain.CheckMinMax on PointDef.Min/Max).
type AlarmService struct {
	store port.AlarmStore

	// pointLookup reports whether a point exists in the current mapping.
	pointLookup func(deviceID, point string) bool

	mu     sync.Mutex
	rules  map[string]domain.AlarmRule // key device/point
	events []domain.AlarmEvent
	seq    int
}

// NewAlarmService loads persisted state. lookup is used to validate rule
// targets against the live mapping; pass nil to accept any target.
func NewAlarmService(store port.AlarmStore, lookup func(deviceID, point string) bool) (*AlarmService, error) {
	s := &AlarmService{store: store, pointLookup: lookup, rules: map[string]domain.AlarmRule{}}
	st, err := store.Load()
	if err != nil {
		return nil, fmt.Errorf("load alarm state: %w", err)
	}
	for _, r := range st.Rules {
		s.rules[r.RuleKey()] = r
	}
	s.events = st.Events
	if s.events == nil {
		s.events = []domain.AlarmEvent{}
	}
	s.seq = st.Seq
	return s, nil
}

// SetPointLookup replaces the mapping-existence check (used after reloads).
func (s *AlarmService) SetPointLookup(lookup func(deviceID, point string) bool) {
	s.mu.Lock()
	s.pointLookup = lookup
	s.mu.Unlock()
}

func (s *AlarmService) persistLocked() {
	rules := make([]domain.AlarmRule, 0, len(s.rules))
	for _, r := range s.rules {
		rules = append(rules, r)
	}
	sortRules(rules)
	events := s.events
	if events == nil {
		events = []domain.AlarmEvent{}
	}
	// Best-effort persistence: evaluation must not fail just because disk is slow.
	if err := s.store.Save(domain.AlarmState{Rules: rules, Events: events, Seq: s.seq}); err != nil {
		log.Printf("persist alarm state: %v", err)
	}
}

// ListRules returns all rules sorted by device, point.
func (s *AlarmService) ListRules() []domain.AlarmRule {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]domain.AlarmRule, 0, len(s.rules))
	for _, r := range s.rules {
		out = append(out, r)
	}
	sortRules(out)
	return out
}

// GetRule returns the rule for a point.
func (s *AlarmService) GetRule(deviceID, point string) (domain.AlarmRule, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.rules[deviceID+"/"+point]
	return r, ok
}

// UpsertRule creates or replaces the rule for a point.
func (s *AlarmService) UpsertRule(rule domain.AlarmRule) (domain.AlarmRule, error) {
	if err := rule.Validate(); err != nil {
		return domain.AlarmRule{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pointLookup != nil && !s.pointLookup(rule.DeviceID, rule.Point) {
		return domain.AlarmRule{}, fmt.Errorf("point not found: %s/%s", rule.DeviceID, rule.Point)
	}
	s.rules[rule.RuleKey()] = rule
	s.persistLocked()
	return rule, nil
}

// DeleteRule removes a rule. Its active events are auto-resolved first.
func (s *AlarmService) DeleteRule(deviceID, point string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := deviceID + "/" + point
	if _, ok := s.rules[key]; !ok {
		return fmt.Errorf("alarm rule not found: %s", key)
	}
	delete(s.rules, key)
	now := time.Now().UTC().Format(time.RFC3339)
	for i := range s.events {
		if s.events[i].DeviceID == deviceID && s.events[i].Point == point &&
			s.events[i].Status == domain.AlarmStatusActive {
			s.resolveLocked(&s.events[i], now, s.events[i].LastValue)
		}
	}
	s.persistLocked()
	return nil
}

// AlarmFilter narrows event listings.
type AlarmFilter struct {
	DeviceID string
	Point    string
	Status   string // "" | active | resolved
	Limit    int
}

// ListEvents returns events newest-first, optionally filtered.
func (s *AlarmService) ListEvents(f AlarmFilter) []domain.AlarmEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]domain.AlarmEvent, 0, len(s.events))
	for i := len(s.events) - 1; i >= 0; i-- {
		e := s.events[i]
		if f.DeviceID != "" && e.DeviceID != f.DeviceID {
			continue
		}
		if f.Point != "" && e.Point != f.Point {
			continue
		}
		if f.Status != "" && e.Status != f.Status {
			continue
		}
		out = append(out, e)
	}
	if f.Limit > 0 && len(out) > f.Limit {
		out = out[:f.Limit]
	}
	return out
}

// ActiveCount returns the number of active alarms, optionally per device.
func (s *AlarmService) ActiveCount(deviceID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, e := range s.events {
		if e.Status != domain.AlarmStatusActive {
			continue
		}
		if deviceID != "" && e.DeviceID != deviceID {
			continue
		}
		n++
	}
	return n
}

// ActiveCounts returns active alarm counts keyed by device ID.
func (s *AlarmService) ActiveCounts() map[string]int {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := map[string]int{}
	for _, e := range s.events {
		if e.Status == domain.AlarmStatusActive {
			m[e.DeviceID]++
		}
	}
	return m
}

// ActiveByPoint returns active level ("high"/"low") for each alarming point.
func (s *AlarmService) ActiveByPoint(deviceID string) map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := map[string]string{}
	for _, e := range s.events {
		if e.Status == domain.AlarmStatusActive && (deviceID == "" || e.DeviceID == deviceID) {
			m[e.DeviceID+"/"+e.Point] = e.Level
		}
	}
	return m
}

// ClearEvent manually resolves one event.
func (s *AlarmService) ClearEvent(id string) (domain.AlarmEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.events {
		if s.events[i].ID == id {
			if s.events[i].Status == domain.AlarmStatusResolved {
				return s.events[i], nil
			}
			s.resolveLocked(&s.events[i], time.Now().UTC().Format(time.RFC3339), s.events[i].LastValue)
			s.persistLocked()
			return s.events[i], nil
		}
	}
	return domain.AlarmEvent{}, fmt.Errorf("alarm event not found: %s", id)
}

func (s *AlarmService) resolveLocked(e *domain.AlarmEvent, at string, value float64) {
	e.Status = domain.AlarmStatusResolved
	e.ResolvedAt = at
	e.UpdatedAt = at
	e.LastValue = value
}

// Evaluate runs all enabled rules against one snapshot and opens/updates/closes events.
func (s *AlarmService) Evaluate(snap *domain.Snapshot) []domain.AlarmEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC().Format(time.RFC3339)
	changed := false
	var raised []domain.AlarmEvent

	for _, pv := range snap.Points {
		key := snap.DeviceID + "/" + pv.Name
		rule, ok := s.rules[key]
		if !ok || !rule.Enabled {
			continue
		}
		eng, ok := numberValue(pv.Value)
		if !ok || pv.Quality != "good" {
			continue
		}
		level := domain.ClassifyAlarm(eng, rule)
		active := s.findActiveLocked(snap.DeviceID, pv.Name)

		if level != "" {
			threshold := rule.High
			if level == domain.AlarmLevelLow {
				threshold = rule.Low
			}
			if active == nil {
				s.seq++
				e := domain.AlarmEvent{
					ID:           "alm-" + strconv.Itoa(s.seq),
					DeviceID:     snap.DeviceID,
					Point:        pv.Name,
					Level:        level,
					Status:       domain.AlarmStatusActive,
					Threshold:    *threshold,
					TriggerValue: eng,
					LastValue:    eng,
					TriggeredAt:  now,
					UpdatedAt:    now,
				}
				s.events = append(s.events, e)
				raised = append(raised, e)
				changed = true
			} else if active.Level != level {
				// crossed from one side to the other without a normal reading:
				// close old event and open a new one.
				active.Status = domain.AlarmStatusResolved
				active.ResolvedAt = now
				active.UpdatedAt = now
				s.seq++
				e := domain.AlarmEvent{
					ID:           "alm-" + strconv.Itoa(s.seq),
					DeviceID:     snap.DeviceID,
					Point:        pv.Name,
					Level:        level,
					Status:       domain.AlarmStatusActive,
					Threshold:    *threshold,
					TriggerValue: eng,
					LastValue:    eng,
					TriggeredAt:  now,
					UpdatedAt:    now,
				}
				s.events = append(s.events, e)
				raised = append(raised, e)
				changed = true
			} else if active.LastValue != eng {
				active.LastValue = eng
				active.UpdatedAt = now
				changed = true
			}
		} else if active != nil {
			// value back within range -> auto-resolve
			active.Status = domain.AlarmStatusResolved
			active.ResolvedAt = now
			active.UpdatedAt = now
			active.LastValue = eng
			changed = true
		}
	}

	// Rules still existing but points absent from this snapshot are left alone.
	if changed {
		s.persistLocked()
	}
	return raised
}

func (s *AlarmService) findActiveLocked(deviceID, point string) *domain.AlarmEvent {
	for i := range s.events {
		e := &s.events[i]
		if e.DeviceID == deviceID && e.Point == point && e.Status == domain.AlarmStatusActive {
			return e
		}
	}
	return nil
}

func sortRules(r []domain.AlarmRule) {
	sort.Slice(r, func(i, j int) bool {
		if r[i].DeviceID != r[j].DeviceID {
			return r[i].DeviceID < r[j].DeviceID
		}
		return r[i].Point < r[j].Point
	})
}

// numberValue extracts a float64 from a snapshot value (bool/int64/float64).
func numberValue(v interface{}) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int64:
		return float64(x), true
	case int:
		return float64(x), true
	case bool:
		if x {
			return 1, true
		}
		return 0, true
	default:
		return 0, false
	}
}
