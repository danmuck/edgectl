package ghost

import (
	"maps"
	"strings"
)

// Ghost in-memory command lifecycle phase marker.
type ExecutionPhase string

const (
	ExecutionAccepted ExecutionPhase = "accepted"
	ExecutionComplete ExecutionPhase = "complete"
)

// Ghost in-memory execution record keyed by command and message ids.
type ExecutionState struct {
	MessageID    uint64
	CommandID    string
	ExecutionID  string
	IntentID     string
	GhostID      string
	SeedSelector string
	Operation    string
	Args         map[string]string
	SeedExecute  SeedExecuteEnv
	SeedResult   SeedResultEnv
	Event        EventEnv
	Outcome      string
	Phase        ExecutionPhase
}

// Ghost execution-state constructor from accepted command input.
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

// Ghost deterministic execution_id builder derived from command_id.
func executionIDForCommand(commandID string) string {
	return "exec." + strings.TrimSpace(commandID)
}

// Ghost helper that returns a defensive copy of command argument maps.
func cloneArgs(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	maps.Copy(out, in)
	return out
}
