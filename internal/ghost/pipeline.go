package ghost

import (
	"fmt"
	"strings"
	"time"

	"github.com/danmuck/edgectl/internal/seeds"
	logs "github.com/danmuck/smplog"
)

const unknownSeedExitCode int32 = 127

// Ghost full command pipeline: boundary accept -> seed.execute -> seed.result -> event.
func (s *Server) HandleCommandAndExecute(cmd CommandEnv) (EventEnv, error) {
	logs.Debugf(
		"ghost.Server.HandleCommandAndExecute message_id=%d command_id=%q",
		cmd.MessageID,
		cmd.CommandID,
	)
	state, err := s.HandleCommand(cmd)
	if err != nil {
		return EventEnv{}, err
	}

	seedExec := buildSeedExecute(state)
	if err := seedExec.Validate(); err != nil {
		return EventEnv{}, err
	}

	seedResult := s.executeSeed(seedExec)
	if err := seedResult.Validate(); err != nil {
		return EventEnv{}, err
	}

	event := buildEvent(state, seedResult)
	if err := event.Validate(); err != nil {
		return EventEnv{}, err
	}

	s.completeExecution(state.ExecutionID, seedExec, seedResult, event)
	logs.Infof(
		"ghost.Server.HandleCommandAndExecute complete command_id=%q execution_id=%q outcome=%q",
		state.CommandID,
		state.ExecutionID,
		event.Outcome,
	)
	return event, nil
}

// Ghost mapping from accepted command state to seed.execute payload.
func buildSeedExecute(state ExecutionState) SeedExecuteEnv {
	return SeedExecuteEnv{
		ExecutionID: state.ExecutionID,
		CommandID:   state.CommandID,
		SeedID:      state.SeedSelector,
		Operation:   state.Operation,
		Args:        cloneArgs(state.Args),
	}
}

// Ghost seed dispatch helper: resolve target seed and invoke requested operation.
func (s *Server) executeSeed(exec SeedExecuteEnv) SeedResultEnv {
	s.mu.RLock()
	reg := s.registry
	s.mu.RUnlock()

	if reg == nil {
		return errorSeedResult(exec, "seed registry unavailable", 1)
	}

	seed, ok := reg.Resolve(exec.SeedID)
	if !ok {
		return errorSeedResult(exec, fmt.Sprintf("unknown seed: %s", exec.SeedID), unknownSeedExitCode)
	}

	meta := seed.Metadata()
	seedID := strings.TrimSpace(meta.ID)
	if seedID == "" {
		seedID = exec.SeedID
	}
	exec.SeedID = seedID

	result, err := seed.Execute(exec.Operation, cloneArgs(exec.Args))
	return normalizeSeedResult(exec, result, err)
}

// Ghost normalization of seed output/error into canonical seed.result fields.
func normalizeSeedResult(exec SeedExecuteEnv, result seeds.SeedResult, execErr error) SeedResultEnv {
	status := strings.TrimSpace(result.Status)
	if status == "" {
		if execErr != nil || result.ExitCode != 0 {
			status = SeedStatusError
		} else {
			status = SeedStatusOK
		}
	}

	stdout := cloneBytes(result.Stdout)
	stderr := cloneBytes(result.Stderr)
	exitCode := result.ExitCode

	if execErr != nil {
		status = SeedStatusError
		if exitCode == 0 {
			exitCode = 1
		}
		if len(stderr) == 0 {
			stderr = []byte(execErr.Error() + "\n")
		}
	}

	return SeedResultEnv{
		ExecutionID: exec.ExecutionID,
		SeedID:      exec.SeedID,
		Status:      status,
		Stdout:      stdout,
		Stderr:      stderr,
		ExitCode:    exitCode,
	}
}

// Ghost deterministic seed.result builder for dispatch-time failures.
func errorSeedResult(exec SeedExecuteEnv, reason string, exitCode int32) SeedResultEnv {
	return SeedResultEnv{
		ExecutionID: exec.ExecutionID,
		SeedID:      exec.SeedID,
		Status:      SeedStatusError,
		Stdout:      nil,
		Stderr:      []byte(reason + "\n"),
		ExitCode:    exitCode,
	}
}

// Ghost event envelope builder from terminal seed result state.
func buildEvent(state ExecutionState, seedResult SeedResultEnv) EventEnv {
	outcome := OutcomeError
	if seedResult.Status == SeedStatusOK && seedResult.ExitCode == 0 {
		outcome = OutcomeSuccess
	}

	return EventEnv{
		EventID:     eventIDForCommand(state.CommandID),
		CommandID:   state.CommandID,
		IntentID:    state.IntentID,
		GhostID:     state.GhostID,
		SeedID:      seedResult.SeedID,
		Outcome:     outcome,
		TimestampMS: uint64(time.Now().UnixMilli()),
	}
}

// Ghost helper that returns a defensive copy of byte slices.
func cloneBytes(in []byte) []byte {
	if len(in) == 0 {
		return nil
	}
	out := make([]byte, len(in))
	copy(out, in)
	return out
}
