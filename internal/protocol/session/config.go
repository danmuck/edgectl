package session

import "time"

// BackoffConfig defines retry backoff behavior.
type BackoffConfig struct {
	InitialDelay time.Duration
	Multiplier   float64
	MaxDelay     time.Duration
	Jitter       bool
}

// Config defines transport/session reliability defaults.
type Config struct {
	ConnectTimeout    time.Duration
	HandshakeTimeout  time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	HeartbeatInterval time.Duration
	SessionDeadAfter  time.Duration
	AckTimeout        time.Duration
	Backoff           BackoffConfig
}

// DefaultConfig returns contract-aligned defaults from reliability.toml.
func DefaultConfig() Config {
	return Config{
		ConnectTimeout:    5 * time.Second,
		HandshakeTimeout:  5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		HeartbeatInterval: 5 * time.Second,
		SessionDeadAfter:  15 * time.Second,
		AckTimeout:        20 * time.Second,
		Backoff: BackoffConfig{
			InitialDelay: 250 * time.Millisecond,
			Multiplier:   2.0,
			MaxDelay:     5 * time.Second,
			Jitter:       true,
		},
	}
}
