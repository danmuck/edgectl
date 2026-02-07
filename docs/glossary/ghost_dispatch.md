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
	Phase        ExecutionPhase
}
```

```go
func (s *Server) HandleCommand(cmd CommandEnv) (ExecutionState, error)
```

```go
func (s *Server) ExecutionByCommandID(commandID string) (ExecutionState, bool)
```

```go
func (s *Server) ExecutionByMessageID(messageID uint64) (ExecutionState, bool)
```
