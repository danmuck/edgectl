package mirage

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/danmuck/edgectl/internal/adminauth"
	"github.com/danmuck/edgectl/internal/identity"
	"github.com/danmuck/edgectl/internal/protocol/schema"
	"github.com/danmuck/edgectl/internal/testutil/testlog"
	"github.com/danmuck/edgectl/internal/testutil/tlstest"
)

const testHostKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGl0IGlzIG5vdCBhIHJlYWwga2V5LCBqdXN0IGJhc2U2NA=="

// Records ssh/scp invocations and snapshots files they reference (temp files are removed after Deploy).
type fakeSSH struct {
	calls      [][]string
	knownHosts []string
	uploads    map[string]string // remote path -> local content at upload time
}

func (f *fakeSSH) Run(name string, args ...string) ([]byte, []byte, uint32, error) {
	f.calls = append(f.calls, append([]string{name}, args...))
	for _, a := range args {
		if strings.HasPrefix(a, "UserKnownHostsFile=") {
			raw, _ := os.ReadFile(strings.TrimPrefix(a, "UserKnownHostsFile="))
			f.knownHosts = append(f.knownHosts, string(raw))
		}
	}
	if name == "scp" {
		local, dest := args[len(args)-2], args[len(args)-1]
		raw, _ := os.ReadFile(local)
		f.uploads[dest[strings.Index(dest, ":")+1:]] = string(raw)
	}
	if name == "ssh" && strings.Contains(args[len(args)-1], `"$HOME"`) {
		return []byte("/home/deploy"), nil, 0, nil
	}
	return nil, nil, 0, nil
}

type deployHarness struct {
	svc        *Service
	leafSerial string
	ssh      *fakeSSH
	pki      *tlstest.PKI
	manifest GhostManifest
}

