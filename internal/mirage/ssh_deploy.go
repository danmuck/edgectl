package mirage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	configContent := d.generateConfig(m, adminListen)
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
	startCmd := "nohup ~/.edgectl/bin/ghostctl -config ~/.edgectl/config.toml > ~/.edgectl/ghostctl.log 2>&1 &"
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
func (d *SSHDeployer) generateConfig(m GhostManifest, adminListen string) string {
	seeds := m.Seeds
	if len(seeds) == 0 {
		seeds = []string{"seed.flow", "seed.host"}
	}

	policy := strings.TrimSpace(m.MiragePolicy)
	if policy == "" {
		policy = "auto"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "id = %q\n", m.GhostID)
	fmt.Fprintf(&b, "admin_listen = %q\n", adminListen)
	fmt.Fprintf(&b, "seeds = [%s]\n", quotedList(seeds))
	fmt.Fprintf(&b, "mirage_policy = %q\n", policy)
	if addr := strings.TrimSpace(m.MirageAddress); addr != "" {
		fmt.Fprintf(&b, "mirage_address = %q\n", addr)
	}
	fmt.Fprintf(&b, "project_fetch_on_boot = false\n")
	return b.String()
}

// quotedList formats a string slice as quoted TOML array entries.
func quotedList(items []string) string {
	parts := make([]string, len(items))
	for i, item := range items {
		parts[i] = fmt.Sprintf("%q", strings.TrimSpace(item))
	}
	return strings.Join(parts, ", ")
}

// extractPort returns the port portion of a host:port address.
func extractPort(addr string) string {
	if idx := strings.LastIndex(addr, ":"); idx >= 0 {
		return addr[idx+1:]
	}
	return "7010"
}
