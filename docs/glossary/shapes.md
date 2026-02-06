# EdgeCTL Object Shapes (Pseudocode)

This file defines pseudocode shapes for protocol objects, interface boundaries, and state ownership.
These shapes mirror `design.toml` and `protocol.toml`.
TLV and wire-level shapes are defined in `/Users/macbook/local/edgectl/docs/glossary/tlv.md`.

```go
// Package boundary map (conceptual)
package internal/protocol // wire contract + semantic field helpers
package internal/mirage   // orchestration authority
package internal/ghost    // execution authority
package internal/seeds    // service interfaces exposed by ghosts
package cmd/miragectl     // Mirage runtime
package cmd/ghostctl      // Ghost runtime
```

```go
// Mirage authority surface
interface Mirage {
  Appear(cfg MirageConfig) error
  Shimmer() error
  Seed(reg GhostSeedRegistry) error

  Issue(intent IssueEnvelope) error
  Reconcile(intentID string) ([]CommandEnvelope, error)
  Report(intentID string) (ReportEnvelope, error)
}
```

```go
// Ghost authority surface
interface Ghost {
  Appear(cfg GhostConfig) error
  Radiate() error
  Seed(reg LocalSeedRegistry) error

  Reconcile(command CommandEnvelope) (EventEnvelope, error)
}
```

```go
// Seed service boundary (owned by ghost runtime)
interface Seed {
  Metadata() SeedMetadata
  Execute(req SeedExecuteEnvelope) (SeedResultEnvelope, error)
}

struct SeedMetadata {
  ID          string
  Name        string
  Description string
}
```

```go
// Registry shapes
struct GhostSeedRegistry {
  GhostID string
  Seeds   []SeedMetadata
}

struct LocalSeedRegistry {
  Seeds map[string]Seed // key: SeedMetadata.ID
}
```

```go
// State ownership shapes
struct DesiredState {
  Owner    string // Mirage
  IntentID string
  Objective string
}

struct ExecutionState {
  Owner      string // Ghost
  CommandID  string
  SeedID     string
  LastResult SeedResultEnvelope
}

struct ObservedState {
  Owner   string // Mirage (aggregated from events)
  IntentID string
  Events  []EventEnvelope
}
```

```go
// Control loop snapshot (single intent)
struct FlowSnapshot {
  Issue       IssueEnvelope
  Commands    []CommandEnvelope
  SeedCalls   []SeedExecuteEnvelope
  SeedResults []SeedResultEnvelope
  Events      []EventEnvelope
  Report      ReportEnvelope
}
```
