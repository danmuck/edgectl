package main

// lab.go brings up a throwaway local stack for hands-on testing (docs/progress/lab_mode.md):
// a private root CA and certificates, a miragectl child process with its managed Ghost, and the
// client configs that point at them. Everything lives under one directory and is safe to delete.

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/danmuck/edgectl/internal/identity"
	"github.com/danmuck/edgectl/internal/logging"
	"github.com/danmuck/edgectl/internal/pki"
)

const (
	labDefaultDir = "local/lab"
	labMirageID   = "mirage.lab"
	labGhostID    = "ghost.local"
	labOperator   = "ops"
	// Second operator identity, for trying two-operator dependency approvals by hand.
	labSecondOperator = "lead"

	labMirageAdminPort = 7020
	labSessionPort     = 9000
	labGhostAdminPort  = 7010
	// Admin ports for Ghosts added from the menu start here.
	labChildPortBase = 7030
)

// Seeds on every lab Ghost. seed.mongod and seed.docker are left out: they act on host services.
var labSeeds = []string{"seed.flow", "seed.kv", "seed.fs", "seed.host"}

// One running lab: where its files are, which loopback addresses it uses, and the Mirage process.
type labEnv struct {
	// Absolute lab directory; also the working directory of the Mirage process.
	dir string
	// Path shown to the user (as given, usually relative to the repo root).
	displayDir string
	ca         *pki.Authority

	mirageAdminAddr string
	sessionAddr     string
	ghostAdminAddr  string

	proc    *exec.Cmd
	logFile *os.File
	// Closed when the Mirage process exits.
	exited chan struct{}
}

func (l *labEnv) certsDir() string        { return filepath.Join(l.dir, "certs") }
func (l *labEnv) logPath() string         { return filepath.Join(l.dir, "mirage.log") }
func (l *labEnv) pidPath() string         { return filepath.Join(l.dir, "mirage.pid") }
func (l *labEnv) mirageBinary() string    { return filepath.Join(l.dir, "bin", "miragectl") }
func (l *labEnv) mirageConfig() string    { return filepath.Join(l.dir, "mirage.toml") }
func (l *labEnv) clientGhostCfg() string  { return filepath.Join(l.dir, "client.ghost.toml") }
func (l *labEnv) clientMirageCfg() string { return filepath.Join(l.dir, "client.mirage.toml") }

// Path under the lab directory in the form the user typed it, for display.
func (l *labEnv) display(parts ...string) string {
	return filepath.Join(append([]string{l.displayDir}, parts...)...)
}

// Prepares a lab directory: stops a lab left behind by a crashed client, replaces the throwaway
// PKI, reserves loopback ports, and writes the Mirage and client configs. Must run from the repo
// root because the Mirage binary is built from source there.
func prepareLab(dir string) (*labEnv, error) {
	if _, err := os.Stat("go.mod"); err != nil {
		return nil, errors.New("lab mode must run from the repo root (go.mod not found)")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	l := &labEnv{dir: abs, displayDir: filepath.Clean(dir)}
	if err := os.MkdirAll(l.dir, 0o755); err != nil {
		return nil, err
	}
	l.stopStale()
	if err := l.resetCerts(); err != nil {
		return nil, fmt.Errorf("lab certificates: %w", err)
	}
	l.mirageAdminAddr = loopbackAddr(freeLoopbackPort(labMirageAdminPort))
	l.sessionAddr = loopbackAddr(freeLoopbackPort(labSessionPort))
	l.ghostAdminAddr = loopbackAddr(freeLoopbackPort(labGhostAdminPort))
	if err := os.WriteFile(l.mirageConfig(), []byte(l.mirageConfigTOML()), 0o644); err != nil {
		return nil, err
	}
	if err := l.writeClientConfigs(); err != nil {
		return nil, err
	}
	return l, nil
}

// Replaces the lab PKI with a fresh root and the fixed lab identities. The lab directory is the
// only place written; a deployment's certs/ is never read or touched.
func (l *labEnv) resetCerts() error {
	if err := os.RemoveAll(l.certsDir()); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(l.certsDir(), "root"), 0o700); err != nil {
		return err
	}
	ca, der, err := pki.NewRoot("edgectl-lab-root", 30, time.Now())
	if err != nil {
		return err
	}
	l.ca = ca
	for _, path := range []string{filepath.Join(l.certsDir(), "root", "ca.crt"), filepath.Join(l.certsDir(), "ca.crt")} {
		if err := pki.WriteCertPEM(path, der); err != nil {
			return err
		}
	}
	if err := pki.WriteKeyPEM(filepath.Join(l.certsDir(), "root", "ca.key"), ca.Key); err != nil {
		return err
	}
	leaves := []identity.Principal{
		{Role: identity.RoleMirage, ID: labMirageID},
		{Role: identity.RoleGhost, ID: labGhostID},
		{Role: identity.RoleOperator, ID: labOperator},
		{Role: identity.RoleOperator, ID: labSecondOperator},
	}
	for _, p := range leaves {
		if err := l.issue(p); err != nil {
			return err
		}
	}
	return nil
}

