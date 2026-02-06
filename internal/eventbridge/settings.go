package eventbridge

import (
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/kingrea/The-Lattice/internal/config"
)

const (
	// DefaultHost is the loopback interface used when no host override is provided.
	DefaultHost = "127.0.0.1"
	// DefaultPort is the default TCP port for the bridge server.
	DefaultPort = 8765
	// DefaultMaxBodyBytes limits request payloads to 1 MB.
	DefaultMaxBodyBytes int64 = 1 << 20
	// DefaultReadTimeout guards hung clients.
	DefaultReadTimeout = 15 * time.Second
	// DefaultWriteTimeout bounds handler writes.
	DefaultWriteTimeout = 15 * time.Second
	// DefaultIdleTimeout bounds keep-alive connections.
	DefaultIdleTimeout = 60 * time.Second
)

// Settings captures runtime configuration for the HTTP event bridge server.
type Settings struct {
	Enabled      bool
	Host         string
	Port         int
	MaxBodyBytes int64
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
}

// SettingsFromConfig builds Settings using the project's .lattice config and environment overrides.
func SettingsFromConfig(cfg *config.Config) Settings {
	settings := Settings{
		Enabled:      true,
		Host:         DefaultHost,
		Port:         DefaultPort,
		MaxBodyBytes: DefaultMaxBodyBytes,
		ReadTimeout:  DefaultReadTimeout,
		WriteTimeout: DefaultWriteTimeout,
		IdleTimeout:  DefaultIdleTimeout,
	}
	if cfg != nil {
		raw := cfg.Project.EventBridge
		if raw.Enabled != nil {
			settings.Enabled = *raw.Enabled
		}
		if host := strings.TrimSpace(raw.Host); host != "" {
			settings.Host = host
		}
		if isValidPort(raw.Port) {
			settings.Port = raw.Port
		}
	}
	settings.applyEnvOverrides()
	settings.normalize()
	return settings
}

func (s *Settings) applyEnvOverrides() {
	if s == nil {
		return
	}
	if value := strings.TrimSpace(os.Getenv("LATTICE_BRIDGE_ENABLED")); value != "" {
		if enabled, err := strconv.ParseBool(value); err == nil {
			s.Enabled = enabled
		}
	}
	if host := strings.TrimSpace(os.Getenv("LATTICE_BRIDGE_HOST")); host != "" {
		s.Host = host
	}
	if port := strings.TrimSpace(os.Getenv("LATTICE_BRIDGE_PORT")); port != "" {
		if parsed, err := strconv.Atoi(port); err == nil && isValidPort(parsed) {
			s.Port = parsed
		}
	}
}

func (s *Settings) normalize() {
	if s == nil {
		return
	}
	s.Host = strings.TrimSpace(s.Host)
	if s.Host == "" {
		s.Host = DefaultHost
	}
	if !isValidPort(s.Port) {
		s.Port = DefaultPort
	}
	if s.MaxBodyBytes <= 0 {
		s.MaxBodyBytes = DefaultMaxBodyBytes
	}
	if s.ReadTimeout <= 0 {
		s.ReadTimeout = DefaultReadTimeout
	}
	if s.WriteTimeout <= 0 {
		s.WriteTimeout = DefaultWriteTimeout
	}
	if s.IdleTimeout <= 0 {
		s.IdleTimeout = DefaultIdleTimeout
	}
}

// Address returns the TCP bind address in host:port form.
func (s Settings) Address() string {
	return net.JoinHostPort(s.Host, strconv.Itoa(s.Port))
}

// URL returns the HTTP base URL for the server.
func (s Settings) URL() string {
	return "http://" + s.Address()
}

func isValidPort(port int) bool {
	return port > 0 && port <= 65535
}
