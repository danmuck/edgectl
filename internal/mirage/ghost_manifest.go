package mirage

// GhostSeedInstallSpec declares one ghost seed dependency install rule.
type GhostSeedInstallSpec struct {
	SeedID             string   `json:"seed_id" toml:"seed_id"`
	Method             string   `json:"method" toml:"method"`
	Repo               string   `json:"repo,omitempty" toml:"repo"`
	Branch             string   `json:"branch,omitempty" toml:"branch"`
	Ref                string   `json:"ref,omitempty" toml:"ref"`
	Source             string   `json:"source,omitempty" toml:"source"`
	Destination        string   `json:"destination,omitempty" toml:"destination"`
	Package            string   `json:"package,omitempty" toml:"package"`
	Tap                string   `json:"tap,omitempty" toml:"tap"`
	BootstrapIfMissing bool     `json:"bootstrap_if_missing,omitempty" toml:"bootstrap_if_missing"`
	BootstrapCmd       []string `json:"bootstrap_cmd,omitempty" toml:"bootstrap_cmd"`
	InstallToBin       bool     `json:"install_to_bin,omitempty" toml:"install_to_bin"`
}

// GhostManifest declares the desired state of one remote Ghost deployment target.
type GhostManifest struct {
	GhostID                          string                 `json:"ghost_id" toml:"ghost_id"`
	Host                             string                 `json:"host" toml:"host"`
	User                             string                 `json:"user" toml:"user"`
	SSHKeyFile                       string                 `json:"ssh_key" toml:"ssh_key"`
	SSHPort                          int                    `json:"ssh_port" toml:"ssh_port"`
	Seeds                            []string               `json:"seeds" toml:"seeds"`
	AdminListen                      string                 `json:"admin_listen" toml:"admin_listen"`
	MiragePolicy                     string                 `json:"mirage_policy" toml:"mirage_policy"`
	MirageAddress                    string                 `json:"mirage_address" toml:"mirage_address"`
	SeedInstallEnabled               bool                   `json:"seed_install_enabled" toml:"seed_install_enabled"`
	SeedInstallRoot                  string                 `json:"seed_install_root" toml:"seed_install_root"`
	SeedInstallBinRoot               string                 `json:"seed_install_bin_root" toml:"seed_install_bin_root"`
	SeedInstallAllowInternalDefaults bool                   `json:"seed_install_allow_internal_defaults" toml:"seed_install_allow_internal_defaults"`
	SeedInstallWhitelist             []string               `json:"seed_install_whitelist" toml:"seed_install_whitelist"`
	SeedInstall                      []GhostSeedInstallSpec `json:"seed_install" toml:"seed_install"`
	ConfigTemplatePath               string                 `json:"config_template_path,omitempty" toml:"config_template_path"`
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