// Issues <certs>/<id>.crt and .key for one principal, valid on loopback, replacing any earlier pair.
func (l *labEnv) issue(p identity.Principal) error {
	if err := p.Validate(); err != nil {
		return err
	}
	leaf, err := l.ca.IssueLeaf(p, []string{"localhost"}, []net.IP{net.ParseIP("127.0.0.1")}, 30, time.Now())
	if err != nil {
		return err
	}
	certPath := filepath.Join(l.certsDir(), p.ID+".crt")
	keyPath := filepath.Join(l.certsDir(), p.ID+".key")
	_ = os.Remove(certPath)
	_ = os.Remove(keyPath)
	if err := pki.WriteCertPEM(certPath, leaf.DER); err != nil {
		return err
	}
	return pki.WriteKeyPEM(keyPath, leaf.Key)
}

// Mirage config for the lab: admin mTLS on, managed Ghost with operator read access so the client
// can show command output, development-mode sessions, no remote preloads, installs untouched (off).
func (l *labEnv) mirageConfigTOML() string {
	q := strconv.Quote
	cert := func(name string) string { return q(filepath.Join(l.certsDir(), name)) }
	seedList := make([]string, 0, len(labSeeds))
	for _, s := range labSeeds {
		seedList = append(seedList, q(s))
	}
	lines := []string{
		"# Written by client-tm lab mode on every start; edits are overwritten.",
		"id = " + q(labMirageID),
		"addr = " + q(l.sessionAddr),
		"admin_listen_addr = " + q(l.mirageAdminAddr),
		"admin_tls_cert_file = " + cert(labMirageID+".crt"),
		"admin_tls_key_file = " + cert(labMirageID+".key"),
		"admin_tls_ca_file = " + cert("ca.crt"),
		"session_security_mode = \"development\"",
		"require_identity_binding = true",
		"buildlog_persist_enabled = false",
		"local_ghost_id = " + q(labGhostID),
		"local_ghost_admin_addr = " + q(l.ghostAdminAddr),
		"local_ghost_admin_tls_cert_file = " + cert(labGhostID+".crt"),
		"local_ghost_admin_tls_key_file = " + cert(labGhostID+".key"),
		"local_ghost_admin_operator_access = true",
		"local_ghost_seeds = [" + strings.Join(seedList, ", ") + "]",
		"local_ghost_heartbeat_interval = \"5s\"",
		"local_ghost_project_fetch_on_boot = false",
	}
	return strings.Join(lines, "\n") + "\n"
}

// Client target files for the lab, kept in the lab directory so the tracked client configs under
// cmd/client-tm are never rewritten.
func (l *labEnv) writeClientConfigs() error {
	q := strconv.Quote
	operator := strings.Join([]string{
		"operator_cert_file = " + q(filepath.Join(l.certsDir(), labOperator+".crt")),
		"operator_key_file = " + q(filepath.Join(l.certsDir(), labOperator+".key")),
		"ca_file = " + q(filepath.Join(l.certsDir(), "ca.crt")),
	}, "\n") + "\n"
	ghostCfg := operator + "clear_screen_after_command = false\n\n[[targets]]\n" +
		"  name = \"lab-ghost\"\n  addr = " + q(l.ghostAdminAddr) + "\n  ghost_id = " + q(labGhostID) + "\n"
	mirageCfg := operator + "local_ghost_id = " + q(labGhostID) + "\nlocal_ghost_admin_addr = " + q(l.ghostAdminAddr) +
		"\n\n[[targets]]\n  name = \"lab-mirage\"\n  addr = " + q(l.mirageAdminAddr) + "\n  mirage_id = " + q(labMirageID) + "\n"
	if err := os.WriteFile(l.clientGhostCfg(), []byte(ghostCfg), 0o644); err != nil {
		return err
	}
	return os.WriteFile(l.clientMirageCfg(), []byte(mirageCfg), 0o644)
}

