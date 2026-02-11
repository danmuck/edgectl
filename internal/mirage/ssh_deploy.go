package mirage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/danmuck/edgectl/internal/tools"
	logs "github.com/danmuck/smplog"
)

// SSHDeployer provisions remote Ghost nodes via SSH/SCP using the system ssh binary.
type SSHDeployer struct {
	runner tools.CommandRunner
}

// NewSSHDeployer creates a deployer backed by the local ssh/scp CLI.
func NewSSHDeployer() *SSHDeployer {
	return &SSHDeployer{runner: tools.ExecRunner{}}
}

// Deploy executes a full remote Ghost provisioning sequence for one manifest.
func (d *SSHDeployer) Deploy(ctx context.Context, req DeployGhostRequest) (DeployGhostResult, error) {
	m := req.Manifest
	if strings.TrimSpace(m.GhostID) == "" {
		return DeployGhostResult{}, fmt.Errorf("ssh_deploy: ghost_id required")
	}
	if strings.TrimSpace(m.Host) == "" {
		return DeployGhostResult{}, fmt.Errorf("ssh_deploy: host required")
	}
	user := strings.TrimSpace(m.User)
	if user == "" {
		user = "deploy"
	}
	sshPort := m.SSHPort
	if sshPort == 0 {
		sshPort = 22
	}
	keyFile := strings.TrimSpace(m.SSHKeyFile)
	if keyFile == "" {
		home, _ := os.UserHomeDir()
		keyFile = filepath.Join(home, ".ssh", "id_ed25519")
	}

	binaryPath := strings.TrimSpace(req.BinaryPath)
	if binaryPath == "" {
		binaryPath = "local/bin/ghostctl"
	}
	if _, err := os.Stat(binaryPath); err != nil {
		return DeployGhostResult{}, fmt.Errorf("ssh_deploy: binary not found at %q: %w", binaryPath, err)
	}

	adminListen := strings.TrimSpace(m.AdminListen)
	if adminListen == "" {
		adminListen = "0.0.0.0:7010"
	}

	remoteTarget := fmt.Sprintf("%s@%s", user, m.Host)
	sshBase := d.sshBaseArgs(keyFile, sshPort)

	logs.Infof("ssh_deploy: deploying ghost_id=%q to %s", m.GhostID, remoteTarget)

	// Step 1: Create remote directory.
	if err := d.sshExec(sshBase, remoteTarget, "mkdir -p ~/.edgectl/bin"); err != nil {
		return DeployGhostResult{Status: "error", GhostID: m.GhostID, Host: m.Host,
			Message: fmt.Sprintf("mkdir failed: %v", err)}, err
	}

	// Step 2: SCP ghostctl binary.
	if err := d.scpFile(sshBase, binaryPath, remoteTarget, "~/.edgectl/bin/ghostctl"); err != nil {
		return DeployGhostResult{Status: "error", GhostID: m.GhostID, Host: m.Host,
			Message: fmt.Sprintf("scp binary failed: %v", err)}, err
	}

	// Step 3: Make binary executable.
	if err := d.sshExec(sshBase, remoteTarget, "chmod +x ~/.edgectl/bin/ghostctl"); err != nil {
		return DeployGhostResult{Status: "error", GhostID: m.GhostID, Host: m.Host,
			Message: fmt.Sprintf("chmod failed: %v", err)}, err
	}

	// Step 4: Generate and upload config.
	configContent, err := d.generateConfig(req, adminListen)
	if err != nil {
		return DeployGhostResult{Status: "error", GhostID: m.GhostID, Host: m.Host}, err
	}
	configTmpFile, err := os.CreateTemp("", "ghostctl-config-*.toml")
	if err != nil {
		return DeployGhostResult{Status: "error", GhostID: m.GhostID, Host: m.Host}, err
	}
	defer os.Remove(configTmpFile.Name())
	if _, err := configTmpFile.WriteString(configContent); err != nil {
		configTmpFile.Close()
		return DeployGhostResult{Status: "error", GhostID: m.GhostID, Host: m.Host}, err
	}
	configTmpFile.Close()

	if err := d.scpFile(sshBase, configTmpFile.Name(), remoteTarget, "~/.edgectl/config.toml"); err != nil {
		return DeployGhostResult{Status: "error", GhostID: m.GhostID, Host: m.Host,
			Message: fmt.Sprintf("scp config failed: %v", err)}, err
	}

	// Step 5: Stop existing process if force redeploy.
	if req.ForceRedeploy {
		// Best-effort kill of existing process.
		_ = d.sshExec(sshBase, remoteTarget, "pkill -f 'edgectl/bin/ghostctl' || true")
	}

	// Step 6: Start ghostctl remotely via nohup.
	startCmd := "nohup env PATH=\"$HOME/.edgectl/local/bin:$PATH\" ~/.edgectl/bin/ghostctl -config ~/.edgectl/config.toml > ~/.edgectl/ghostctl.log 2>&1 &"
	if err := d.sshExec(sshBase, remoteTarget, startCmd); err != nil {
		return DeployGhostResult{Status: "error", GhostID: m.GhostID, Host: m.Host,
			Message: fmt.Sprintf("start failed: %v", err)}, err
	}

	adminAddr := fmt.Sprintf("%s:%s", m.Host, extractPort(adminListen))
	logs.Infof("ssh_deploy: ghost started ghost_id=%q admin_addr=%q", m.GhostID, adminAddr)

	return DeployGhostResult{
		GhostID:   m.GhostID,
		Host:      m.Host,
		AdminAddr: adminAddr,
		Status:    "deployed",
		Message:   "ghost process started via nohup",
	}, nil
}

