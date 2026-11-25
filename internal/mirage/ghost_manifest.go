package mirage

import (
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/danmuck/edgectl/internal/identity"
)

var (
	// deploy_ghost named a ghost_id with no configured manifest (admin_security.toml deploy.unknown_target).
	ErrUnknownDeployTarget = errors.New("mirage: unknown deploy target")
	// Manifest or delivered credentials do not satisfy admin_security.toml [deploy].
	ErrInvalidDeployManifest = errors.New("mirage: invalid deploy manifest")
)

// GhostManifest declares the desired state of one remote Ghost deployment target. It is the only
// source of where and how a Ghost is deployed (admin_security.toml [deploy]): host, user, ssh_port,
// the pinned SSH host key, and the Ghost's own leaf credentials. Seed-install fields configure whether
// the Ghost accepts approved install plans and where it may install; install specs are never shipped
// in Ghost config (internal/depcatalog is the source).
type GhostManifest struct {
	GhostID    string `json:"ghost_id" toml:"ghost_id"`
	Host       string `json:"host" toml:"host"`
	User       string `json:"user" toml:"user"`
	SSHKeyFile string `json:"ssh_key" toml:"ssh_key"`
	SSHPort    int    `json:"ssh_port" toml:"ssh_port"`
	// Pinned SSH host public key, "<type> <base64>" as in known_hosts (deploy.ssh_host_key).
	SSHHostKey string `json:"ssh_host_key" toml:"ssh_host_key"`
	// This Ghost's own leaf certificate and key on the Mirage host (deploy.credentials_delivered).
	TLSCertFile string `json:"tls_cert_file" toml:"tls_cert_file"`
	TLSKeyFile  string `json:"tls_key_file" toml:"tls_key_file"`
	// Local ghostctl binary uploaded to the target; default local/bin/ghostctl.
	BinaryPath                       string   `json:"binary_path,omitempty" toml:"binary_path"`
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

// DeployGhostRequest names a configured target; everything else comes from its manifest
// (admin_security.toml deploy.targets).
type DeployGhostRequest struct {
	GhostID       string `json:"ghost_id"`
	ForceRedeploy bool   `json:"force_redeploy"`
}

// DeployGhostResult reports the outcome of one remote ghost deployment. It never carries key material
// (deploy.key_transfer).
type DeployGhostResult struct {
	GhostID   string `json:"ghost_id"`
	Host      string `json:"host"`
	AdminAddr string `json:"admin_addr"`
	Status    string `json:"status"`
	Message   string `json:"message,omitempty"`
}

var (
	// Hostname or IPv4 literal; no leading '-' so it can never be read as an ssh option. IPv6 literals
	// are not accepted (scp target syntax would need brackets).
	deployHostPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.\-]*$`)
	deployUserPattern = regexp.MustCompile(`^[a-z_][a-z0-9_\-]{0,31}$`)
	sshHostKeyTypes   = map[string]struct{}{
		"ssh-ed25519": {}, "ecdsa-sha2-nistp256": {}, "ecdsa-sha2-nistp384": {}, "ecdsa-sha2-nistp521": {}, "ssh-rsa": {},
		"rsa-sha2-256": {}, "rsa-sha2-512": {},
	}
)

// Resolved, validated deploy target. Built only from a configured manifest.
type deployTarget struct {
	manifest GhostManifest
	user     string
	port     int
	hostKey  string
	keyFile  string
	binary   string
}

// Validates m against admin_security.toml [deploy]: safe host/user, pinned host key, and this Ghost's
// own leaf (0600 key matching the certificate, principal edgectl://ghost/<ghost_id>, chaining to
// roots and not denied).
func resolveDeployTarget(m GhostManifest, roots *x509.CertPool, denied identity.DeniedSerials) (deployTarget, error) {
	id := strings.TrimSpace(m.GhostID)
	host := strings.TrimSpace(m.Host)
	if id == "" || !deployHostPattern.MatchString(host) {
		return deployTarget{}, fmt.Errorf("%w: ghost_id and a valid host required", ErrInvalidDeployManifest)
	}
	t := deployTarget{manifest: m, user: strings.TrimSpace(m.User), port: m.SSHPort, binary: strings.TrimSpace(m.BinaryPath)}
	t.manifest.GhostID, t.manifest.Host = id, host
	if t.user == "" {
		t.user = "deploy"
	}
	if !deployUserPattern.MatchString(t.user) {
		return deployTarget{}, fmt.Errorf("%w: ghost_id=%s invalid user %q", ErrInvalidDeployManifest, id, t.user)
	}
	if t.port == 0 {
		t.port = 22
	}
	if t.port < 1 || t.port > 65535 {
		return deployTarget{}, fmt.Errorf("%w: ghost_id=%s invalid ssh_port %d", ErrInvalidDeployManifest, id, t.port)
	}
	key, err := parseSSHHostKey(m.SSHHostKey)
	if err != nil {
		return deployTarget{}, fmt.Errorf("%w: ghost_id=%s ssh_host_key: %v", ErrInvalidDeployManifest, id, err)
	}
	t.hostKey = key
	t.keyFile = strings.TrimSpace(m.SSHKeyFile)
	if t.keyFile == "" {
		home, _ := os.UserHomeDir()
		t.keyFile = home + "/.ssh/id_ed25519"
	}
	if t.binary == "" {
		t.binary = "local/bin/ghostctl"
	}
	if err := checkGhostLeaf(id, m.TLSCertFile, m.TLSKeyFile, roots, denied); err != nil {
		return deployTarget{}, err
	}
	t.manifest.TLSCertFile, t.manifest.TLSKeyFile = strings.TrimSpace(m.TLSCertFile), strings.TrimSpace(m.TLSKeyFile)
	return t, nil
}

// Accepts exactly "<type> <base64>" (an optional trailing comment is dropped); refuses anything that
// could add lines or options to the generated known_hosts file.
func parseSSHHostKey(raw string) (string, error) {
	if strings.ContainsAny(raw, "\r\n") {
		return "", errors.New("must be a single line")
	}
	fields := strings.Fields(raw)
	if len(fields) < 2 {
		return "", errors.New(`required: "<type> <base64>" from the target's known_hosts or ssh-keyscan`)
	}
	if _, ok := sshHostKeyTypes[fields[0]]; !ok {
		return "", fmt.Errorf("unsupported key type %q", fields[0])
	}
	if _, err := base64.StdEncoding.DecodeString(fields[1]); err != nil {
		return "", fmt.Errorf("key is not base64: %v", err)
	}
	return fields[0] + " " + fields[1], nil
}

