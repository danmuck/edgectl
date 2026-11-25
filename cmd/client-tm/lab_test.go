package main

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/danmuck/edgectl/internal/adminauth"
	"github.com/danmuck/edgectl/internal/identity"
)

// Lab environment rooted in a temp dir with fixed addresses, without building or starting anything.
func newTestLab(t *testing.T) *labEnv {
	t.Helper()
	dir := t.TempDir()
	l := &labEnv{
		dir: dir, displayDir: "local/lab",
		mirageAdminAddr: "127.0.0.1:7020", apiV1Addr: "127.0.0.1:7021", sessionAddr: "127.0.0.1:9000", ghostAdminAddr: "127.0.0.1:7010",
	}
	if err := l.resetCerts(); err != nil {
		t.Fatalf("reset certs: %v", err)
	}
	return l
}

// The lab PKI yields usable server identities for Mirage and the managed Ghost, an operator
// identity for the client, and can add a child Ghost certificate later.
func TestLabCertsAreUsableIdentities(t *testing.T) {
	l := newTestLab(t)
	creds := func(id string) adminauth.Credentials {
		return adminauth.Credentials{
			CertFile: filepath.Join(l.certsDir(), id+".crt"),
			KeyFile:  filepath.Join(l.certsDir(), id+".key"),
			CAFile:   filepath.Join(l.certsDir(), "ca.crt"),
		}
	}
	if _, err := adminauth.ServerTLSConfig(creds(labMirageID), identity.Principal{Role: identity.RoleMirage, ID: labMirageID}); err != nil {
		t.Fatalf("mirage identity: %v", err)
	}
	if _, err := adminauth.ServerTLSConfig(creds(labGhostID), identity.Principal{Role: identity.RoleGhost, ID: labGhostID}); err != nil {
		t.Fatalf("ghost identity: %v", err)
	}
	for _, op := range []string{labOperator, labSecondOperator} {
		if _, err := adminauth.SelfFromCert(creds(op), identity.RoleOperator); err != nil {
			t.Fatalf("operator %s: %v", op, err)
		}
	}
	child := identity.Principal{Role: identity.RoleGhost, ID: labGhostID + ".g1"}
	if err := l.issue(child); err != nil {
		t.Fatalf("issue child: %v", err)
	}
	if err := l.issue(child); err != nil {
		t.Fatalf("re-issue child must replace the pair: %v", err)
	}
	if _, err := adminauth.ServerTLSConfig(creds(child.ID), child); err != nil {
		t.Fatalf("child identity: %v", err)
	}
	if info, err := os.Stat(filepath.Join(l.certsDir(), "root", "ca.key")); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("root key must exist with 0600: %v %v", info, err)
	}
}

// The generated Mirage config is valid TOML, turns admin mTLS on with lab-local certificate paths,
// and keeps the lab safe: loopback addresses, no host-service seeds, no boot-time fetch.
func TestLabMirageConfig(t *testing.T) {
	l := newTestLab(t)
	var cfg map[string]any
	if _, err := toml.Decode(l.mirageConfigTOML(), &cfg); err != nil {
		t.Fatalf("config is not valid TOML: %v", err)
	}
	for _, key := range []string{"admin_tls_cert_file", "admin_tls_key_file", "admin_tls_ca_file", "local_ghost_admin_tls_cert_file", "local_ghost_admin_tls_key_file"} {
		path, _ := cfg[key].(string)
		if !strings.HasPrefix(path, l.certsDir()) {
			t.Errorf("%s = %q: want a path under the lab certs dir", key, path)
		}
		if _, err := os.Stat(path); err != nil {
			t.Errorf("%s points at a missing file: %v", key, err)
		}
	}
	for _, key := range []string{"addr", "admin_listen_addr", "api_v1_addr", "local_ghost_admin_addr"} {
		addr, _ := cfg[key].(string)
		if !adminauth.IsLoopbackAddr(addr) {
			t.Errorf("%s = %q: lab addresses must be loopback", key, addr)
		}
	}
	if cfg["id"] != labMirageID || cfg["local_ghost_id"] != labGhostID || cfg["local_ghost_admin_operator_access"] != true {
		t.Errorf("unexpected identities or operator access: %v", cfg)
	}
	if cfg["local_ghost_project_fetch_on_boot"] != false || cfg["buildlog_persist_enabled"] != false {
		t.Errorf("lab must not fetch on boot or persist buildlogs: %v", cfg)
	}
	for _, seed := range cfg["local_ghost_seeds"].([]any) {
		if seed == "seed.mongod" || seed == "seed.docker" {
			t.Errorf("lab must not enable host-service seed %v", seed)
		}
	}
}

