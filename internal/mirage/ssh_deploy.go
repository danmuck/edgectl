package mirage

import (
	"context"
	"crypto/x509"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/danmuck/edgectl/internal/identity"
	"github.com/danmuck/edgectl/internal/tools"
	logs "github.com/danmuck/smplog"
)

// Remote layout under the deploy user's home. The tls directory is created 0700 and the key is
// written 0600 (admin_security.toml deploy.key_transfer).
const (
	remoteDir      = ".edgectl"
	remoteBin      = ".edgectl/bin/ghostctl"
	remoteConfig   = ".edgectl/config.toml"
	remoteTLSDir   = ".edgectl/tls"
	remoteCertFile = ".edgectl/tls/ghost.crt"
	remoteKeyFile  = ".edgectl/tls/ghost.key"
	remoteCAFile   = ".edgectl/tls/ca.crt"
)

// Absolute home path reported by the target; anything else aborts the deploy.
var remoteHomePattern = regexp.MustCompile(`^/[A-Za-z0-9._/\-]+$`)

// SSHDeployer provisions one remote Ghost from a validated manifest via the system ssh/scp binaries.
// It delivers only that Ghost's own leaf credentials, the root CA certificate, and the pinned Mirage
// principal (admin_security.toml [deploy]); no credential is shared between Ghosts.
type SSHDeployer struct {
	runner tools.CommandRunner
	// Root CA certificate delivered to the Ghost (Mirage's admin CA).
	caFile string
	// Mirage principal the Ghost pins for its admin endpoint and session.
	miragePrincipal identity.Principal
	// Admin denied serials carried into the Ghost config.
	deniedSerials []string
}

// NewSSHDeployer creates a deployer backed by the local ssh/scp CLI.
func NewSSHDeployer(caFile string, miragePrincipal identity.Principal, deniedSerials []string) *SSHDeployer {
	return &SSHDeployer{runner: tools.ExecRunner{}, caFile: caFile, miragePrincipal: miragePrincipal, deniedSerials: deniedSerials}
}

// DeployGhost deploys a configured target by ghost_id. Unknown ids are rejected, and host, user, port,
// host key, and credentials come only from the manifest (admin_security.toml deploy.targets). Requires
// admin TLS: the Ghost is delivered Mirage's CA and pins Mirage's principal.
func (s *Service) DeployGhost(ctx context.Context, req DeployGhostRequest) (DeployGhostResult, error) {
	id := strings.TrimSpace(req.GhostID)
	var manifest *GhostManifest
	for i := range s.cfg.GhostManifests {
		if strings.TrimSpace(s.cfg.GhostManifests[i].GhostID) == id && id != "" {
			manifest = &s.cfg.GhostManifests[i]
			break
		}
	}
	if manifest == nil {
		return DeployGhostResult{}, fmt.Errorf("%w: ghost_id=%q has no ghost_manifests entry", ErrUnknownDeployTarget, id)
	}
	if !s.cfg.AdminTLS.Enabled() || strings.TrimSpace(s.cfg.AdminTLS.CAFile) == "" {
		return DeployGhostResult{}, fmt.Errorf("%w: deploy requires admin_tls_cert_file and admin_tls_ca_file", ErrInvalidDeployManifest)
	}
	roots, err := loadCertPool(s.cfg.AdminTLS.CAFile)
	if err != nil {
		return DeployGhostResult{}, fmt.Errorf("%w: %v", ErrInvalidDeployManifest, err)
	}
	denied, err := identity.ParseDeniedSerials(s.cfg.AdminTLS.DeniedSerials)
	if err != nil {
		return DeployGhostResult{}, err
	}
	target, err := resolveDeployTarget(*manifest, roots, denied)
	if err != nil {
		return DeployGhostResult{}, err
	}
	deployer := NewSSHDeployer(s.cfg.AdminTLS.CAFile, s.selfPrincipal(), s.cfg.AdminTLS.DeniedSerials)
	if s.sshRunner != nil {
		deployer.runner = s.sshRunner
	}
	return deployer.Deploy(ctx, target, req.ForceRedeploy)
}

func loadCertPool(caFile string) (*x509.CertPool, error) {
	raw, err := os.ReadFile(strings.TrimSpace(caFile))
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(raw) {
		return nil, fmt.Errorf("no certificates in %s", caFile)
	}
	return pool, nil
}

