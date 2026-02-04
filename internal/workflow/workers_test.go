package workflow

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadWorkersLegacyFormat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workers.json")
	legacy := []WorkerEntry{{Name: "Ada", Role: "specialist"}}
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatalf("marshal legacy: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write legacy file: %v", err)
	}
	workers, err := LoadWorkers(path)
	if err != nil {
		t.Fatalf("LoadWorkers legacy: %v", err)
	}
	if len(workers) != 1 || workers[0].Name != "Ada" {
		t.Fatalf("unexpected legacy workers: %+v", workers)
	}
}

func TestLoadWorkersEnvelopeFormat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workers.json")
	payload := struct {
		Workers  []WorkerEntry  `json:"workers"`
		Metadata map[string]any `json:"_lattice"`
	}{
		Workers:  []WorkerEntry{{Name: "Mina", Role: "specialist"}},
		Metadata: map[string]any{"artifact": "workers-json"},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write envelope file: %v", err)
	}
	workers, err := LoadWorkers(path)
	if err != nil {
		t.Fatalf("LoadWorkers envelope: %v", err)
	}
	if len(workers) != 1 || workers[0].Name != "Mina" {
		t.Fatalf("unexpected envelope workers: %+v", workers)
	}
}