// Mirage with admin TLS and one valid manifest for ghost.pi.
func newDeployHarness(t *testing.T) *deployHarness {
	t.Helper()
	p := tlstest.NewPKI(t)
	mCert, mKey, _ := p.Issue(t, identity.RoleMirage, "mirage.main")
	gCert, gKey, gSerial := p.Issue(t, identity.RoleGhost, "ghost.pi")
	binary := filepath.Join(t.TempDir(), "ghostctl")
	if err := os.WriteFile(binary, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	m := GhostManifest{
		GhostID: "ghost.pi", Host: "10.0.0.20", User: "deploy", SSHPort: 2222, SSHHostKey: testHostKey,
		TLSCertFile: gCert, TLSKeyFile: gKey, BinaryPath: binary, MirageAddress: "10.0.0.7:9000",
	}
	cfg := DefaultServiceConfig()
	cfg.MirageID = "mirage.main"
	cfg.LocalGhostAdminAddr = ""
	cfg.AdminTLS = adminauth.Credentials{CertFile: mCert, KeyFile: mKey, CAFile: p.CAFile()}
	cfg.GhostManifests = []GhostManifest{m}
	svc := NewServiceWithConfig(cfg)
	f := &fakeSSH{uploads: map[string]string{}}
	svc.sshRunner = f
	return &deployHarness{svc: svc, leafSerial: gSerial, ssh: f, pki: p, manifest: m}
}

// Deploy uses only the manifest: pinned host key with strict checking, the Ghost's own credentials
// delivered under a 0700 dir with a 0600 key, Mirage pinned, and no secret in config or result.
func TestDeployGhostFromManifestWithPinnedHostKey(t *testing.T) {
	testlog.Start(t)
	h := newDeployHarness(t)
	result, err := h.svc.DeployGhost(context.Background(), DeployGhostRequest{GhostID: "ghost.pi"})
	if err != nil {
		t.Fatalf("deploy: %v", err)
	}
	if result.Host != "10.0.0.20" || result.Status != "deployed" {
		t.Fatalf("result: %+v", result)
	}
	for _, call := range h.ssh.calls {
		joined := strings.Join(call, " ")
		if !strings.Contains(joined, "StrictHostKeyChecking=yes") || !strings.Contains(joined, "GlobalKnownHostsFile=/dev/null") ||
			strings.Contains(joined, "accept-new") || !strings.Contains(joined, "deploy@10.0.0.20") {
			t.Fatalf("call without strict pinned host key or to another host: %v", call)
		}
	}
	for _, kh := range h.ssh.knownHosts {
		if kh != "[10.0.0.20]:2222 "+testHostKey+"\n" {
			t.Fatalf("known_hosts = %q", kh)
		}
	}

	keyPEM, _ := os.ReadFile(h.manifest.TLSKeyFile)
	if h.ssh.uploads[remoteKeyFile] != string(keyPEM) {
		t.Fatalf("ghost key not delivered to %s", remoteKeyFile)
	}
	caPEM, _ := os.ReadFile(h.pki.CAFile())
	if h.ssh.uploads[remoteCAFile] != string(caPEM) {
		t.Fatalf("root CA not delivered")
	}
	var prepIdx, keyIdx, chmodIdx = -1, -1, -1
	for i, call := range h.ssh.calls {
		last := call[len(call)-1]
		switch {
		case strings.Contains(last, "umask 077") && strings.Contains(last, "chmod 700 .edgectl .edgectl/tls"):
			prepIdx = i
		case strings.HasSuffix(last, ":"+remoteKeyFile):
			keyIdx = i
		case strings.Contains(last, "chmod 600 "+remoteKeyFile):
			chmodIdx = i
		}
	}
	if !(prepIdx >= 0 && prepIdx < keyIdx && keyIdx < chmodIdx) {
		t.Fatalf("key must land in a 0700 dir and be chmod 600: prep=%d key=%d chmod=%d", prepIdx, keyIdx, chmodIdx)
	}

	conf := h.ssh.uploads[remoteConfig]
	for _, want := range []string{
		`admin_tls_key_file = "/home/deploy/.edgectl/tls/ghost.key"`,
		`admin_tls_ca_file = "/home/deploy/.edgectl/tls/ca.crt"`,
		`mirage_principal = "edgectl://mirage/mirage.main"`,
		`id = "ghost.pi"`,
	} {
		if !strings.Contains(conf, want) {
			t.Fatalf("config missing %s:\n%s", want, conf)
		}
	}
	raw, _ := json.Marshal(result)
	for _, secret := range []string{"PRIVATE KEY", "admin_frame_auth_token"} {
		if strings.Contains(conf, secret) || strings.Contains(string(raw), secret) {
			t.Fatalf("%q leaked into config or result", secret)
		}
	}
}

// Only configured ghost_ids deploy, and request JSON cannot supply a host.
func TestDeployGhostRejectsUnknownAndIgnoresRequestHost(t *testing.T) {
	testlog.Start(t)
	h := newDeployHarness(t)
	resp := h.svc.handleAdminControlRequest(adminControlRequest{Action: "deploy_ghost", Deploy: DeployGhostRequest{GhostID: "ghost.unknown"}})
	if resp.OK || resp.ErrorCode != schema.ErrCodeSemanticValidation || !strings.Contains(resp.Error, ErrUnknownDeployTarget.Error()) {
		t.Fatalf("unknown target: ok=%v code=%d err=%s", resp.OK, resp.ErrorCode, resp.Error)
	}
	if len(h.ssh.calls) != 0 {
		t.Fatalf("unknown target must not reach ssh: %v", h.ssh.calls)
	}

	var req adminControlRequest
	wire := `{"action":"deploy_ghost","deploy":{"ghost_id":"ghost.pi","manifest":{"host":"evil.example","user":"root"},"host":"evil.example"}}`
	if err := json.Unmarshal([]byte(wire), &req); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.DeployGhost(context.Background(), req.Deploy); err != nil {
		t.Fatalf("deploy: %v", err)
	}
	for _, call := range h.ssh.calls {
		if strings.Contains(strings.Join(call, " "), "evil.example") || strings.Contains(strings.Join(call, " "), "root@") {
			t.Fatalf("request-supplied target used: %v", call)
		}
	}
}

// Manifests without a valid pinned key, safe host, or this Ghost's own valid leaf are refused before ssh.
func TestDeployGhostRejectsInvalidManifests(t *testing.T) {
	testlog.Start(t)
	h := newDeployHarness(t)
	otherCert, otherKey, _ := h.pki.Issue(t, identity.RoleGhost, "ghost.other")
	foreign := tlstest.NewPKI(t)
	fCert, fKey, _ := foreign.Issue(t, identity.RoleGhost, "ghost.pi")
	looseKey := filepath.Join(t.TempDir(), "loose.key")
	keyPEM, _ := os.ReadFile(h.manifest.TLSKeyFile)
	if err := os.WriteFile(looseKey, keyPEM, 0o644); err != nil {
		t.Fatal(err)
	}

	cases := map[string]func(m *GhostManifest){
		"no host key":        func(m *GhostManifest) { m.SSHHostKey = "" },
		"multi-line key":     func(m *GhostManifest) { m.SSHHostKey = testHostKey + "\nevil.example " + testHostKey },
		"unknown key type":   func(m *GhostManifest) { m.SSHHostKey = "ssh-dss AAAA" },
		"option-like host":   func(m *GhostManifest) { m.Host = "-oProxyCommand=sh" },
		"bad user":           func(m *GhostManifest) { m.User = "root;id" },
		"other ghost's cert": func(m *GhostManifest) { m.TLSCertFile, m.TLSKeyFile = otherCert, otherKey },
		"foreign CA":         func(m *GhostManifest) { m.TLSCertFile, m.TLSKeyFile = fCert, fKey },
		"key not 0600":       func(m *GhostManifest) { m.TLSKeyFile = looseKey },
		"no credentials":     func(m *GhostManifest) { m.TLSCertFile, m.TLSKeyFile = "", "" },
	}
	for name, mutate := range cases {
		m := h.manifest
		mutate(&m)
		h.svc.cfg.GhostManifests = []GhostManifest{m}
		if _, err := h.svc.DeployGhost(context.Background(), DeployGhostRequest{GhostID: "ghost.pi"}); !errors.Is(err, ErrInvalidDeployManifest) {
			t.Errorf("%s: expected ErrInvalidDeployManifest, got %v", name, err)
		}
	}
	// A denied serial on the Ghost leaf is refused.
	h.svc.cfg.GhostManifests = []GhostManifest{h.manifest}
	h.svc.cfg.AdminTLS.DeniedSerials = []string{h.leafSerial}
	if _, err := h.svc.DeployGhost(context.Background(), DeployGhostRequest{GhostID: "ghost.pi"}); !errors.Is(err, ErrInvalidDeployManifest) {
		t.Errorf("denied leaf: expected ErrInvalidDeployManifest, got %v", err)
	}
	// Without admin TLS there is no CA to deliver or Mirage principal to pin.
	h.svc.cfg.AdminTLS = adminauth.Credentials{}
	if _, err := h.svc.DeployGhost(context.Background(), DeployGhostRequest{GhostID: "ghost.pi"}); !errors.Is(err, ErrInvalidDeployManifest) {
		t.Errorf("no admin TLS: expected ErrInvalidDeployManifest, got %v", err)
	}
	if len(h.ssh.calls) != 0 {
		t.Fatalf("invalid manifests must not reach ssh: %v", h.ssh.calls)
	}
}

// Templates can shape the config but never redirect credentials, unpin Mirage, or carry tokens/specs.
func TestGenerateConfigTemplateCannotOverrideIdentity(t *testing.T) {
	template := filepath.Join(t.TempDir(), "pi.tls.config.toml")
	content := `
id = "ghost.base"
admin_listen = "0.0.0.0:7011"
mirage_policy = "required"
admin_frame_auth_token = "s3cret"
admin_tls_key_file = "/tmp/other.key"
mirage_principal = "edgectl://mirage/evil"
seed_install_enabled = false

[[seed_install]]
seed_id = "seed.stale"
method = "brew"
package = "stale"
`
	if err := os.WriteFile(template, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	d := NewSSHDeployer("ca.crt", identity.Principal{Role: identity.RoleMirage, ID: "mirage.main"}, nil)
	cfg, err := d.generateConfig(GhostManifest{
		GhostID: "ghost.pi.alpha", MirageAddress: "10.0.0.7:9000", SeedInstallEnabled: true,
		SeedInstallAllowInternalDefaults: true, ConfigTemplatePath: template,
	}, "/home/deploy", "0.0.0.0:7021")
	if err != nil {
		t.Fatalf("generate config: %v", err)
	}
	for _, want := range []string{
		`id = "ghost.pi.alpha"`, `admin_listen = "0.0.0.0:7021"`, `mirage_policy = "auto"`,
		`admin_tls_key_file = "/home/deploy/.edgectl/tls/ghost.key"`, `mirage_principal = "edgectl://mirage/mirage.main"`,
		`mirage_peer_identity = "ghost.pi.alpha"`, `seed_install_enabled = true`,
		`seed_install_whitelist = ["seed.docker", "seed.flow", "seed.fs", "seed.host", "seed.kv", "seed.mongod"]`,
	} {
		if !strings.Contains(cfg, want) {
			t.Fatalf("expected %s, got:\n%s", want, cfg)
		}
	}
	for _, banned := range []string{"admin_frame_auth_token", "/tmp/other.key", "mirage/evil", "seed_id"} {
		if strings.Contains(cfg, banned) {
			t.Fatalf("generated config must not contain %q:\n%s", banned, cfg)
		}
	}
}