// Deploy runs the provisioning sequence for one resolved target. Every ssh/scp call verifies the
// pinned host key (StrictHostKeyChecking=yes against a known_hosts file holding only that key).
// Key material is never logged or returned; failures report the step only.
func (d *SSHDeployer) Deploy(_ context.Context, t deployTarget, force bool) (DeployGhostResult, error) {
	m := t.manifest
	fail := func(step string, err error) (DeployGhostResult, error) {
		return DeployGhostResult{Status: "error", GhostID: m.GhostID, Host: m.Host, Message: step + " failed"}, fmt.Errorf("ssh_deploy %s: %w", step, err)
	}
	if _, err := os.Stat(t.binary); err != nil {
		return fail("binary", err)
	}
	knownHosts, err := writeKnownHosts(m.Host, t.port, t.hostKey)
	if err != nil {
		return fail("known_hosts", err)
	}
	defer os.Remove(knownHosts)

	remote := fmt.Sprintf("%s@%s", t.user, m.Host)
	base := sshBaseArgs(t.keyFile, knownHosts, t.port)
	logs.Infof("ssh_deploy: deploying ghost_id=%q to %s port=%d", m.GhostID, remote, t.port)

	out, err := d.sshExec(base, remote, `printf '%s' "$HOME"`)
	if err != nil {
		return fail("home", err)
	}
	home := strings.TrimSpace(string(out))
	if !remoteHomePattern.MatchString(home) {
		return fail("home", fmt.Errorf("unexpected home path %q", home))
	}
	// 0700 tls dir before any key lands; a stale key file is removed so scp cannot keep its old mode.
	prep := fmt.Sprintf("umask 077 && mkdir -p %s/bin %s && chmod 700 %s %s && rm -f %s",
		remoteDir, remoteTLSDir, remoteDir, remoteTLSDir, remoteKeyFile)
	if _, err := d.sshExec(base, remote, prep); err != nil {
		return fail("prepare", err)
	}
	for _, f := range []struct{ local, remote, step string }{
		{t.binary, remoteBin, "upload binary"},
		{d.caFile, remoteCAFile, "upload ca"},
		{m.TLSCertFile, remoteCertFile, "upload certificate"},
		{m.TLSKeyFile, remoteKeyFile, "upload key"},
	} {
		if err := d.scpFile(base, f.local, remote, f.remote); err != nil {
			return fail(f.step, err)
		}
	}
	perms := fmt.Sprintf("chmod 700 %s && chmod 600 %s && chmod 644 %s %s", remoteBin, remoteKeyFile, remoteCertFile, remoteCAFile)
	if _, err := d.sshExec(base, remote, perms); err != nil {
		return fail("permissions", err)
	}

	adminListen := strings.TrimSpace(m.AdminListen)
	if adminListen == "" {
		adminListen = "0.0.0.0:7010"
	}
	configContent, err := d.generateConfig(m, home, adminListen)
	if err != nil {
		return fail("config", err)
	}
	cfgFile, err := os.CreateTemp("", "ghostctl-config-*.toml")
	if err != nil {
		return fail("config", err)
	}
	defer os.Remove(cfgFile.Name())
	if _, err := cfgFile.WriteString(configContent); err != nil {
		cfgFile.Close()
		return fail("config", err)
	}
	cfgFile.Close()
	if err := d.scpFile(base, cfgFile.Name(), remote, remoteConfig); err != nil {
		return fail("upload config", err)
	}

	if force {
		_, _ = d.sshExec(base, remote, "pkill -f 'edgectl/bin/ghostctl' || true")
	}
	start := fmt.Sprintf(`nohup env PATH="$HOME/.edgectl/local/bin:$PATH" ~/%s -config ~/%s > ~/.edgectl/ghostctl.log 2>&1 &`, remoteBin, remoteConfig)
	if _, err := d.sshExec(base, remote, start); err != nil {
		return fail("start", err)
	}

	adminAddr := fmt.Sprintf("%s:%s", m.Host, extractPort(adminListen))
	logs.Infof("ssh_deploy: ghost started ghost_id=%q admin_addr=%q", m.GhostID, adminAddr)
	return DeployGhostResult{GhostID: m.GhostID, Host: m.Host, AdminAddr: adminAddr, Status: "deployed", Message: "ghost process started via nohup"}, nil
}

// Writes a 0600 temp known_hosts file holding only the pinned key for host:port.
func writeKnownHosts(host string, port int, hostKey string) (string, error) {
	f, err := os.CreateTemp("", "edgectl-known-hosts-*")
	if err != nil {
		return "", err
	}
	name := host
	if port != 22 {
		name = fmt.Sprintf("[%s]:%d", host, port)
	}
	_, werr := fmt.Fprintf(f, "%s %s\n", name, hostKey)
	cerr := f.Close()
	if werr != nil || cerr != nil {
		os.Remove(f.Name())
		return "", fmt.Errorf("write known_hosts: %v %v", werr, cerr)
	}
	return f.Name(), nil
}

