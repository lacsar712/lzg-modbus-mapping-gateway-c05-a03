package usecase

import (
	"testing"

	"github.com/bytecode/modbus-mapping-gateway/internal/domain"
)

type memAlarmStore struct {
	state domain.AlarmState
}

func (m *memAlarmStore) Load() (domain.AlarmState, error) { return m.state, nil }
func (m *memAlarmStore) Save(st domain.AlarmState) error  { m.state = st; return nil }
func (m *memAlarmStore) Path() string                     { return "memory" }

func newTestAlarms(t *testing.T, rules ...domain.AlarmRule) *AlarmService {
	t.Helper()
	st := domain.AlarmState{}
	st.Rules = append(st.Rules, rules...)
	svc, err := NewAlarmService(&memAlarmStore{state: st}, nil)
	if err != nil {
		t.Fatalf("NewAlarmService: %v", err)
	}
	return svc
}

func TestEvaluateRaisesAndAutoResolves(t *testing.T) {
	high := 30.0
	svc := newTestAlarms(t, domain.AlarmRule{
		DeviceID: "plc", Point: "temperature", High: &high, Enabled: true,
	})

	snap := &domain.Snapshot{DeviceID: "plc", Points: []domain.PointValue{
		{Name: "temperature", Quality: "good", Value: float64(36.5)},
	}}
	raised := svc.Evaluate(snap)
	if len(raised) != 1 || raised[0].Level != domain.AlarmLevelHigh {
		t.Fatalf("want 1 high alarm raised, got %+v", raised)
	}
	if svc.ActiveCount("") != 1 {
		t.Fatalf("want 1 active, got %d", svc.ActiveCount(""))
	}

	// repeated out-of-range snapshot updates, does not duplicate
	svc.Evaluate(&domain.Snapshot{DeviceID: "plc", Points: []domain.PointValue{
		{Name: "temperature", Quality: "good", Value: float64(37.1)},
	}})
	if got := svc.ActiveCount(""); got != 1 {
		t.Fatalf("still want 1 active, got %d", got)
	}

	// back in range -> resolved
	svc.Evaluate(&domain.Snapshot{DeviceID: "plc", Points: []domain.PointValue{
		{Name: "temperature", Quality: "good", Value: float64(25.0)},
	}})
	if svc.ActiveCount("") != 0 {
		t.Fatalf("want 0 active after recovery, got %d", svc.ActiveCount(""))
	}
	evs := svc.ListEvents(AlarmFilter{Status: domain.AlarmStatusResolved})
	if len(evs) != 1 || evs[0].ResolvedAt == "" {
		t.Fatalf("want 1 resolved event with timestamp, got %+v", evs)
	}
}

func TestEvaluateLowThresholdAndDisabled(t *testing.T) {
	low := 10.0
	svc := newTestAlarms(t, domain.AlarmRule{
		DeviceID: "plc", Point: "temp", Low: &low, Enabled: false,
	})
	svc.Evaluate(&domain.Snapshot{DeviceID: "plc", Points: []domain.PointValue{
		{Name: "temp", Quality: "good", Value: float64(5)},
	}})
	if svc.ActiveCount("") != 0 {
		t.Fatal("disabled rule must not alarm")
	}

	r, _ := svc.GetRule("plc", "temp")
	r.Enabled = true
	if _, err := svc.UpsertRule(r); err != nil {
		t.Fatalf("enable rule: %v", err)
	}
	svc.Evaluate(&domain.Snapshot{DeviceID: "plc", Points: []domain.PointValue{
		{Name: "temp", Quality: "good", Value: float64(5)},
	}})
	if svc.ActiveCount("") != 1 {
		t.Fatal("enabled low rule should alarm")
	}
}

func TestEvaluateIgnoresBadQuality(t *testing.T) {
	high := 30.0
	svc := newTestAlarms(t, domain.AlarmRule{
		DeviceID: "plc", Point: "temp", High: &high, Enabled: true,
	})
	svc.Evaluate(&domain.Snapshot{DeviceID: "plc", Points: []domain.PointValue{
		{Name: "temp", Quality: "bad", Error: "missing registers"},
	}})
	if svc.ActiveCount("") != 0 {
		t.Fatal("bad-quality read must not raise alarm")
	}
}

func TestUpsertRuleRejectsUnknownPoint(t *testing.T) {
	high := 1.0
	svc, err := NewAlarmService(&memAlarmStore{}, func(dev, p string) bool {
		return dev == "plc" && p == "known"
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpsertRule(domain.AlarmRule{DeviceID: "plc", Point: "ghost", High: &high, Enabled: true}); err == nil {
		t.Fatal("expected rejection of unknown point")
	}
	if _, err := svc.UpsertRule(domain.AlarmRule{DeviceID: "plc", Point: "known", High: &high, Enabled: true}); err != nil {
		t.Fatalf("known point should be accepted: %v", err)
	}
}

func TestDeleteRuleResolvesActive(t *testing.T) {
	high := 30.0
	svc := newTestAlarms(t, domain.AlarmRule{
		DeviceID: "plc", Point: "temp", High: &high, Enabled: true,
	})
	svc.Evaluate(&domain.Snapshot{DeviceID: "plc", Points: []domain.PointValue{
		{Name: "temp", Quality: "good", Value: 40.0},
	}})
	if err := svc.DeleteRule("plc", "temp"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if svc.ActiveCount("") != 0 {
		t.Fatal("deleting rule should resolve its active alarms")
	}
	if len(svc.ListRules()) != 0 {
		t.Fatal("rule should be gone")
	}
}
