# Ghost Dispatch Slice (Stub)

Status: `stub`

Contract references:

- `../architecture/definitions/design.toml`
- `../architecture/definitions/protocol.toml`

## Planned Go Definitions

```go
type CommandExecutor interface {
	Execute(command CommandEnv) (EventEnv, error)
}
```

```go
type SeedRegistry interface {
	Resolve(seedSelector string) (Seed, bool)
}
```

```go
func DispatchCommand(reg SeedRegistry, command CommandEnv) (SeedExecuteEnv, error)
```

```go
func BuildEvent(command CommandEnv, result SeedResultEnv, outcome string) EventEnv
```
