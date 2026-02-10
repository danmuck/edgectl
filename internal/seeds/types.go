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

// CommandArgSpec defines one guided argument input for a seed command template.
type CommandArgSpec struct {
	Key          string
	Prompt       string
	Required     bool
	DefaultValue string
	Multiline    bool
	Terminator   string
}

// CommandTemplate defines one seed-owned command catalog entry for operator clients.
type CommandTemplate struct {
	ID              string
	Label           string
	Description     string
	SeedSelector    string
	Operation       string
	Args            []CommandArgSpec
	DefaultBlocking bool
}

// Seed is the seed execution boundary used by Ghost-local dispatch.
type Seed interface {
	Metadata() SeedMetadata
	Operations() []OperationSpec
	CommandCatalog() []CommandTemplate
	Execute(action string, args map[string]string) (SeedResult, error)
}
