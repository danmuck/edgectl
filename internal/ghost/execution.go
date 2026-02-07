package ghost

import (
	"maps"
	"strings"
)

type ExecutionPhase string

const (
	ExecutionAccepted ExecutionPhase = "accepted"
)

// ExecutionState tracks command intake state keyed by command/message ids.
type ExecutionState struct {
	MessageID    uint64
	CommandID    string
	ExecutionID  string
	IntentID     string
	GhostID      string
	SeedSelector string
	Operation    string
	Args         map[string]string
	Phase        ExecutionPhase
}

func newExecutionState(cmd CommandEnv) ExecutionState {
	return ExecutionState{
		MessageID:    cmd.MessageID,
		CommandID:    strings.TrimSpace(cmd.CommandID),
		ExecutionID:  executionIDForCommand(cmd.CommandID),
		IntentID:     strings.TrimSpace(cmd.IntentID),
		GhostID:      strings.TrimSpace(cmd.GhostID),
		SeedSelector: strings.TrimSpace(cmd.SeedSelector),
		Operation:    strings.TrimSpace(cmd.Operation),
		Args:         cloneArgs(cmd.Args),
		Phase:        ExecutionAccepted,
	}
}

func executionIDForCommand(commandID string) string {
	return "exec." + strings.TrimSpace(commandID)
}

func cloneArgs(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	maps.Copy(out, in)
	return out
}
