package remotecontrol

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type connectorState struct {
	Enabled           bool   `json:"enabled"`
	EnvironmentID     string `json:"environmentID,omitempty"`
	EnvironmentName   string `json:"environmentName,omitempty"`
	ServerCredential  string `json:"serverCredential,omitempty"`
	RegionID          string `json:"regionID,omitempty"`
	RoutingGeneration int64  `json:"routingGeneration,omitempty"`
}

type stateStore struct {
	path string
}

func newStateStore(baseDir string) stateStore {
	return stateStore{path: filepath.Join(baseDir, "remote-control", "state.json")}
}

func (s stateStore) Load() (connectorState, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return connectorState{}, nil
	}
	if err != nil {
		return connectorState{}, fmt.Errorf("read remote-control state: %w", err)
	}
	var state connectorState
	if err := json.Unmarshal(data, &state); err != nil {
		return connectorState{}, fmt.Errorf("decode remote-control state: %w", err)
	}
	return state, nil
}

func (s stateStore) Save(state connectorState) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return fmt.Errorf("create remote-control state directory: %w", err)
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode remote-control state: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(s.path), ".state-*.tmp")
	if err != nil {
		return fmt.Errorf("create remote-control state file: %w", err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryName, s.path); err != nil {
		return fmt.Errorf("replace remote-control state: %w", err)
	}
	return nil
}