// Client target files point at the lab endpoints with the lab operator identity.
func TestLabClientConfigs(t *testing.T) {
	l := newTestLab(t)
	if err := l.writeClientConfigs(); err != nil {
		t.Fatal(err)
	}
	var m mirageConfigFile
	if _, err := toml.DecodeFile(l.clientMirageCfg(), &m); err != nil {
		t.Fatal(err)
	}
	if len(m.Targets) != 1 || m.Targets[0].Addr != l.mirageAdminAddr || m.Targets[0].MirageID != labMirageID {
		t.Fatalf("mirage target: %+v", m.Targets)
	}
	if !strings.HasSuffix(m.OperatorCertFile, labOperator+".crt") || m.LocalGhostAdminAddr != l.ghostAdminAddr {
		t.Fatalf("mirage client config: %+v", m)
	}
	var g ghostConfigFile
	if _, err := toml.DecodeFile(l.clientGhostCfg(), &g); err != nil {
		t.Fatal(err)
	}
	if len(g.Targets) != 1 || g.Targets[0].GhostID != labGhostID || g.Targets[0].Addr != l.ghostAdminAddr {
		t.Fatalf("ghost target: %+v", g.Targets)
	}
}

// A taken preferred port is replaced by a free one, so the lab starts beside a running stack.
func TestFreeLoopbackPortAvoidsBusyPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	busy := ln.Addr().(*net.TCPAddr).Port
	if got := freeLoopbackPort(busy); got == busy || got <= 0 {
		t.Fatalf("expected a different free port than busy %d, got %d", busy, got)
	}
}

// Runtime log lines are condensed to the last few audit and custody rows; other lines are ignored.
func TestLabActivityLines(t *testing.T) {
	log := strings.Join([]string{
		`INFO mirage.admin listening addr="127.0.0.1:7020"`,
		`INFO admin.audit component=mirage peer="127.0.0.1:1" principal="edgectl://operator/ops" role=operator action="submit_issue" decision=allow error_code=0 timestamp_ms=1`,
		`AUDIT admin.audit component=ghost peer="127.0.0.1:2" principal="edgectl://operator/ops" role=operator action="execute_envelope" decision=deny error_code=1000 timestamp_ms=2`,
		`INFO custody component="mirage" peer="ghost.local" direction="outbound" trace_id="trace.i" request_id="cmd.i.1" message_id=2 message_type=2 status="dispatched" timestamp_ms=3`,
	}, "\n")
	audit, custody := labActivityLines(log, 8)
	if len(audit) != 2 || !strings.Contains(audit[0], "allow") || !strings.Contains(audit[0], "submit_issue") {
		t.Fatalf("audit rows: %q", audit)
	}
	if !strings.Contains(audit[1], "deny") || !strings.Contains(audit[1], "code 1000") || !strings.Contains(audit[1], "ghost") {
		t.Fatalf("deny row: %q", audit[1])
	}
	if len(custody) != 1 || !strings.Contains(custody[0], "dispatched") || !strings.Contains(custody[0], "cmd.i.1") {
		t.Fatalf("custody rows: %q", custody)
	}
	if a, c := labActivityLines("", 8); a[0] != "none yet" || c[0] != "none yet" {
		t.Fatalf("empty log: %q %q", a, c)
	}
	if a, _ := labActivityLines(strings.Repeat(strings.Split(log, "\n")[1]+"\n", 20), 3); len(a) != 3 {
		t.Fatalf("limit not applied: %d", len(a))
	}
}
