# EdgeCTL Go Shapes (Copy/Paste Units)

This file provides minimal Go-native definitions for core control-plane shapes.
Canonical contract ownership remains in:

- `../architecture/definitions/design.toml`
- `../architecture/definitions/protocol.toml`
- `../architecture/definitions/tlv.toml`

## Runtime Config

```go
type MirageConfig struct {
	NodeID string
}
```

```go
type GhostConfig struct {
	GhostID string
}
```

## Seed Metadata and Registries

```go
type SeedMetadata struct {
	ID          string
	Name        string
	Description string
}
```

```go
type GhostSeedRegistry struct {
	GhostID string
	Seeds   []SeedMetadata
}
```

```go
type LocalSeedRegistry struct {
	Seeds map[string]Seed
}
```

```go
func NewLocalSeedRegistry() LocalSeedRegistry
```

```go
func (r *LocalSeedRegistry) Register(seed Seed) error
```

```go
func (r *LocalSeedRegistry) Resolve(seedID string) (Seed, bool)
```

## Service Interface

```go
type Seed interface {
	Metadata() SeedMetadata
	Execute(req SeedExecuteEnv) (SeedResultEnv, error)
}
```

## Authority Interfaces

```go
type Mirage interface {
	Appear(cfg MirageConfig) error
	Shimmer() error
	Seed(reg GhostSeedRegistry) error

	Issue(issue IssueEnv) error
	Reconcile(intentID string) ([]CommandEnv, error)
	Report(intentID string) (ReportEnv, error)
}
```

```go
type Ghost interface {
	Appear(cfg GhostConfig) error
	Seed(reg LocalSeedRegistry) error
	Radiate() error

	Reconcile(command CommandEnv) (EventEnv, error)
}
```

## State Ownership Shapes

```go
type DesiredState struct {
	IntentID    string
	Objective   string
	TargetScope string
}
```

```go
type ExecutionState struct {
	CommandID  string
	SeedID     string
	LastResult SeedResultEnv
}
```

```go
type ObservedState struct {
	IntentID string
	Events   []EventEnv
}
```

```go
type FlowSnapshot struct {
	Issue       IssueEnv
	Commands    []CommandEnv
	SeedCalls   []SeedExecuteEnv
	SeedResults []SeedResultEnv
	Events      []EventEnv
	Report      ReportEnv
}
```
