package session

import "time"

// Session transport enforcement level selector.
type SecurityMode string

const (
	SecurityModeDevelopment SecurityMode = "development"
	SecurityModeProduction  SecurityMode = "production"
)

// Session TLS/mTLS transport settings.
type TLSConfig struct {
	Enabled            bool
	Mutual             bool
	CertFile           string
	KeyFile            string
	CAFile             string
	ServerName         string
	InsecureSkipVerify bool
}

// Session retry backoff behavior settings.
type BackoffConfig struct {
	InitialDelay time.Duration
	Multiplier   float64
	MaxDelay     time.Duration
	Jitter       bool
}

// Session transport/reliability configuration.
type Config struct {
	ConnectTimeout    time.Duration
	HandshakeTimeout  time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	HeartbeatInterval time.Duration
	SessionDeadAfter  time.Duration
	AckTimeout        time.Duration
	SecurityMode      SecurityMode
	TLS               TLSConfig
	Backoff           BackoffConfig
}

// Session default config aligned to reliability contract defaults.
func DefaultConfig() Config {
	return Config{
		ConnectTimeout:    5 * time.Second,
		HandshakeTimeout:  5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		HeartbeatInterval: 5 * time.Second,
		SessionDeadAfter:  15 * time.Second,
		AckTimeout:        20 * time.Second,
		SecurityMode:      SecurityModeDevelopment,
		Backoff: BackoffConfig{
			InitialDelay: 250 * time.Millisecond,
			Multiplier:   2.0,
			MaxDelay:     5 * time.Second,
			Jitter:       true,
		},
	}
}

// Session config merger that fills unset fields while preserving overrides.
func (c Config) WithDefaults() Config {
	d := DefaultConfig()

	if c.ConnectTimeout <= 0 {
		c.ConnectTimeout = d.ConnectTimeout
	}
	if c.HandshakeTimeout <= 0 {
		c.HandshakeTimeout = d.HandshakeTimeout
	}
	if c.ReadTimeout <= 0 {
		c.ReadTimeout = d.ReadTimeout
	}
	if c.WriteTimeout <= 0 {
		c.WriteTimeout = d.WriteTimeout
	}
	if c.HeartbeatInterval <= 0 {
		c.HeartbeatInterval = d.HeartbeatInterval
	}
	if c.SessionDeadAfter <= 0 {
		c.SessionDeadAfter = d.SessionDeadAfter
	}
	if c.AckTimeout <= 0 {
		c.AckTimeout = d.AckTimeout
	}
	if c.Backoff.InitialDelay <= 0 {
		c.Backoff.InitialDelay = d.Backoff.InitialDelay
	}
	if c.Backoff.Multiplier <= 0 {
		c.Backoff.Multiplier = d.Backoff.Multiplier
	}
	if c.Backoff.MaxDelay <= 0 {
		c.Backoff.MaxDelay = d.Backoff.MaxDelay
	}
	c.SecurityMode = NormalizeSecurityMode(c.SecurityMode)
	return c
}
