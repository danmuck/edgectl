```go
// Semantic envelope: issue (User -> Mirage)
struct IssueEnvelope {
  IntentID    string
  Actor       string
  TargetScope string
  Objective   string
}
```

```go
// Semantic envelope: command (Mirage -> Ghost)
struct CommandEnvelope {
  CommandID    string
  IntentID     string
  GhostID      string
  SeedSelector string
  Operation    string
}
```

```go
// Semantic envelope: seed_execute (Ghost -> Seed)
struct SeedExecuteEnvelope {
  ExecutionID string
  CommandID   string
  SeedID      string
  Operation   string
  Args        map[string]string
}
```

```go
// Semantic envelope: seed_result (Seed -> Ghost)
struct SeedResultEnvelope {
  ExecutionID string
  SeedID      string
  Status      string
  Stdout      []byte
  Stderr      []byte
  ExitCode    int
}
```

```go
// Semantic envelope: event (Ghost -> Mirage)
struct EventEnvelope {
  EventID   string
  CommandID string
  IntentID  string
  GhostID   string
  SeedID    string
  Outcome   string
}
```

```go
// Semantic envelope: report (Mirage -> User)
struct ReportEnvelope {
  IntentID        string
  Phase           string // satisfied | in_progress | corrective
  Summary         string
  CompletionState string
}
```
