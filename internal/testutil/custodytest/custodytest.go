// Package custodytest captures structured custody log events for test assertions.
package custodytest

import (
	"sync"
	"testing"

	"github.com/danmuck/edgectl/internal/observability"
)

// Capture is an observability.Logger that records every event written to it.
type Capture struct {
	mu     sync.Mutex
	events []observability.LogEvent
}

// Write records one event.
func (c *Capture) Write(e observability.LogEvent) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, e)
	return nil
}

// Events returns a copy of recorded events in emission order.
func (c *Capture) Events() []observability.LogEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]observability.LogEvent{}, c.events...)
}

// ForTrace returns recorded events whose trace_id matches.
func (c *Capture) ForTrace(traceID string) []observability.LogEvent {
	var out []observability.LogEvent
	for _, e := range c.Events() {
		if e.TraceID == traceID {
			out = append(out, e)
		}
	}
	return out
}

// Install replaces the custody logger for the duration of the test.
func Install(t *testing.T) *Capture {
	t.Helper()
	c := &Capture{}
	restore := observability.SetLogger(c)
	t.Cleanup(restore)
	return c
}

// RequireValid fails the test for any event missing observability.toml required fields.
func RequireValid(t *testing.T, events []observability.LogEvent) {
	t.Helper()
	for i, e := range events {
		if err := e.Validate(); err != nil {
			t.Errorf("event %d invalid: %v\n  %s", i, err, e.Line())
		}
	}
}

// Step identifies one custody hop for ordered-chain assertions.
type Step struct {
	Component   string
	MessageType uint32
	Direction   string
}

// RequireOrderedSteps fails unless want appears in events as an ordered subsequence.
func RequireOrderedSteps(t *testing.T, events []observability.LogEvent, want []Step) {
	t.Helper()
	i := 0
	for _, e := range events {
		if i < len(want) && e.Component == want[i].Component && e.MessageType == want[i].MessageType && e.Direction == want[i].Direction {
			i++
		}
	}
	if i < len(want) {
		t.Errorf("custody chain incomplete: matched %d/%d steps, missing %+v", i, len(want), want[i])
		for _, e := range events {
			t.Logf("  %s", e.Line())
		}
	}
}

// S builds one Step.
func S(component string, messageType uint32, direction string) Step {
	return Step{Component: component, MessageType: messageType, Direction: direction}
}
