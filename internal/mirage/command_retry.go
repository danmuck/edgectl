package mirage

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/danmuck/edgectl/internal/protocol/session"
	logs "github.com/danmuck/smplog"
)

// CommandState is the per-command lifecycle state Mirage records in observed state.
// It is not a wire value; reports keep completion_state in_progress|satisfied|failed.
type CommandState string

const (
	// Terminal event observed with outcome=success.
	CommandComplete CommandState = "complete"
	// Terminal event observed with outcome=error.
	CommandFailed CommandState = "failed"
	// Caller deadline expired before a terminal event and before attempts were exhausted.
	CommandTimedOut CommandState = "timed_out"
	// Every dispatch attempt failed without a terminal event.
	CommandRetryExhausted CommandState = "retry_exhausted"
)

// DefaultCommandMaxAttempts bounds dispatch attempts per command when no terminal event is observed.
const DefaultCommandMaxAttempts = 3

// CommandStatus is the recorded lifecycle outcome for one planned command.
type CommandStatus struct {
	State     CommandState
	Attempts  int
	LastError string
	UpdatedAt time.Time
}

// Terminal reports whether no further dispatch will be attempted for this command.
func (s CommandStatus) Terminal() bool {
	switch s.State {
	case CommandComplete, CommandFailed, CommandTimedOut, CommandRetryExhausted:
		return true
	}
	return false
}

// CommandRetryPolicy bounds command dispatch retries (reliability.toml retry_policy.command, [backoff]).
type CommandRetryPolicy struct {
	MaxAttempts int
	Backoff     session.BackoffConfig
}

// DefaultCommandRetryPolicy uses DefaultCommandMaxAttempts and contract backoff.
func DefaultCommandRetryPolicy() CommandRetryPolicy {
	return CommandRetryPolicy{
		MaxAttempts: DefaultCommandMaxAttempts,
		Backoff:     session.DefaultConfig().Backoff,
	}
}

// Fills unset fields from defaults while preserving overrides.
func (p CommandRetryPolicy) withDefaults() CommandRetryPolicy {
	d := DefaultCommandRetryPolicy()
	if p.MaxAttempts <= 0 {
		p.MaxAttempts = d.MaxAttempts
	}
	if p.Backoff.InitialDelay <= 0 {
		p.Backoff = d.Backoff
	}
	return p
}

// SetCommandRetryPolicy replaces the dispatch retry policy; unset fields fall back to defaults.
func (o *Orchestrator) SetCommandRetryPolicy(p CommandRetryPolicy) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.retry = p.withDefaults()
}

// Status derived from a terminal event outcome.
func statusFromEvent(event session.Event, attempts int, at time.Time) CommandStatus {
	state := CommandComplete
	if strings.TrimSpace(event.Outcome) != OutcomeSuccess {
		state = CommandFailed
	}
	return CommandStatus{State: state, Attempts: attempts, UpdatedAt: at}
}

// Default sleeper honoring context cancellation.
func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// dispatchResult is the outcome of dispatchWithRetry.
type dispatchResult struct {
	// Event is set when a terminal event was returned by the executor.
	Event    session.Event
	HasEvent bool
	// Observed is true when a terminal event arrived through another path (session) mid-retry.
	Observed bool
	Status   CommandStatus
}

// dispatchWithRetry executes one command, retrying with contract backoff while no terminal event
// is observed (reliability.toml retry_policy.command). The command_id is reused on every attempt so
// Ghost replays instead of re-executing. Stops on: executor event, session-observed event, caller
// deadline (timed_out), or MaxAttempts failures (retry_exhausted).
func (o *Orchestrator) dispatchWithRetry(
	ctx context.Context,
	intentID string,
	exec CommandExecutor,
	cmd session.Command,
) dispatchResult {
	o.mu.RLock()
	policy := o.retry
	sleep := o.sleep
	now := o.now
	o.mu.RUnlock()

	var lastErr error
	attempt := 0
	for attempt < policy.MaxAttempts {
		attempt++
		event, err := exec.ExecuteCommand(ctx, cmd)
		if err == nil {
			return dispatchResult{Event: event, HasEvent: true, Status: statusFromEvent(event, attempt, now())}
		}
		lastErr = err
		logs.Warnf(
			"mirage.command.retry component=mirage intent_id=%s command_id=%s ghost_id=%s attempt=%d max_attempts=%d err=%q",
			intentID, cmd.CommandID, cmd.GhostID, attempt, policy.MaxAttempts, err.Error(),
		)
		if ctx.Err() != nil {
			return o.nonEventTerminal(intentID, cmd, CommandTimedOut, attempt, lastErr)
		}
		if attempt >= policy.MaxAttempts {
			break
		}
		if err := sleep(ctx, session.NextBackoffDelay(policy.Backoff, attempt, nil)); err != nil {
			return o.nonEventTerminal(intentID, cmd, CommandTimedOut, attempt, lastErr)
		}
		// Retry only when no terminal event has been observed for this command.
		if o.commandObserved(intentID, cmd.CommandID) {
			logs.Infof(
				"mirage.command.retry.stop component=mirage intent_id=%s command_id=%s reason=terminal_event_observed attempt=%d",
				intentID, cmd.CommandID, attempt,
			)
			return dispatchResult{Observed: true}
		}
	}
	return o.nonEventTerminal(intentID, cmd, CommandRetryExhausted, attempt, lastErr)
}

// Terminal status for a command that ended without any terminal event.
func (o *Orchestrator) nonEventTerminal(
	intentID string,
	cmd session.Command,
	state CommandState,
	attempts int,
	lastErr error,
) dispatchResult {
	msg := ""
	if lastErr != nil {
		msg = lastErr.Error()
		if errors.Is(lastErr, context.DeadlineExceeded) && state != CommandTimedOut {
			state = CommandTimedOut
		}
	}
	logs.Warnf(
		"mirage.command.terminal component=mirage intent_id=%s command_id=%s ghost_id=%s state=%s attempts=%d err=%q",
		intentID, cmd.CommandID, cmd.GhostID, state, attempts, msg,
	)
	o.mu.RLock()
	now := o.now
	o.mu.RUnlock()
	return dispatchResult{Status: CommandStatus{State: state, Attempts: attempts, LastError: msg, UpdatedAt: now()}}
}

// Whether a terminal event for command_id is already in observed state.
func (o *Orchestrator) commandObserved(intentID, commandID string) bool {
	o.mu.RLock()
	defer o.mu.RUnlock()
	obs := o.observed[intentID]
	if obs == nil {
		return false
	}
	_, ok := obs.ByCommandID[commandID]
	return ok
}
