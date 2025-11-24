package state

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// DefaultStatePath is the default location for the state file
	DefaultStatePath = "/var/lib/focusd/state"

	stateEnabled  = "enabled"
	stateDisabled = "disabled"
)

// State represents the current state of focusd
type State struct {
	path string
}

// New creates a new State manager with the given path
func New(path string) *State {
	if path == "" {
		path = DefaultStatePath
	}
	return &State{path: path}
}

// IsEnabled returns true if blocking is currently enabled
func (s *State) IsEnabled() (bool, error) {
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		// Default to enabled if state file doesn't exist
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("reading state file: %w", err)
	}

	state := strings.TrimSpace(string(data))
	return state == stateEnabled, nil
}

// SetEnabled sets the blocking state
func (s *State) SetEnabled(enabled bool) error {
	// Ensure the directory exists
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("creating state directory: %w", err)
	}

	value := stateDisabled
	if enabled {
		value = stateEnabled
	}

	if err := os.WriteFile(s.path, []byte(value+"\n"), 0o640); err != nil {
		return fmt.Errorf("writing state file: %w", err)
	}

	return nil
}

// String returns the current state as a string
func (s *State) String() (string, error) {
	enabled, err := s.IsEnabled()
	if err != nil {
		return "", err
	}

	if enabled {
		return "enabled", nil
	}
	return "disabled", nil
}
