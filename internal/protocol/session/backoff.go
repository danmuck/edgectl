package session

import (
	"math"
	"math/rand"
	"time"
)

// NextBackoffDelay returns the retry delay for attempt N (1-based) per reliability.toml [backoff]:
// exponential from InitialDelay by Multiplier, jittered to [0.5, 1.5) of the base when enabled,
// and always bounded by MaxDelay after jitter. A nil rng uses the shared math/rand source.
func NextBackoffDelay(cfg BackoffConfig, attempt int, rng *rand.Rand) time.Duration {
	if cfg.InitialDelay <= 0 {
		return 0
	}
	if attempt < 1 {
		attempt = 1
	}
	if cfg.Multiplier < 1.0 {
		cfg.Multiplier = 1.0
	}
	delay := float64(cfg.InitialDelay) * math.Pow(cfg.Multiplier, float64(attempt-1))
	if cfg.Jitter {
		var f float64
		if rng != nil {
			f = rng.Float64()
		} else {
			f = rand.Float64()
		}
		delay *= 0.5 + f
	}
	if cfg.MaxDelay > 0 && delay > float64(cfg.MaxDelay) {
		delay = float64(cfg.MaxDelay)
	}
	return time.Duration(delay)
}