// Common ssh flags: pinned host key only (no TOFU, no system known_hosts), no prompts, key auth, port.
func sshBaseArgs(keyFile string, knownHosts string, port int) []string {
	return []string{
		"-o", "StrictHostKeyChecking=yes",
		"-o", "UserKnownHostsFile=" + knownHosts,
		"-o", "GlobalKnownHostsFile=/dev/null",
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=10",
		"-i", keyFile,
		"-p", fmt.Sprintf("%d", port),
	}
}

// Runs one command on the target; returns stdout.
func (d *SSHDeployer) sshExec(baseArgs []string, target string, cmd string) ([]byte, error) {
	args := append(append([]string{}, baseArgs...), target, cmd)
	logs.Infof("ssh_deploy: ssh %s %q", target, cmd)
	stdout, stderr, exitCode, err := d.runner.Run("ssh", args...)
	if err != nil {
		return nil, fmt.Errorf("ssh exit=%d stderr=%q: %w", exitCode, strings.TrimSpace(string(stderr)), err)
	}
	return stdout, nil
}

// Uploads a local file to a path relative to the target home. Only paths are logged.
func (d *SSHDeployer) scpFile(baseArgs []string, localPath string, target string, remotePath string) error {
	// scp uses -P for port, not -p.
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
		return fmt.Errorf("scp exit=%d stderr=%q: %w", exitCode, strings.TrimSpace(string(stderr)), err)
	}
	return nil
}

// Keys the deployer always sets, overriding any template value, so a template cannot point the Ghost
// at other credentials or unpin Mirage.
func (d *SSHDeployer) identityKeys(m GhostManifest, home string) map[string]any {
	cert, key, ca := home+"/"+remoteCertFile, home+"/"+remoteKeyFile, home+"/"+remoteCAFile
	return map[string]any{
		"admin_tls_cert_file":      cert,
		"admin_tls_key_file":       key,
		"admin_tls_ca_file":        ca,
		"admin_tls_denied_serials": append([]string{}, d.deniedSerials...),
		"mirage_principal":         d.miragePrincipal.String(),
		"mirage_tls_cert_file":     cert,
		"mirage_tls_key_file":      key,
		"mirage_tls_ca_file":       ca,
		"mirage_peer_identity":     m.GhostID,
	}
}

// Produces the Ghost config: template (if any) or minimal defaults, overlaid with manifest values and
// identity keys. It carries paths to delivered credentials, never key material or shared tokens.
func (d *SSHDeployer) generateConfig(m GhostManifest, home string, adminListen string) (string, error) {
	cfg := make(map[string]any)
	if templatePath := strings.TrimSpace(m.ConfigTemplatePath); templatePath != "" {
		if _, err := toml.DecodeFile(templatePath, &cfg); err != nil {
			return "", fmt.Errorf("load config template %q: %w", templatePath, err)
		}
		if cfg == nil {
			cfg = make(map[string]any)
		}
	}
	seeds := m.Seeds
	if len(seeds) == 0 {
		seeds = []string{"seed.flow", "seed.host"}
	}
	policy := strings.TrimSpace(m.MiragePolicy)
	if policy == "" {
		policy = "auto"
	}
	cfg["id"] = m.GhostID
	cfg["admin_listen"] = adminListen
	cfg["seeds"] = seeds
	cfg["mirage_policy"] = policy
	cfg["project_fetch_on_boot"] = false
	if addr := strings.TrimSpace(m.MirageAddress); addr != "" {
		cfg["mirage_address"] = addr
	}
	// Removed shared-secret keys never ship; ghostctl refuses configs that carry them.
	delete(cfg, "admin_frame_auth_token")
	for k, v := range d.identityKeys(m, home) {
		cfg[k] = v
	}
	applySeedInstallTemplateOverrides(cfg, m)
	var out strings.Builder
	if err := toml.NewEncoder(&out).Encode(cfg); err != nil {
		return "", fmt.Errorf("encode ghost config: %w", err)
	}
	return out.String(), nil
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

func applySeedInstallTemplateOverrides(cfg map[string]any, m GhostManifest) {
	if !m.SeedInstallEnabled {
		cfg["seed_install_enabled"] = false
		delete(cfg, "seed_install")
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
	// Install specs never ride in Ghost config; drop any a template carried.
	delete(cfg, "seed_install")
}

// extractPort returns the port portion of a host:port address.
func extractPort(addr string) string {
	if idx := strings.LastIndex(addr, ":"); idx >= 0 {
		return addr[idx+1:]
	}
	return "7010"
}
