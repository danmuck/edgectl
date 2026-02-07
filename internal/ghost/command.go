package ghost

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrInvalidCommandEnv     = errors.New("ghost: invalid command envelope")
	ErrNotRadiating          = errors.New("ghost: not radiating")
	ErrCommandTargetMismatch = errors.New("ghost: command target mismatch")
	ErrDuplicateCommandID    = errors.New("ghost: duplicate command_id")
	ErrDuplicateMessageID    = errors.New("ghost: duplicate message_id")
)

// CommandEnv is the Ghost input boundary envelope from Mirage.
type CommandEnv struct {
	MessageID    uint64
	CommandID    string
	IntentID     string
	GhostID      string
	SeedSelector string
	Operation    string
	Args         map[string]string
}

// Validate enforces required command envelope fields.
func (e CommandEnv) Validate() error {
	if e.MessageID == 0 {
		return wrapInvalidCommand("missing message_id")
	}
	if strings.TrimSpace(e.CommandID) == "" {
		return wrapInvalidCommand("missing command_id")
	}
	if strings.TrimSpace(e.IntentID) == "" {
		return wrapInvalidCommand("missing intent_id")
	}
	if strings.TrimSpace(e.GhostID) == "" {
		return wrapInvalidCommand("missing ghost_id")
	}
	if strings.TrimSpace(e.SeedSelector) == "" {
		return wrapInvalidCommand("missing seed_selector")
	}
	if strings.TrimSpace(e.Operation) == "" {
		return wrapInvalidCommand("missing operation")
	}
	return nil
}

func wrapInvalidCommand(reason string) error {
	return fmt.Errorf("%w: %s", ErrInvalidCommandEnv, reason)
}
