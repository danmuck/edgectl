// Package activity keeps the most recent audit and custody records of this process in memory so
// the northbound API can serve them (docs/architecture/definitions/api_v1.toml [parity.activity]).
//
// Ownership boundary:
// - a bounded, process-wide record of what adminauth.Audit and observability.Emit wrote
//
// - nothing is persisted; the log stream remains the durable record
//
// Canonical references (consult before changes):
// - docs/index.md
//
// - docs/architecture/definitions/api_v1.toml
//
// - docs/architecture/definitions/admin_security.toml
//
// - docs/architecture/definitions/observability.toml
package activity

import "sync"

// Record kinds (api_v1.toml [shapes.activity_record] kind).
const (
	KindAudit   = "audit"
	KindCustody = "custody"
)

// Retention bound (api_v1.toml [parity.activity] retention).
const Capacity = 1000

// Record is one audit or custody record with the key/value pairs that were logged.
type Record struct {
	Kind        string            `json:"kind"`
	TimestampMS uint64            `json:"timestamp_ms"`
	Fields      map[string]string `json:"fields"`
}

var (
	mu sync.Mutex
	// Ring of the last Capacity records; next is the slot the next record overwrites.
	ring  []Record
	next  int
	count int
)

// Add stores one record, dropping the oldest once Capacity is reached. fields is kept as given;
// callers pass a map they do not reuse.
func Add(kind string, timestampMS uint64, fields map[string]string) {
	mu.Lock()
	defer mu.Unlock()
	if ring == nil {
		ring = make([]Record, Capacity)
	}
	ring[next] = Record{Kind: kind, TimestampMS: timestampMS, Fields: fields}
	next = (next + 1) % Capacity
	if count < Capacity {
		count++
	}
}

// Recent returns up to limit of the newest records, oldest first; kind "" matches every kind and
// limit <= 0 means no limit. Field maps are copied.
func Recent(limit int, kind string) []Record {
	mu.Lock()
	defer mu.Unlock()
	out := make([]Record, 0, count)
	start := (next - count + Capacity) % Capacity
	for i := 0; i < count; i++ {
		rec := ring[(start+i)%Capacity]
		if kind != "" && rec.Kind != kind {
			continue
		}
		fields := make(map[string]string, len(rec.Fields))
		for k, v := range rec.Fields {
			fields[k] = v
		}
		rec.Fields = fields
		out = append(out, rec)
	}
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out
}

// Reset empties the record; for tests.
func Reset() {
	mu.Lock()
	defer mu.Unlock()
	ring, next, count = nil, 0, 0
}
