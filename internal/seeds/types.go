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
	ExitCode uint32
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

// OKResult builds a success SeedResult with the given stdout text.
func OKResult(stdout string) SeedResult {
	return SeedResult{
		Status:   "ok",
		Stdout:   []byte(stdout),
		ExitCode: 0,
	}
}

// ErrorResult builds a failure SeedResult from a message string.
func ErrorResult(msg string) SeedResult {
	return SeedResult{
		Status:   "error",
		Stderr:   []byte(msg + "\n"),
		ExitCode: 1,
	}
}

// ErrorResultErr builds a failure SeedResult from an error value.
func ErrorResultErr(err error) SeedResult {
	msg := "error"
	if err != nil {
		msg = err.Error()
	}
	return SeedResult{
		Status:   "error",
		Stderr:   []byte(msg + "\n"),
		ExitCode: 1,
	}
}

// DepSpec declares one external dependency required by a seed on a target host.
type DepSpec struct {
	Name          string        // human-readable dependency name (e.g. "docker")
	Binary        string        // expected binary name in PATH (e.g. "docker")
	InstallMethod InstallMethod // allowlisted install method (apt, brew, github, etc.)
	AptPackage    string        // apt package name for Ubuntu targets
	CurlURL       string        // direct download URL for curl method
	Destination   string        // install destination relative to install root
	Required      bool          // hard dependency — blocks seed execution if missing
}

// Seed is the seed execution boundary used by Ghost-local dispatch.
type Seed interface {
	Metadata() SeedMetadata
	Operations() []OperationSpec
	CommandCatalog() []CommandTemplate
	Execute(action string, args map[string]string) (SeedResult, error)
}
