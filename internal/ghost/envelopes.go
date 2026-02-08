package ghost

import (
	"fmt"
	"strings"
)

const (
	OutcomeSuccess = "success"
	OutcomeError   = "error"

	SeedStatusOK    = "ok"
	SeedStatusError = "error"
)

// Ghost normalized seed.execute request envelope for seed dispatch.
type SeedExecuteEnv struct {
	ExecutionID string
	CommandID   string
	SeedID      string
	Operation   string
	Args        map[string]string
}

// Ghost seed.execute validator for required envelope fields.
func (e SeedExecuteEnv) Validate() error {
	if strings.TrimSpace(e.ExecutionID) == "" {
		return fmt.Errorf("%w: missing execution_id", ErrInvalidCommandEnv)
	}
	if strings.TrimSpace(e.CommandID) == "" {
		return fmt.Errorf("%w: missing command_id", ErrInvalidCommandEnv)
	}
	if strings.TrimSpace(e.SeedID) == "" {
		return fmt.Errorf("%w: missing seed_id", ErrInvalidCommandEnv)
	}
	if strings.TrimSpace(e.Operation) == "" {
		return fmt.Errorf("%w: missing operation", ErrInvalidCommandEnv)
	}
	return nil
}

// Ghost normalized seed.result envelope emitted from seed execution.
type SeedResultEnv struct {
	ExecutionID string
	SeedID      string
	Status      string
	Stdout      []byte
	Stderr      []byte
	ExitCode    int32
}

// Ghost seed.result validator for required envelope fields.
func (e SeedResultEnv) Validate() error {
	if strings.TrimSpace(e.ExecutionID) == "" {
		return fmt.Errorf("%w: missing execution_id", ErrInvalidCommandEnv)
	}
	if strings.TrimSpace(e.SeedID) == "" {
		return fmt.Errorf("%w: missing seed_id", ErrInvalidCommandEnv)
	}
	if strings.TrimSpace(e.Status) == "" {
		return fmt.Errorf("%w: missing status", ErrInvalidCommandEnv)
	}
	return nil
}

// Ghost terminal event envelope emitted after command execution closure.
type EventEnv struct {
	EventID     string
	CommandID   string
	IntentID    string
	GhostID     string
	SeedID      string
	Outcome     string
	TimestampMS uint64
}

// Ghost event validator for required terminal envelope fields.
func (e EventEnv) Validate() error {
	if strings.TrimSpace(e.EventID) == "" {
		return fmt.Errorf("%w: missing event_id", ErrInvalidCommandEnv)
	}
	if strings.TrimSpace(e.CommandID) == "" {
		return fmt.Errorf("%w: missing command_id", ErrInvalidCommandEnv)
	}
	if strings.TrimSpace(e.IntentID) == "" {
		return fmt.Errorf("%w: missing intent_id", ErrInvalidCommandEnv)
	}
	if strings.TrimSpace(e.GhostID) == "" {
		return fmt.Errorf("%w: missing ghost_id", ErrInvalidCommandEnv)
	}
	if strings.TrimSpace(e.SeedID) == "" {
		return fmt.Errorf("%w: missing seed_id", ErrInvalidCommandEnv)
	}
	if e.Outcome != OutcomeSuccess && e.Outcome != OutcomeError {
		return fmt.Errorf("%w: invalid outcome", ErrInvalidCommandEnv)
	}
	if e.TimestampMS == 0 {
		return fmt.Errorf("%w: missing timestamp_ms", ErrInvalidCommandEnv)
	}
	return nil
}

// Ghost deterministic event_id builder derived from command_id.
func eventIDForCommand(commandID string) string {
	return "evt." + strings.TrimSpace(commandID)
}
