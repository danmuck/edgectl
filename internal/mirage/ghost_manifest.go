package mirage

// GhostManifest declares the desired state of one remote Ghost deployment target.
type GhostManifest struct {
	GhostID       string   `json:"ghost_id" toml:"ghost_id"`
	Host          string   `json:"host" toml:"host"`
	User          string   `json:"user" toml:"user"`
	SSHKeyFile    string   `json:"ssh_key" toml:"ssh_key"`
	SSHPort       int      `json:"ssh_port" toml:"ssh_port"`
	Seeds         []string `json:"seeds" toml:"seeds"`
	AdminListen   string   `json:"admin_listen" toml:"admin_listen"`
	MiragePolicy  string   `json:"mirage_policy" toml:"mirage_policy"`
	MirageAddress string   `json:"mirage_address" toml:"mirage_address"`
}

// DeployGhostRequest carries one remote ghost deployment instruction from admin boundary.
type DeployGhostRequest struct {
	Manifest       GhostManifest `json:"manifest"`
	BinaryPath     string        `json:"binary_path"`
	ForceRedeploy  bool          `json:"force_redeploy"`
}

// DeployGhostResult reports the outcome of one remote ghost deployment.
type DeployGhostResult struct {
	GhostID   string `json:"ghost_id"`
	Host      string `json:"host"`
	AdminAddr string `json:"admin_addr"`
	Status    string `json:"status"`
	Message   string `json:"message,omitempty"`
}
