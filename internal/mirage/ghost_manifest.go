package mirage

// GhostManifest declares the desired state of one remote Ghost deployment target. Seed-install
// fields configure whether the Ghost accepts Mirage-approved install plans and where it may install;
// install specs themselves are never shipped in Ghost config (dependency_catalog.go is the source).
type GhostManifest struct {
	GhostID                          string   `json:"ghost_id" toml:"ghost_id"`
	Host                             string   `json:"host" toml:"host"`
	User                             string   `json:"user" toml:"user"`
	SSHKeyFile                       string   `json:"ssh_key" toml:"ssh_key"`
	SSHPort                          int      `json:"ssh_port" toml:"ssh_port"`
	Seeds                            []string `json:"seeds" toml:"seeds"`
	AdminListen                      string   `json:"admin_listen" toml:"admin_listen"`
	MiragePolicy                     string   `json:"mirage_policy" toml:"mirage_policy"`
	MirageAddress                    string   `json:"mirage_address" toml:"mirage_address"`
	SeedInstallEnabled               bool     `json:"seed_install_enabled" toml:"seed_install_enabled"`
	SeedInstallRoot                  string   `json:"seed_install_root" toml:"seed_install_root"`
	SeedInstallBinRoot               string   `json:"seed_install_bin_root" toml:"seed_install_bin_root"`
	SeedInstallAllowInternalDefaults bool     `json:"seed_install_allow_internal_defaults" toml:"seed_install_allow_internal_defaults"`
	SeedInstallWhitelist             []string `json:"seed_install_whitelist" toml:"seed_install_whitelist"`
	ConfigTemplatePath               string   `json:"config_template_path,omitempty" toml:"config_template_path"`
}

// DeployGhostRequest carries one remote ghost deployment instruction from admin boundary.
type DeployGhostRequest struct {
	Manifest           GhostManifest `json:"manifest"`
	BinaryPath         string        `json:"binary_path"`
	ForceRedeploy      bool          `json:"force_redeploy"`
	ConfigTemplatePath string        `json:"config_template_path,omitempty"`
}

// DeployGhostResult reports the outcome of one remote ghost deployment.
type DeployGhostResult struct {
	GhostID   string `json:"ghost_id"`
	Host      string `json:"host"`
	AdminAddr string `json:"admin_addr"`
	Status    string `json:"status"`
	Message   string `json:"message,omitempty"`
}