// sshBaseArgs builds common SSH flags for key auth and port.
func (d *SSHDeployer) sshBaseArgs(keyFile string, port int) []string {
	return []string{
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "ConnectTimeout=10",
		"-i", keyFile,
		"-p", fmt.Sprintf("%d", port),
	}
}

// sshExec runs a single command on a remote host via ssh.
func (d *SSHDeployer) sshExec(baseArgs []string, target string, cmd string) error {
	args := append(append([]string{}, baseArgs...), target, cmd)
	logs.Infof("ssh_deploy: ssh %s %q", target, cmd)
	_, stderr, exitCode, err := d.runner.Run("ssh", args...)
	if err != nil {
		return fmt.Errorf("ssh exec failed cmd=%q exit=%d stderr=%q: %w",
			cmd, exitCode, strings.TrimSpace(string(stderr)), err)
	}
	return nil
}

// scpFile uploads a local file to a remote path via scp.
func (d *SSHDeployer) scpFile(baseArgs []string, localPath string, target string, remotePath string) error {
	// scp uses -P for port, not -p. Rewrite port flag.
	scpArgs := make([]string, 0, len(baseArgs)+2)
	for i := 0; i < len(baseArgs); i++ {
		if baseArgs[i] == "-p" && i+1 < len(baseArgs) {
			scpArgs = append(scpArgs, "-P", baseArgs[i+1])
			i++
			continue
		}
		scpArgs = append(scpArgs, baseArgs[i])
	}
	dest := fmt.Sprintf("%s:%s", target, remotePath)
	scpArgs = append(scpArgs, localPath, dest)
	logs.Infof("ssh_deploy: scp %s -> %s", localPath, dest)
	_, stderr, exitCode, err := d.runner.Run("scp", scpArgs...)
	if err != nil {
		return fmt.Errorf("scp failed exit=%d stderr=%q: %w",
			exitCode, strings.TrimSpace(string(stderr)), err)
	}
	return nil
}

