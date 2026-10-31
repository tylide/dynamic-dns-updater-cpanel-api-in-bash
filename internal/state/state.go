package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// State avoids hammering parse_zone when nothing changed.
// Source of truth stays the cPanel API; this is only a short-circuit.
type State struct {
	PublicIP  string    `json:"public_ip"`
	ZoneIP    string    `json:"zone_ip"`
	InSync    bool      `json:"in_sync"`
	LastCheck time.Time `json:"last_check"`
}

// Load reads the file; missing file = empty state, nil error.
func Load(path string) (*State, error) {
	s := &State{}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, err
	}
	if len(b) == 0 {
		return s, nil
	}
	if err := json.Unmarshal(b, s); err != nil {
		return nil, err
	}
	return s, nil
}

// Save writes atomically with 0600.
func (s *State) Save(path string) error {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Fresh reports whether we can skip the API check.
func (s *State) Fresh(publicIP string, forceRefresh time.Duration) bool {
	if !s.InSync || s.PublicIP == "" {
		return false
	}
	if s.PublicIP != publicIP {
		return false
	}
	if s.ZoneIP != "" && s.ZoneIP != publicIP {
		return false
	}
	if forceRefresh <= 0 {
		return true
	}
	return time.Since(s.LastCheck) < forceRefresh
}
