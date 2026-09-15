package domain

import "fmt"

// Alarm levels.
const (
	AlarmLevelHigh = "high"
	AlarmLevelLow  = "low"
)

// Alarm statuses.
const (
	AlarmStatusActive   = "active"
	AlarmStatusResolved = "resolved"
)

// AlarmRule configures read-side threshold monitoring for a point.
//
// 注意与 PointDef.Min/Max 的区别：Min/Max 是写值时的合法范围校验
// （CheckMinMax，面向写），报警阈值面向 snapshot 读到的工程值，
// 越界只产生报警事件，不阻止任何操作。
type AlarmRule struct {
	DeviceID string   `json:"deviceId" yaml:"deviceId"`
	Point    string   `json:"point" yaml:"point"`
	High     *float64 `json:"high,omitempty" yaml:"high,omitempty"`
	Low      *float64 `json:"low,omitempty" yaml:"low,omitempty"`
	Enabled  bool     `json:"enabled" yaml:"enabled"`
}

// RuleKey uniquely identifies a rule by device + point.
func (r AlarmRule) RuleKey() string {
	return r.DeviceID + "/" + r.Point
}

func (r AlarmRule) Validate() error {
	if r.DeviceID == "" {
		return fmt.Errorf("alarm rule: deviceId required")
	}
	if r.Point == "" {
		return fmt.Errorf("alarm rule: point required")
	}
	if r.High == nil && r.Low == nil {
		return fmt.Errorf("alarm rule %s/%s: at least one of high/low required", r.DeviceID, r.Point)
	}
	if r.High != nil && r.Low != nil && *r.Low >= *r.High {
		return fmt.Errorf("alarm rule %s/%s: low must be below high", r.DeviceID, r.Point)
	}
	return nil
}

// AlarmEvent is one threshold excursion of a point.
type AlarmEvent struct {
	ID           string  `json:"id"`
	DeviceID     string  `json:"deviceId"`
	Point        string  `json:"point"`
	Level        string  `json:"level"`  // high | low
	Status       string  `json:"status"` // active | resolved
	Threshold    float64 `json:"threshold"`
	TriggerValue float64 `json:"triggerValue"`
	LastValue    float64 `json:"lastValue"`
	TriggeredAt  string  `json:"triggeredAt"`
	UpdatedAt    string  `json:"updatedAt"`
	ResolvedAt   string  `json:"resolvedAt,omitempty"`
}

// AlarmState is the persisted alarm rule/event set.
type AlarmState struct {
	Rules  []AlarmRule  `json:"rules"`
	Events []AlarmEvent `json:"events"`
	Seq    int          `json:"seq"`
}

// ClassifyAlarm returns "high"/"low" when eng violates the rule, "" otherwise.
func ClassifyAlarm(eng float64, rule AlarmRule) string {
	if !rule.Enabled {
		return ""
	}
	if rule.High != nil && eng > *rule.High {
		return AlarmLevelHigh
	}
	if rule.Low != nil && eng < *rule.Low {
		return AlarmLevelLow
	}
	return ""
}