// The Ghost leaf must be that Ghost's identity and deliverable: 0600 key, matching pair, chains to
// roots for both server and client use, not denied.
func checkGhostLeaf(ghostID, certFile, keyFile string, roots *x509.CertPool, denied identity.DeniedSerials) error {
	certFile, keyFile = strings.TrimSpace(certFile), strings.TrimSpace(keyFile)
	if certFile == "" || keyFile == "" {
		return fmt.Errorf("%w: ghost_id=%s tls_cert_file and tls_key_file required", ErrInvalidDeployManifest, ghostID)
	}
	pair, err := identity.LoadKeyPair(certFile, keyFile)
	if err != nil {
		return fmt.Errorf("%w: ghost_id=%s leaf: %v", ErrInvalidDeployManifest, ghostID, err)
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return fmt.Errorf("%w: ghost_id=%s leaf: %v", ErrInvalidDeployManifest, ghostID, err)
	}
	p, err := identity.RequireRole(leaf, identity.RoleGhost)
	if err != nil || p.ID != ghostID {
		return fmt.Errorf("%w: ghost_id=%s leaf principal is not edgectl://ghost/%s", ErrInvalidDeployManifest, ghostID, ghostID)
	}
	for _, usage := range []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth} {
		if _, err := leaf.Verify(x509.VerifyOptions{Roots: roots, KeyUsages: []x509.ExtKeyUsage{usage}}); err != nil {
			return fmt.Errorf("%w: ghost_id=%s leaf does not verify under the admin CA: %v", ErrInvalidDeployManifest, ghostID, err)
		}
	}
	if denied.Contains(leaf.SerialNumber) {
		return fmt.Errorf("%w: ghost_id=%s leaf serial is denied", ErrInvalidDeployManifest, ghostID)
	}
	return nil
}
