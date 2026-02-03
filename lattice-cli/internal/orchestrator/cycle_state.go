package orchestrator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type cycleState struct {
	Current int `json:"current"`
}

func (o *Orchestrator) cycleStatePath() string {
	return filepath.Join(o.config.LatticeProjectDir, "state", "cycle.json")
}

func (o *Orchestrator) ensureCycleState() (int, error) {
	path := o.cycleStatePath()
	if _, err := os.Stat(path); err == nil {
		state, err := o.readCycleState()
		if err != nil {
			return 0, err
		}
		if state.Current < 1 {
			state.Current = 1
			if err := o.writeCycleState(state); err != nil {
				return 0, err
			}
		}
		return state.Current, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return 0, err
	}
	state := cycleState{Current: 1}
	if err := o.writeCycleState(state); err != nil {
		return 0, err
	}
	return state.Current, nil
}

func (o *Orchestrator) currentCycleNumber() (int, error) {
	if _, err := os.Stat(o.cycleStatePath()); os.IsNotExist(err) {
		return o.ensureCycleState()
	}
	state, err := o.readCycleState()
	if err != nil {
		return 0, err
	}
	if state.Current < 1 {
		state.Current = 1
		if err := o.writeCycleState(state); err != nil {
			return 0, err
		}
	}
	return state.Current, nil
}

func (o *Orchestrator) incrementCycleNumber() (int, error) {
	state, err := o.readCycleState()
	if os.IsNotExist(err) {
		state.Current = 1
	} else if err != nil {
		return 0, err
	}
	state.Current++
	if err := o.writeCycleState(state); err != nil {
		return 0, err
	}
	return state.Current, nil
}

func (o *Orchestrator) readCycleState() (cycleState, error) {
	path := o.cycleStatePath()
	data, err := os.ReadFile(path)
	if err != nil {
		return cycleState{}, err
	}
	var state cycleState
	if err := json.Unmarshal(data, &state); err != nil {
		return cycleState{}, fmt.Errorf("failed to parse cycle state: %w", err)
	}
	return state, nil
}

func (o *Orchestrator) writeCycleState(state cycleState) error {
	path := o.cycleStatePath()
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
