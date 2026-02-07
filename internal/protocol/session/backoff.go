package session

import (
	"math"
	"math/rand"
	"time"
)

// NextBackoffDelay returns the retry delay for attempt N (1-based).
func NextBackoffDelay(cfg BackoffConfig, attempt int, rng *rand.Rand) time.Duration {
	if attempt <= 1 {
		return cfg.InitialDelay
	}
	if cfg.InitialDelay <= 0 {
		return 0
	}
	if cfg.Multiplier < 1.0 {
		cfg.Multiplier = 1.0
	}
	delay := float64(cfg.InitialDelay) * math.Pow(cfg.Multiplier, float64(attempt-1))
	if cfg.MaxDelay > 0 && delay > float64(cfg.MaxDelay) {
		delay = float64(cfg.MaxDelay)
	}
	if cfg.Jitter {
		f := 0.5
		if rng != nil {
			f = 0.5 + rng.Float64()
		}
		delay = delay * f
	}
	return time.Duration(delay)
}
