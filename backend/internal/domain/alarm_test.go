package domain

import "testing"

func f(v float64) *float64 { return &v }

func TestAlarmRuleValidate(t *testing.T) {
	cases := []struct {
		name string
		rule AlarmRule
		ok   bool
	}{
		{"high only", AlarmRule{DeviceID: "d", Point: "p", High: f(10)}, true},
		{"low only", AlarmRule{DeviceID: "d", Point: "p", Low: f(0)}, true},
		{"both", AlarmRule{DeviceID: "d", Point: "p", Low: f(0), High: f(10)}, true},
		{"missing device", AlarmRule{Point: "p", High: f(10)}, false},
		{"missing point", AlarmRule{DeviceID: "d", High: f(10)}, false},
		{"no thresholds", AlarmRule{DeviceID: "d", Point: "p"}, false},
		{"low >= high", AlarmRule{DeviceID: "d", Point: "p", Low: f(10), High: f(10)}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.rule.Validate()
			if tc.ok && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tc.ok && err == nil {
				t.Fatalf("expected error, got nil")
			}
		})
	}
}

func TestClassifyAlarm(t *testing.T) {
	rule := AlarmRule{DeviceID: "d", Point: "p", Low: f(10), High: f(30), Enabled: true}
	if got := ClassifyAlarm(30.0001, rule); got != AlarmLevelHigh {
		t.Fatalf("want high, got %q", got)
	}
	if got := ClassifyAlarm(9.9, rule); got != AlarmLevelLow {
		t.Fatalf("want low, got %q", got)
	}
	if got := ClassifyAlarm(20, rule); got != "" {
		t.Fatalf("want none, got %q", got)
	}
	// boundary is inclusive-OK
	if got := ClassifyAlarm(30, rule); got != "" {
		t.Fatalf("boundary high should not alarm, got %q", got)
	}
	if got := ClassifyAlarm(10, rule); got != "" {
		t.Fatalf("boundary low should not alarm, got %q", got)
	}
	// disabled rule never alarms
	rule.Enabled = false
	if got := ClassifyAlarm(100, rule); got != "" {
		t.Fatalf("disabled rule alarmed: %q", got)
	}
}
