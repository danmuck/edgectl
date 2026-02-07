# EdgeCTL Envelope Shapes (Copy/Paste Units)

This file provides minimal Go-native semantic envelope definitions.
Canonical required fields and ID mapping remain in:

- `../architecture/definitions/protocol.toml`
- `../architecture/definitions/tlv.toml`

## Common Validation Error

```go
var ErrMissingRequiredField = errors.New("missing required field")
```

## Issue Envelope (User -> Mirage)

```go
type IssueEnv struct {
	IntentID    string
	Actor       string
	TargetScope string
	Objective   string
}
```

```go
func (e IssueEnv) Validate() error
```

## Command Envelope (Mirage -> Ghost)

```go
type CommandEnv struct {
	CommandID    string
	IntentID     string
	GhostID      string
	SeedSelector string
	Operation    string
	Args         map[string]string
}
```

```go
func (e CommandEnv) Validate() error
```

## Seed Execute Envelope (Ghost -> Seed)

```go
type SeedExecuteEnv struct {
	ExecutionID string
	CommandID   string
	SeedID      string
	Operation   string
	Args        map[string]string
}
```

```go
func (e SeedExecuteEnv) Validate() error
```

## Seed Result Envelope (Seed -> Ghost)

```go
type SeedResultEnv struct {
	ExecutionID string
	SeedID      string
	Status      string
	Stdout      []byte
	Stderr      []byte
	ExitCode    int32
}
```

```go
func (e SeedResultEnv) Validate() error
```

## Event Envelope (Ghost -> Mirage)

```go
type EventEnv struct {
	EventID     string
	CommandID   string
	IntentID    string
	GhostID     string
	SeedID      string
	Outcome     string
	TimestampMS uint64
}
```

```go
func (e EventEnv) Validate() error
```

## Report Envelope (Mirage -> User)

```go
type ReportEnv struct {
	IntentID        string
	Phase           string
	Summary         string
	CompletionState string
}
```

```go
func (e ReportEnv) Validate() error
```