// generateConfig produces a minimal ghostctl config.toml for the remote host.
func (d *SSHDeployer) generateConfig(req DeployGhostRequest, adminListen string) (string, error) {
	m := req.Manifest
	seeds := m.Seeds
	if len(seeds) == 0 {
		seeds = []string{"seed.flow", "seed.host"}
	}

	policy := strings.TrimSpace(m.MiragePolicy)
	if policy == "" {
		policy = "auto"
	}

	templatePath := strings.TrimSpace(m.ConfigTemplatePath)
	if templatePath == "" {
		templatePath = strings.TrimSpace(req.ConfigTemplatePath)
	}
	if templatePath != "" {
		return d.generateConfigFromTemplate(templatePath, m, seeds, adminListen, policy)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "id = %q\n", m.GhostID)
	fmt.Fprintf(&b, "admin_listen = %q\n", adminListen)
	fmt.Fprintf(&b, "seeds = [%s]\n", quotedList(seeds))
	fmt.Fprintf(&b, "mirage_policy = %q\n", policy)
	if addr := strings.TrimSpace(m.MirageAddress); addr != "" {
		fmt.Fprintf(&b, "mirage_address = %q\n", addr)
	}
	if m.SeedInstallEnabled {
		installRoot := strings.TrimSpace(m.SeedInstallRoot)
		if installRoot == "" {
			installRoot = "local/seeds"
		}
		binRoot := strings.TrimSpace(m.SeedInstallBinRoot)
		if binRoot == "" {
			binRoot = "local/bin"
		}
		whitelist := normalizeSeedInstallWhitelist(m.SeedInstallWhitelist)
		if len(whitelist) == 0 && m.SeedInstallAllowInternalDefaults {
			whitelist = internalSeedInstallDefaults()
		}
		fmt.Fprintf(&b, "seed_install_enabled = true\n")
		fmt.Fprintf(&b, "seed_install_root = %q\n", installRoot)
		fmt.Fprintf(&b, "seed_install_bin_root = %q\n", binRoot)
		fmt.Fprintf(&b, "seed_install_whitelist = [%s]\n", quotedList(whitelist))
		for i := range m.SeedInstall {
			spec := m.SeedInstall[i]
			fmt.Fprintf(&b, "\n[[seed_install]]\n")
			fmt.Fprintf(&b, "seed_id = %q\n", strings.TrimSpace(spec.SeedID))
			fmt.Fprintf(&b, "method = %q\n", strings.TrimSpace(spec.Method))
			if v := strings.TrimSpace(spec.Repo); v != "" {
				fmt.Fprintf(&b, "repo = %q\n", v)
			}
			if v := strings.TrimSpace(spec.Branch); v != "" {
				fmt.Fprintf(&b, "branch = %q\n", v)
			}
			if v := strings.TrimSpace(spec.Ref); v != "" {
				fmt.Fprintf(&b, "ref = %q\n", v)
			}
			if v := strings.TrimSpace(spec.Source); v != "" {
				fmt.Fprintf(&b, "source = %q\n", v)
			}
			if v := strings.TrimSpace(spec.Destination); v != "" {
				fmt.Fprintf(&b, "destination = %q\n", v)
			}
			if v := strings.TrimSpace(spec.Package); v != "" {
				fmt.Fprintf(&b, "package = %q\n", v)
			}
			if v := strings.TrimSpace(spec.Tap); v != "" {
				fmt.Fprintf(&b, "tap = %q\n", v)
			}
			if spec.BootstrapIfMissing {
				fmt.Fprintf(&b, "bootstrap_if_missing = true\n")
			}
			if len(spec.BootstrapCmd) > 0 {
				fmt.Fprintf(&b, "bootstrap_cmd = [%s]\n", quotedList(spec.BootstrapCmd))
			}
			if spec.InstallToBin {
				fmt.Fprintf(&b, "install_to_bin = true\n")
			}
		}
	} else {
		fmt.Fprintf(&b, "seed_install_enabled = false\n")
	}
	fmt.Fprintf(&b, "project_fetch_on_boot = false\n")
	return b.String(), nil
}

// quotedList formats a string slice as quoted TOML array entries.
func quotedList(items []string) string {
	parts := make([]string, len(items))
	for i, item := range items {
		parts[i] = fmt.Sprintf("%q", strings.TrimSpace(item))
	}
	return strings.Join(parts, ", ")
}