// Builds miragectl from this checkout so the lab always runs the code in the working tree.
func (l *labEnv) build() error {
	if err := os.MkdirAll(filepath.Dir(l.mirageBinary()), 0o755); err != nil {
		return err
	}
	out, err := exec.Command("go", "build", "-o", l.mirageBinary(), "./cmd/miragectl").CombinedOutput()
	if err != nil {
		return fmt.Errorf("go build ./cmd/miragectl: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Starts Mirage (and its managed Ghost) with the lab directory as working directory, so seed.fs
// data and logs stay inside it. Output goes to the lab log, uncolored for later display.
func (l *labEnv) start() error {
	logFile, err := os.Create(l.logPath())
	if err != nil {
		return err
	}
	cmd := exec.Command(l.mirageBinary(), "-config", l.mirageConfig())
	cmd.Dir = l.dir
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Env = append(os.Environ(), logging.EnvLogNoColor+"=true", logging.EnvLogLevel+"=info")
	// Own process group: Ctrl-C in the terminal reaches the client, which then stops Mirage itself.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		logFile.Close()
		return err
	}
	l.proc, l.logFile, l.exited = cmd, logFile, make(chan struct{})
	_ = os.WriteFile(l.pidPath(), []byte(strconv.Itoa(cmd.Process.Pid)), 0o644)
	go func() {
		_ = cmd.Wait()
		close(l.exited)
	}()
	return nil
}

// Blocks until Mirage answers status with its managed Ghost attached, the process dies, or the
// timeout passes. ready is polled; its error is reported if the lab never comes up.
func (l *labEnv) waitReady(ready func() error, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var last error
	for time.Now().Before(deadline) {
		select {
		case <-l.exited:
			return fmt.Errorf("mirage exited during startup; last log lines:\n%s", indentLines(tailFile(l.logPath(), 12), "  "))
		default:
		}
		if last = ready(); last == nil {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("lab did not become ready in %s: %v; see %s", timeout, last, l.display("mirage.log"))
}

// Stops the Mirage process: SIGTERM, then SIGKILL after a grace period. Safe to call twice.
func (l *labEnv) stop() {
	if l == nil || l.proc == nil || l.proc.Process == nil {
		return
	}
	_ = l.proc.Process.Signal(syscall.SIGTERM)
	select {
	case <-l.exited:
	case <-time.After(3 * time.Second):
		_ = l.proc.Process.Kill()
		<-l.exited
	}
	if l.logFile != nil {
		_ = l.logFile.Close()
	}
	_ = os.Remove(l.pidPath())
	l.proc = nil
}

// Stops a Mirage left running by a previous lab client that did not exit cleanly. Only a process
// whose command line is this lab's own binary is signalled.
func (l *labEnv) stopStale() {
	raw, err := os.ReadFile(l.pidPath())
	if err != nil {
		return
	}
	defer os.Remove(l.pidPath())
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil || pid <= 1 {
		return
	}
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "command=").Output()
	if err != nil || !strings.Contains(string(out), l.mirageBinary()) {
		return
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return
	}
	_ = proc.Signal(syscall.SIGTERM)
	for i := 0; i < 30; i++ {
		if proc.Signal(syscall.Signal(0)) != nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	_ = proc.Kill()
}

// Admin port for a Ghost added from the menu: first free loopback port from the child base.
func (l *labEnv) nextChildAddr() string {
	for port := labChildPortBase; port < labChildPortBase+200; port++ {
		if loopbackPortFree(port) {
			return loopbackAddr(port)
		}
	}
	return loopbackAddr(freeLoopbackPort(0))
}

func loopbackAddr(port int) string {
	return net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
}

func loopbackPortFree(port int) bool {
	ln, err := net.Listen("tcp", loopbackAddr(port))
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}

// Preferred port when it is free, otherwise any free loopback port, so the lab starts next to a
// stack the user already has running.
func freeLoopbackPort(preferred int) int {
	if preferred > 0 && loopbackPortFree(preferred) {
		return preferred
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return preferred
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

// Last n lines of a file, or a short note when it cannot be read.
func tailFile(path string, n int) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "(no log: " + err.Error() + ")"
	}
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
