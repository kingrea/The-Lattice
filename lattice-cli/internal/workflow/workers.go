package workflow

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// WorkerEntry represents a hired worker captured in workflow/team/workers.json.
// Capacity is optional and expressed in story points the worker can take on per cycle.
type WorkerEntry struct {
	Name      string `json:"name"`
	Community string `json:"community"`
	Role      string `json:"role"`
	IsSpark   bool   `json:"isSpark,omitempty"`
	Capacity  int    `json:"capacity,omitempty"`
}

// LoadWorkers reads the worker roster from disk.
func LoadWorkers(path string) ([]WorkerEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var workers []WorkerEntry
	if err := json.Unmarshal(data, &workers); err != nil {
		return nil, fmt.Errorf("failed to parse workers roster: %w", err)
	}
	return workers, nil
}

// SaveWorkers writes the worker roster to disk, preserving directory structure.
func SaveWorkers(path string, workers []WorkerEntry) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(workers, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// Normalize ensures essential fields are present.
func (w WorkerEntry) Normalize() (WorkerEntry, error) {
	trimmed := strings.TrimSpace(w.Name)
	if trimmed == "" {
		return WorkerEntry{}, errors.New("worker entry missing name")
	}
	w.Name = trimmed
	w.Role = strings.TrimSpace(w.Role)
	return w, nil
}
