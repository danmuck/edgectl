package session

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrInvalidSecurityMode     = errors.New("session: invalid security mode")
	ErrTLSRequired             = errors.New("session: tls required")
	ErrMTLSRequired            = errors.New("session: mtls required")
	ErrTLSCertFileRequired     = errors.New("session: tls cert file required")
	ErrTLSKeyFileRequired      = errors.New("session: tls key file required")
	ErrTLSCAFileRequired       = errors.New("session: tls ca file required")
	ErrTLSInsecureSkipNotAllow = errors.New("session: insecure skip verify not allowed")
)

func NormalizeSecurityMode(mode SecurityMode) SecurityMode {
	if strings.TrimSpace(string(mode)) == "" {
		return SecurityModeDevelopment
	}
	return SecurityMode(strings.ToLower(strings.TrimSpace(string(mode))))
}

func (c Config) ValidateClientTransport() error {
	mode := NormalizeSecurityMode(c.SecurityMode)
	switch mode {
	case SecurityModeDevelopment, SecurityModeProduction:
	default:
		return fmt.Errorf("%w: %q", ErrInvalidSecurityMode, c.SecurityMode)
	}

	if mode == SecurityModeProduction {
		if !c.TLS.Enabled {
			return ErrTLSRequired
		}
		if !c.TLS.Mutual {
			return ErrMTLSRequired
		}
		if c.TLS.InsecureSkipVerify {
			return ErrTLSInsecureSkipNotAllow
		}
	}
	if c.TLS.Mutual && !c.TLS.Enabled {
		return ErrTLSRequired
	}
	if c.TLS.Enabled && strings.TrimSpace(c.TLS.CAFile) == "" && !c.TLS.InsecureSkipVerify {
		return ErrTLSCAFileRequired
	}
	if c.TLS.Mutual {
		if strings.TrimSpace(c.TLS.CertFile) == "" {
			return ErrTLSCertFileRequired
		}
		if strings.TrimSpace(c.TLS.KeyFile) == "" {
			return ErrTLSKeyFileRequired
		}
	}
	return nil
}

func (c Config) ValidateServerTransport() error {
	mode := NormalizeSecurityMode(c.SecurityMode)
	switch mode {
	case SecurityModeDevelopment, SecurityModeProduction:
	default:
		return fmt.Errorf("%w: %q", ErrInvalidSecurityMode, c.SecurityMode)
	}

	if mode == SecurityModeProduction {
		if !c.TLS.Enabled {
			return ErrTLSRequired
		}
		if !c.TLS.Mutual {
			return ErrMTLSRequired
		}
	}
	if c.TLS.Mutual && !c.TLS.Enabled {
		return ErrTLSRequired
	}
	if c.TLS.Enabled {
		if strings.TrimSpace(c.TLS.CertFile) == "" {
			return ErrTLSCertFileRequired
		}
		if strings.TrimSpace(c.TLS.KeyFile) == "" {
			return ErrTLSKeyFileRequired
		}
	}
	if c.TLS.Mutual && strings.TrimSpace(c.TLS.CAFile) == "" {
		return ErrTLSCAFileRequired
	}
	return nil
}
