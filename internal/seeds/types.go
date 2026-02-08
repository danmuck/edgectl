package seeds

// SeedMetadata is the contract for seed identity and display data.
type SeedMetadata struct {
	ID          string
	Name        string
	Description string
}

// SeedResult is the minimal deterministic execution result shape.
type SeedResult struct {
	Status   string
	Stdout   []byte
	Stderr   []byte
	ExitCode int32
}

// OperationSpec defines one supported seed action.
type OperationSpec struct {
	Name        string
	Description string
	Idempotent  bool
}

// Seed is the seed execution boundary used by Ghost-local dispatch.
type Seed interface {
	Metadata() SeedMetadata
	Operations() []OperationSpec
	Execute(action string, args map[string]string) (SeedResult, error)
}
