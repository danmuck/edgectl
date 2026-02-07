# Mirage Reconcile Slice (Stub)

Status: `stub`

Contract references:

- `../architecture/definitions/design.toml`
- `../architecture/definitions/protocol.toml`

## Planned Go Definitions

```go
type IntentStore interface {
	Put(issue IssueEnv) error
	Get(intentID string) (IssueEnv, bool, error)
}
```

```go
type ObservedStore interface {
	Append(event EventEnv) error
	List(intentID string) ([]EventEnv, error)
}
```

```go
type Reconciler interface {
	Reconcile(intentID string) ([]CommandEnv, error)
}
```

```go
func BuildReport(intentID string, phase string, summary string) ReportEnv
```
