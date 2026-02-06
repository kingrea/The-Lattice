package eventbridge

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/kingrea/The-Lattice/internal/config"
)

func TestSettingsFromConfigHonorsEnv(t *testing.T) {
	t.Setenv("LATTICE_BRIDGE_PORT", "9001")
	t.Setenv("LATTICE_BRIDGE_HOST", "0.0.0.0")
	t.Setenv("LATTICE_BRIDGE_ENABLED", "false")
	cfg := &config.Config{}
	settings := SettingsFromConfig(cfg)
	if settings.Port != 9001 {
		t.Fatalf("expected port 9001, got %d", settings.Port)
	}
	if settings.Host != "0.0.0.0" {
		t.Fatalf("expected host override, got %s", settings.Host)
	}
	if settings.Enabled {
		t.Fatalf("expected enabled=false from env override")
	}
}

func TestEventValidate(t *testing.T) {
	evt := Event{
		Version:   EventSchemaVersion,
		EventID:   "abc",
		Type:      "model_response",
		SessionID: "session",
		ModuleID:  "module",
		Workflow:  "wf",
	}
	if err := evt.Validate(); err != nil {
		t.Fatalf("expected valid event, got %v", err)
	}
	evt.Version = 99
	if err := evt.Validate(); err == nil {
		t.Fatalf("expected version error")
	}
}

func TestServerAcceptsEvents(t *testing.T) {
	t.Parallel()
	fixed := time.Unix(1730000000, 0).UTC()
	recorded := make(chan Event, 1)
	settings := Settings{Enabled: true, Host: "127.0.0.1", Port: 0, MaxBodyBytes: 1024, ReadTimeout: time.Second, WriteTimeout: time.Second, IdleTimeout: time.Second}
	srv := NewServer(settings,
		WithClock(func() time.Time { return fixed }),
		WithProcessor(EventProcessorFunc(func(e Event) error {
			recorded <- e
			return nil
		})))
	t.Cleanup(func() {
		_ = srv.Shutdown(context.Background())
	})
	if err := srv.Start(context.Background()); err != nil {
		t.Fatalf("start server: %v", err)
	}
	base := srv.BaseURL()
	resp, err := http.Get(base + "/health")
	if err != nil {
		t.Fatalf("health request failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 health, got %d", resp.StatusCode)
	}
	payload := Event{
		Version:   EventSchemaVersion,
		EventID:   "evt-1",
		Type:      "model_response",
		SessionID: "sess",
		ModuleID:  "mod",
		Workflow:  "wf",
	}
	buf, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	resp, err = http.Post(base+"/events", "application/json", bytes.NewReader(buf))
	if err != nil {
		t.Fatalf("post event: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", resp.StatusCode)
	}
	select {
	case evt := <-recorded:
		if !evt.ServerTime.Equal(fixed) {
			t.Fatalf("expected server time %s, got %s", fixed, evt.ServerTime)
		}
	default:
		t.Fatalf("event not forwarded to processor")
	}
}

func TestServerEnforcesPayloadLimit(t *testing.T) {
	t.Parallel()
	settings := Settings{Enabled: true, Host: "127.0.0.1", Port: 0, MaxBodyBytes: 64, ReadTimeout: time.Second, WriteTimeout: time.Second, IdleTimeout: time.Second}
	srv := NewServer(settings)
	t.Cleanup(func() {
		_ = srv.Shutdown(context.Background())
	})
	if err := srv.Start(context.Background()); err != nil {
		t.Fatalf("start server: %v", err)
	}
	base := srv.BaseURL()
	tooLarge := bytes.Repeat([]byte("a"), 512)
	payload := map[string]any{
		"version":    EventSchemaVersion,
		"event_id":   "evt",
		"type":       "model_response",
		"session_id": "sess",
		"module_id":  "mod",
		"workflow":   "wf",
		"payload":    string(tooLarge),
	}
	buf, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp, err := http.Post(base+"/events", "application/json", bytes.NewReader(buf))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d", resp.StatusCode)
	}
}
