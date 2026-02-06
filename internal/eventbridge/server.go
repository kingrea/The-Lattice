package eventbridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"
)

// ServerStatus reports runtime lifecycle states for the HTTP server.
type ServerStatus string

const (
	StatusStarting ServerStatus = "starting"
	StatusReady    ServerStatus = "ready"
	StatusDraining ServerStatus = "draining"
)

var errServerDisabled = errors.New("eventbridge: server disabled")

// Server wraps the HTTP listener and handlers backing the event bridge.
type Server struct {
	settings  Settings
	processor EventProcessor
	logger    Logger
	clock     func() time.Time

	mu          sync.RWMutex
	server      *http.Server
	listener    net.Listener
	status      ServerStatus
	startTime   time.Time
	routerReady bool
}

// Option customizes server construction.
type Option func(*Server)

// WithProcessor overrides the default no-op event processor.
func WithProcessor(p EventProcessor) Option {
	return func(s *Server) {
		if p != nil {
			s.processor = p
		}
	}
}

// WithLogger overrides the default no-op logger.
func WithLogger(l Logger) Option {
	return func(s *Server) {
		if l != nil {
			s.logger = l
		}
	}
}

// WithClock allows tests to control timestamps.
func WithClock(clock func() time.Time) Option {
	return func(s *Server) {
		if clock != nil {
			s.clock = clock
		}
	}
}

// NewServer prepares a bridge server using the provided settings.
func NewServer(settings Settings, opts ...Option) *Server {
	s := &Server{
		settings:  settings,
		processor: EventProcessorFunc(func(Event) error { return nil }),
		logger:    nopLogger{},
		clock:     func() time.Time { return time.Now().UTC() },
		status:    StatusStarting,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(s)
		}
	}
	return s
}

// Start binds the TCP listener and begins serving HTTP traffic.
func (s *Server) Start(ctx context.Context) error {
	if s == nil {
		return fmt.Errorf("eventbridge: server is nil")
	}
	if !s.settings.Enabled {
		return errServerDisabled
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener != nil {
		return fmt.Errorf("eventbridge: server already started")
	}
	addr := s.settings.Address()
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("eventbridge: listen %s: %w", addr, err)
	}
	s.listener = listener
	s.routerReady = true
	s.startTime = s.clock()
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/events", s.handleEvents)
	server := &http.Server{
		Handler:      mux,
		ReadTimeout:  s.settings.ReadTimeout,
		WriteTimeout: s.settings.WriteTimeout,
		IdleTimeout:  s.settings.IdleTimeout,
	}
	if ctx != nil {
		server.BaseContext = func(net.Listener) context.Context { return ctx }
	}
	s.server = server
	s.status = StatusReady
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Printf("eventbridge: serve error: %v", err)
		}
	}()
	s.logger.Printf("eventbridge: listening on %s", listener.Addr().String())
	return nil
}

// Shutdown stops accepting new connections and waits for in-flight requests to exit.
func (s *Server) Shutdown(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener == nil || s.server == nil {
		return nil
	}
	s.status = StatusDraining
	deadline := ctx
	if deadline == nil {
		var cancel context.CancelFunc
		deadline, cancel = context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
	}
	if err := s.server.Shutdown(deadline); err != nil {
		return err
	}
	s.listener = nil
	s.server = nil
	return nil
}

// Addr returns the bound TCP address once the server has started.
func (s *Server) Addr() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}

// BaseURL returns the HTTP base URL (scheme + host:port) for the running server.
func (s *Server) BaseURL() string {
	addr := s.Addr()
	if addr == "" {
		return s.settings.URL()
	}
	return "http://" + addr
}

// Status reports the server's lifecycle state.
func (s *Server) Status() ServerStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.status
}

func (s *Server) now() time.Time {
	if s.clock == nil {
		return time.Now().UTC()
	}
	return s.clock().UTC()
}

func (s *Server) uptimeSeconds() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.startTime.IsZero() {
		return 0
	}
	return int64(time.Since(s.startTime).Seconds())
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", fmt.Sprintf("%s, %s", http.MethodGet, http.MethodHead))
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	resp := healthResponse{
		Status:        string(s.Status()),
		Version:       ProtocolVersion,
		RouterReady:   s.routerReady,
		UptimeSeconds: s.uptimeSeconds(),
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if r.Body == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "empty body"})
		return
	}
	reader := http.MaxBytesReader(w, r.Body, s.settings.MaxBodyBytes)
	defer reader.Close()
	body, err := io.ReadAll(reader)
	if err != nil {
		if errors.Is(err, http.ErrBodyReadAfterClose) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "request body closed"})
			return
		}
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "payload exceeds limit"})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unable to read body"})
		return
	}
	var evt Event
	if err := json.Unmarshal(body, &evt); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	evt.Normalize()
	if err := evt.Validate(); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	evt.StampServerTime(s.now())
	if err := s.processor.HandleEvent(evt); err != nil {
		s.logger.Printf("eventbridge: processor error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "event processing failed"})
		return
	}
	writeJSON(w, http.StatusAccepted, eventResponse{Status: "accepted", ServerTime: evt.ServerTime})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

type nopLogger struct{}

func (nopLogger) Printf(string, ...any) {}