func normalizeSeedInstallWhitelist(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for i := range in {
		id := strings.TrimSpace(in[i])
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func internalSeedInstallDefaults() []string {
	return []string{
		"seed.docker",
		"seed.flow",
		"seed.fs",
		"seed.host",
		"seed.kv",
		"seed.mongod",
	}
}

func (d *SSHDeployer) generateConfigFromTemplate(
	templatePath string,
	m GhostManifest,
	seeds []string,
	adminListen string,
	policy string,
) (string, error) {
	var cfg map[string]any
	if _, err := toml.DecodeFile(templatePath, &cfg); err != nil {
		return "", fmt.Errorf("ssh_deploy: load config template %q: %w", templatePath, err)
	}
	if cfg == nil {
		cfg = make(map[string]any)
	}
	cfg["id"] = m.GhostID
	cfg["admin_listen"] = adminListen
	cfg["seeds"] = seeds
	cfg["mirage_policy"] = policy
	cfg["mirage_peer_identity"] = m.GhostID
	if addr := strings.TrimSpace(m.MirageAddress); addr != "" {
		cfg["mirage_address"] = addr
	}
	applySeedInstallTemplateOverrides(cfg, m)
	var out strings.Builder
	if err := toml.NewEncoder(&out).Encode(cfg); err != nil {
		return "", fmt.Errorf("ssh_deploy: encode config template %q: %w", templatePath, err)
	}
	return out.String(), nil
}

func applySeedInstallTemplateOverrides(cfg map[string]any, m GhostManifest) {
	if !m.SeedInstallEnabled {
		cfg["seed_install_enabled"] = false
		return
	}
	installRoot := strings.TrimSpace(m.SeedInstallRoot)
	if installRoot == "" {
		installRoot = "local/seeds"
	}
	binRoot := strings.TrimSpace(m.SeedInstallBinRoot)
	if binRoot == "" {
		binRoot = "local/bin"
	}
	whitelist := normalizeSeedInstallWhitelist(m.SeedInstallWhitelist)
	if len(whitelist) == 0 && m.SeedInstallAllowInternalDefaults {
		whitelist = internalSeedInstallDefaults()
	}
	cfg["seed_install_enabled"] = true
	cfg["seed_install_root"] = installRoot
	cfg["seed_install_bin_root"] = binRoot
	cfg["seed_install_whitelist"] = whitelist
	seedInstall := make([]map[string]any, 0, len(m.SeedInstall))
	for i := range m.SeedInstall {
		spec := m.SeedInstall[i]
		seedID := strings.TrimSpace(spec.SeedID)
		method := strings.TrimSpace(spec.Method)
		if seedID == "" || method == "" {
			continue
		}
		row := map[string]any{
			"seed_id": seedID,
			"method":  method,
		}
		if v := strings.TrimSpace(spec.Repo); v != "" {
			row["repo"] = v
		}
		if v := strings.TrimSpace(spec.Branch); v != "" {
			row["branch"] = v
		}
		if v := strings.TrimSpace(spec.Ref); v != "" {
			row["ref"] = v
		}
		if v := strings.TrimSpace(spec.Source); v != "" {
			row["source"] = v
		}
		if v := strings.TrimSpace(spec.Destination); v != "" {
			row["destination"] = v
		}
		if v := strings.TrimSpace(spec.Package); v != "" {
			row["package"] = v
		}
		if v := strings.TrimSpace(spec.Tap); v != "" {
			row["tap"] = v
		}
		if spec.BootstrapIfMissing {
			row["bootstrap_if_missing"] = true
		}
		if len(spec.BootstrapCmd) > 0 {
			row["bootstrap_cmd"] = spec.BootstrapCmd
		}
		if spec.InstallToBin {
			row["install_to_bin"] = true
		}
		seedInstall = append(seedInstall, row)
	}
	cfg["seed_install"] = seedInstall
}

// extractPort returns the port portion of a host:port address.
func extractPort(addr string) string {
	if idx := strings.LastIndex(addr, ":"); idx >= 0 {
		return addr[idx+1:]
	}
	return "7010"
}
