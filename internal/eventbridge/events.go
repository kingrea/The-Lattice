package eventbridge

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	// ProtocolVersion identifies the bridge contract version exposed via /health.
	ProtocolVersion = "1.0.0"
	// EventSchemaVersion is the currently supported inbound event version.
	EventSchemaVersion = 1
)

// Event captures a single notification emitted by the OpenCode plugin.
type Event struct {
	Version    int             `json:"version"`
	EventID    string          `json:"event_id"`
	Sequence   int64           `json:"sequence"`
	Type       string          `json:"type"`
	ClientTime time.Time       `json:"client_time"`
	ServerTime time.Time       `json:"server_time"`
	SessionID  string          `json:"session_id"`
	ModuleID   string          `json:"module_id"`
	Workflow   string          `json:"workflow"`
	Payload    json.RawMessage `json:"payload"`
}

// Normalize applies defaults and canonical formatting before validation.
func (e *Event) Normalize() {
	if e == nil {
		return
	}
	if e.Version == 0 {
		e.Version = EventSchemaVersion
	}
	e.EventID = strings.TrimSpace(e.EventID)
	e.Type = strings.TrimSpace(e.Type)
	e.SessionID = strings.TrimSpace(e.SessionID)
	e.ModuleID = strings.TrimSpace(e.ModuleID)
	e.Workflow = strings.TrimSpace(e.Workflow)
}

// StampServerTime overwrites ServerTime with the supplied clock reading (UTC).
func (e *Event) StampServerTime(now time.Time) {
	if e == nil {
		return
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	e.ServerTime = now.UTC()
}

// Validate enforces baseline schema requirements for incoming events.
func (e Event) Validate() error {
	if e.Version != EventSchemaVersion {
		return fmt.Errorf("version %d not supported", e.Version)
	}
	if e.EventID == "" {
		return errors.New("event_id is required")
	}
	if e.Type == "" {
		return errors.New("type is required")
	}
	if e.SessionID == "" {
		return errors.New("session_id is required")
	}
	if e.ModuleID == "" {
		return errors.New("module_id is required")
	}
	if e.Workflow == "" {
		return errors.New("workflow is required")
	}
	return nil
}

// EventProcessor consumes validated events.
type EventProcessor interface {
	HandleEvent(Event) error
}

// EventProcessorFunc adapts a function into an EventProcessor.
type EventProcessorFunc func(Event) error

// HandleEvent executes f(e).
func (f EventProcessorFunc) HandleEvent(e Event) error {
	if f == nil {
		return nil
	}
	return f(e)
}

// Logger records bridge status information. It matches logging.Logger's signature.
type Logger interface {
	Printf(format string, args ...any)
}

type healthResponse struct {
	Status        string `json:"status"`
	Version       string `json:"version"`
	RouterReady   bool   `json:"router_ready"`
	UptimeSeconds int64  `json:"uptime_seconds"`
}

type eventResponse struct {
	Status     string    `json:"status"`
	ServerTime time.Time `json:"server_time"`
}
