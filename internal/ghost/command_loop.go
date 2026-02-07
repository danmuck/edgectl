package ghost

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrInvalidCommandRequest  = errors.New("ghost: invalid command request")
	ErrCommandRequestNotFound = errors.New("ghost: command request not found")
	ErrCommandAlreadyObserved = errors.New("ghost: command already observed")
)

const (
	ReportPhaseInProgress = "in_progress"
	ReportPhaseComplete   = "complete"

	CompletionSatisfied = "satisfied"
	CompletionFailed    = "failed"
)

type CommandRequest struct {
	RequestID string
	Command   CommandEnv
}

// ReportEnv is a minimal terminal-client/Mirage status envelope for command flow tests.
type ReportEnv struct {
	IntentID        string
	Phase           string
	Summary         string
	CompletionState string
	CommandID       string
	ExecutionID     string
	EventID         string
	Outcome         string
	LastUpdated     time.Time
}

type ObservedCommand struct {
	CommandID   string
	ExecutionID string
	Event       EventEnv
	Outcome     string
	UpdatedAt   time.Time
}

type CommandSnapshot struct {
	Desired     CommandEnv
	Observed    ObservedCommand
	HasObserved bool
}

// SingleCommandLoop is a minimal reconcile controller for one-command closure.
type SingleCommandLoop struct {
	mu       sync.RWMutex
	desired  map[string]CommandEnv
	observed map[string]ObservedCommand
	seq      atomic.Uint64
}

func NewSingleCommandLoop() *SingleCommandLoop {
	return &SingleCommandLoop{
		desired:  make(map[string]CommandEnv),
		observed: make(map[string]ObservedCommand),
	}
}

func (l *SingleCommandLoop) SubmitCommand(req CommandRequest) error {
	key := strings.TrimSpace(req.RequestID)
	if key == "" {
		return fmt.Errorf("%w: missing request_id", ErrInvalidCommandRequest)
	}
	msgSeq := l.seq.Add(1)
	cmd, err := normalizeSubmittedCommand(req.Command, key, msgSeq)
	if err != nil {
		return err
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	l.desired[key] = cmd
	delete(l.observed, key)
	return nil
}

func (l *SingleCommandLoop) SnapshotCommand(requestID string) (CommandSnapshot, bool) {
	key := strings.TrimSpace(requestID)
	l.mu.RLock()
	defer l.mu.RUnlock()
	desired, ok := l.desired[key]
	if !ok {
		return CommandSnapshot{}, false
	}
	out := CommandSnapshot{Desired: desired}
	if obs, ok := l.observed[key]; ok {
		out.Observed = obs
		out.HasObserved = true
	}
	return out, true
}

func (l *SingleCommandLoop) ReconcileOnce(srv *Server, requestID string) (ReportEnv, error) {
	key := strings.TrimSpace(requestID)
	l.mu.RLock()
	cmd, ok := l.desired[key]
	_, alreadyObserved := l.observed[key]
	l.mu.RUnlock()
	if !ok {
		return ReportEnv{}, fmt.Errorf("%w: %s", ErrCommandRequestNotFound, key)
	}
	if alreadyObserved {
		return ReportEnv{}, fmt.Errorf("%w: %s", ErrCommandAlreadyObserved, key)
	}

	event, err := srv.HandleCommandAndExecute(cmd)
	if err != nil {
		return ReportEnv{}, err
	}

	exec, ok := srv.ExecutionByCommandID(cmd.CommandID)
	if !ok {
		return ReportEnv{}, fmt.Errorf("ghost: missing execution state for command %q", cmd.CommandID)
	}

	obs := ObservedCommand{
		CommandID:   cmd.CommandID,
		ExecutionID: exec.ExecutionID,
		Event:       event,
		Outcome:     event.Outcome,
		UpdatedAt:   time.Now(),
	}
	l.mu.Lock()
	l.observed[key] = obs
	l.mu.Unlock()

	return reportFor(cmd, obs), nil
}

func normalizeSubmittedCommand(cmd CommandEnv, requestID string, msgSeq uint64) (CommandEnv, error) {
	out := cmd
	if out.MessageID == 0 {
		out.MessageID = msgSeq
	}
	if strings.TrimSpace(out.CommandID) == "" {
		out.CommandID = fmt.Sprintf("cmd.%s.%d", requestID, msgSeq)
	}
	if err := out.Validate(); err != nil {
		return CommandEnv{}, fmt.Errorf("%w: %s", ErrInvalidCommandRequest, err)
	}
	return out, nil
}

func reportFor(cmd CommandEnv, observed ObservedCommand) ReportEnv {
	completion := CompletionFailed
	summary := fmt.Sprintf("command %s failed", cmd.CommandID)
	if observed.Outcome == OutcomeSuccess {
		completion = CompletionSatisfied
		summary = fmt.Sprintf("command %s satisfied", cmd.CommandID)
	}
	return ReportEnv{
		IntentID:        cmd.IntentID,
		Phase:           ReportPhaseComplete,
		Summary:         summary,
		CompletionState: completion,
		CommandID:       observed.CommandID,
		ExecutionID:     observed.ExecutionID,
		EventID:         observed.Event.EventID,
		Outcome:         observed.Outcome,
		LastUpdated:     observed.UpdatedAt,
	}
}
