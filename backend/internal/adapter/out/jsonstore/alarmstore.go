package jsonstore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/bytecode/modbus-mapping-gateway/internal/domain"
	"github.com/bytecode/modbus-mapping-gateway/internal/port"
)

// AlarmStore persists rules/events to a local JSON file, atomically.
type AlarmStore struct {
	path string
	mu   sync.Mutex
}

var _ port.AlarmStore = (*AlarmStore)(nil)

func NewAlarmStore(path string) *AlarmStore {
	return &AlarmStore{path: path}
}

func (s *AlarmStore) Path() string { return s.path }

func (s *AlarmStore) Load() (domain.AlarmState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var st domain.AlarmState
	b, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return st, nil
		}
		return st, err
	}
	if len(b) == 0 {
		return st, nil
	}
	if err := json.Unmarshal(b, &st); err != nil {
		return domain.AlarmState{}, err
	}
	return st, nil
}

func (s *AlarmStore) Save(state domain.AlarmState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
