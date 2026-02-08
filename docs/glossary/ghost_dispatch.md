# Ghost Dispatch Slice

Status: `in progress`

Contract references:

- `../architecture/definitions/design.toml`
- `../architecture/definitions/protocol.toml`
- `../architecture/definitions/observability.toml`
- `../architecture/transport.md`
- `../architecture/models/discovery.mmd`

## Boundary and Lifecycle Rules

- Ghost accepts `command` only after `appear -> seed -> radiate`.
- Command boundary validation requires:
- `message_id`, `command_id`, `intent_id`, `ghost_id`, `seed_selector`, `operation`
- Ghost rejects command when target `ghost_id` does not match local ghost identity.
- Ghost creates execution state on accepted command and indexes by:
- `command_id`
- `message_id`
- `execution_id`
- Every accepted command produces exactly one terminal event:
- `outcome=success` for successful seed execution
- `outcome=error` for unknown seed/unknown action/seed execution failure

## Current Go Definitions

```go
type CommandEnv struct {
	MessageID    uint64
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

```go
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
```

```go
func (s *Server) HandleCommand(cmd CommandEnv) (ExecutionState, error)
```

```go
func (s *Server) HandleCommandAndExecute(cmd CommandEnv) (EventEnv, error)
```

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
func (s *Server) GetExecution(executionID string) (ExecutionState, bool)
```

```go
func (s *Server) GetByCommandID(commandID string) (ExecutionState, bool)
```

```go
func (s *Server) ExecutionByMessageID(messageID uint64) (ExecutionState, bool)
```
