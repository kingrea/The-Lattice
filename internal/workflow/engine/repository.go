package engine

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/yourusername/lattice/internal/workflow"
)

// ErrStateNotFound is returned when no persisted engine state exists yet.
var ErrStateNotFound = errors.New("workflow engine: state not found")

// StateStore persists workflow engine state snapshots.
type StateStore interface {
	Load() (State, error)
	Save(State) error
}

// Repository stores engine state within the workflow directory.
type Repository struct {
	path string
}

// NewRepository creates a repository rooted at the workflow engine directory.
func NewRepository(wf *workflow.Workflow) *Repository {
	return &Repository{path: filepath.Join(wf.Dir(), "engine", "state.json")}
}

// Load reads the persisted state if present.
func (r *Repository) Load() (State, error) {
	data, err := os.ReadFile(r.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return State{}, ErrStateNotFound
		}
		return State{}, err
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, err
	}
	return state, nil
}

// Save writes the engine state to disk with best-effort atomicity.
func (r *Repository) Save(state State) error {
	if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(r.path, append(encoded, '\n'), 0o644)
}
