package session

import (
	"sort"
	"strings"
	"sync"
	"time"
)

// Session outbox record for one event awaiting event.ack.
type PendingEvent struct {
	EventID       string
	CommandID     string
	GhostID       string
	Attempts      int
	QueuedAt      time.Time
	LastAttemptAt time.Time
	AckDeadlineAt time.Time
	LastError     string
}

// Session outbox store keyed by stable event_id.
type EventOutbox struct {
	mu    sync.RWMutex
	items map[string]PendingEvent
}

// Session outbox constructor for an empty event_id-keyed store.
func NewEventOutbox() *EventOutbox {
	return &EventOutbox{
		items: make(map[string]PendingEvent),
	}
}

// Session outbox insert-or-replace by event_id.
func (o *EventOutbox) Upsert(item PendingEvent) {
	key := strings.TrimSpace(item.EventID)
	if key == "" {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.items[key] = item
}

// Session outbox attempt tracker for retry counters and latest error details.
func (o *EventOutbox) MarkAttempt(eventID string, at time.Time, lastErr string) (PendingEvent, bool) {
	key := strings.TrimSpace(eventID)
	o.mu.Lock()
	defer o.mu.Unlock()
	item, ok := o.items[key]
	if !ok {
		return PendingEvent{}, false
	}
	item.Attempts++
	item.LastAttemptAt = at
	item.LastError = strings.TrimSpace(lastErr)
	o.items[key] = item
	return item, true
}

// Session outbox delete by event_id.
func (o *EventOutbox) Remove(eventID string) {
	key := strings.TrimSpace(eventID)
	o.mu.Lock()
	defer o.mu.Unlock()
	delete(o.items, key)
}

// Session outbox lookup by event_id.
func (o *EventOutbox) Get(eventID string) (PendingEvent, bool) {
	key := strings.TrimSpace(eventID)
	o.mu.RLock()
	defer o.mu.RUnlock()
	item, ok := o.items[key]
	return item, ok
}

// Session outbox snapshot in deterministic event_id order.
func (o *EventOutbox) List() []PendingEvent {
	o.mu.RLock()
	defer o.mu.RUnlock()
	out := make([]PendingEvent, 0, len(o.items))
	for _, item := range o.items {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].EventID < out[j].EventID
	})
	return out
}
