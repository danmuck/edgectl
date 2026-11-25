package main

// lab.go brings up a local stack for hands-on testing (docs/progress/lab_mode.md): a private root
// CA and certificates, a miragectl child process with its managed Ghost, and the client configs
// that point at them. Certificates and the Mirage config are kept between starts while they are
// still usable. Everything lives under one directory and is safe to delete.

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/danmuck/edgectl/internal/adminauth"
	"github.com/danmuck/edgectl/internal/identity"
	"github.com/danmuck/edgectl/internal/logging"
	"github.com/danmuck/edgectl/internal/pki"
	logs "github.com/danmuck/smplog"
)

const (
	labDefaultDir = "local/lab"
	labMirageID   = "mirage.lab"
	labGhostID    = "ghost.local"
	labOperator   = "ops"
	// Second operator identity, for trying two-operator dependency approvals by hand.
	labSecondOperator = "lead"

	labMirageAdminPort = 7020
	// Northbound api/v1 (api_v1.toml), for a web front end acting as the lab operator.
	labAPIv1Port      = 7021
	labSessionPort    = 9000
	labGhostAdminPort = 7010
	// Admin ports for Ghosts added from the menu start here.
	labChildPortBase = 7030

	// Lifetime of a freshly issued lab root and leaf, in days.
	labCertDays = 30
	// Validity a kept certificate must still have, so it does not expire during a session.
	labCertMinRemaining = 24 * time.Hour
)

// Fixed identities every lab PKI holds: Mirage, its managed Ghost, and the two operators.
func labLeaves() []identity.Principal {
	return []identity.Principal{
		{Role: identity.RoleMirage, ID: labMirageID},
		{Role: identity.RoleGhost, ID: labGhostID},
		{Role: identity.RoleOperator, ID: labOperator},
		{Role: identity.RoleOperator, ID: labSecondOperator},
	}
}

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
	apiV1Addr       string
	sessionAddr     string
	ghostAdminAddr  string

	proc *exec.Cmd
	// Lab log sink, closed after the Mirage process exits.
	logFile io.Closer
	// Closed when the Mirage process exits.
	exited chan struct{}
	// What the bootstrap kept or created, one line each, for the front end to show.
	notes []string
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

// Brings a lab directory to a startable state, keeping what a previous start left when it is still
// usable: stops a lab left behind by a crashed client, reuses or replaces the PKI, reuses or
// rewrites the Mirage config, and writes the client configs. Must run from the repo root because
// the Mirage binary is built from source there.
func ensureLab(dir string) (*labEnv, error) {
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

	now := time.Now()
	ca, expires, certErr := labCertsValid(l.certsDir(), now)
	if certErr == nil {
		l.ca = ca
		l.notef("certificates  kept (valid until %s)", expires.Local().Format("2006-01-02"))
	} else {
		if err := l.resetCerts(); err != nil {
			return nil, fmt.Errorf("lab certificates: %w", err)
		}
		l.notef("certificates  new, valid %d days (%s)", labCertDays, labBootstrapReason(l.certsDir(), certErr))
	}

	cfg, cfgErr := l.loadMirageConfig()
	if cfgErr == nil {
		l.mirageAdminAddr, l.apiV1Addr = cfg.AdminListenAddr, cfg.APIv1Addr
		l.sessionAddr, l.ghostAdminAddr = cfg.Addr, cfg.LocalGhostAdminAddr
		// A kept config is the user's: a busy port is reported, never silently moved.
		for _, addr := range []string{l.mirageAdminAddr, l.apiV1Addr, l.sessionAddr, l.ghostAdminAddr} {
			if !loopbackAddrFree(addr) {
				return nil, fmt.Errorf("%s is in use (from %s): stop what holds it, or delete that file to pick new ports",
					addr, l.display("mirage.toml"))
			}
		}
		l.notef("config        kept (%s)", l.display("mirage.toml"))
	} else {
		l.mirageAdminAddr = loopbackAddr(freeLoopbackPort(labMirageAdminPort))
		l.apiV1Addr = loopbackAddr(freeLoopbackPort(labAPIv1Port))
		l.sessionAddr = loopbackAddr(freeLoopbackPort(labSessionPort))
		l.ghostAdminAddr = loopbackAddr(freeLoopbackPort(labGhostAdminPort))
		if err := os.WriteFile(l.mirageConfig(), []byte(l.mirageConfigTOML()), 0o644); err != nil {
			return nil, err
		}
		l.notef("config        new (%s)", labBootstrapReason(l.mirageConfig(), cfgErr))
	}
	// Derived from the addresses above on every start.
	if err := l.writeClientConfigs(); err != nil {
		return nil, err
	}
	return l, nil
}

func (l *labEnv) notef(format string, args ...any) {
	l.notes = append(l.notes, fmt.Sprintf(format, args...))
}

// Short reason shown when certificates or config are created: nothing at path yet, or what was wrong.
func labBootstrapReason(path string, err error) string {
	if _, statErr := os.Stat(path); errors.Is(statErr, os.ErrNotExist) {
		return "none found"
	}
	return err.Error()
}

// Checks a kept lab PKI: the root loads and matches its key, the copy Mirage reads is the same
// root, and every fixed identity has a key pair that chains to the root, carries its principal,
// and stays valid past the session margin. Returns the root and the earliest expiry. Any error
// means the whole PKI is replaced; child Ghost certificates are not checked (reissued on add).
func labCertsValid(certsDir string, now time.Time) (*pki.Authority, time.Time, error) {
	rootPath := filepath.Join(certsDir, "root", "ca.crt")
	ca, err := pki.LoadAuthority(rootPath, filepath.Join(certsDir, "root", "ca.key"))
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("root CA: %w", err)
	}
	if pub, ok := ca.Cert.PublicKey.(*ecdsa.PublicKey); !ok || !pub.Equal(&ca.Key.PublicKey) {
		return nil, time.Time{}, errors.New("root CA: key does not match certificate")
	}
	rootPEM, err := os.ReadFile(rootPath)
	if err != nil {
		return nil, time.Time{}, err
	}
	copyPEM, err := os.ReadFile(filepath.Join(certsDir, "ca.crt"))
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("ca.crt: %w", err)
	}
	if !bytes.Equal(rootPEM, copyPEM) {
		return nil, time.Time{}, errors.New("ca.crt differs from root/ca.crt")
	}
	roots := x509.NewCertPool()
	roots.AddCert(ca.Cert)
	expires := ca.Cert.NotAfter
	for _, p := range labLeaves() {
		pair, err := identity.LoadKeyPair(filepath.Join(certsDir, p.ID+".crt"), filepath.Join(certsDir, p.ID+".key"))
		if err != nil {
			return nil, time.Time{}, fmt.Errorf("%s: %w", p.ID, err)
		}
		leaf, err := x509.ParseCertificate(pair.Certificate[0])
		if err != nil {
			return nil, time.Time{}, fmt.Errorf("%s: %w", p.ID, err)
		}
		if got, err := identity.PrincipalFromCert(leaf); err != nil || got != p {
			return nil, time.Time{}, fmt.Errorf("%s: certificate principal is %q, want %q (%v)", p.ID, got, p, err)
		}
		if _, err := leaf.Verify(x509.VerifyOptions{
			Roots: roots, CurrentTime: now,
			KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		}); err != nil {
			return nil, time.Time{}, fmt.Errorf("%s: %w", p.ID, err)
		}
		if leaf.NotAfter.Before(expires) {
			expires = leaf.NotAfter
		}
	}
	if expires.Before(now.Add(labCertMinRemaining)) {
		return nil, time.Time{}, fmt.Errorf("certificates expire %s", expires.Local().Format("2006-01-02 15:04"))
	}
	return ca, expires, nil
}

// Keys of the lab Mirage config that the lab itself depends on; other keys are the user's to edit.
type labMirageFile struct {
	ID                         string `toml:"id"`
	Addr                       string `toml:"addr"`
	AdminListenAddr            string `toml:"admin_listen_addr"`
	APIv1Addr                  string `toml:"api_v1_addr"`
	AdminTLSCertFile           string `toml:"admin_tls_cert_file"`
	AdminTLSKeyFile            string `toml:"admin_tls_key_file"`
	AdminTLSCAFile             string `toml:"admin_tls_ca_file"`
	LocalGhostID               string `toml:"local_ghost_id"`
	LocalGhostAdminAddr        string `toml:"local_ghost_admin_addr"`
	LocalGhostAdminTLSCertFile string `toml:"local_ghost_admin_tls_cert_file"`
	LocalGhostAdminTLSKeyFile  string `toml:"local_ghost_admin_tls_key_file"`
}

// Reads a kept lab Mirage config. An error means it is missing or incomplete and is rewritten:
// it must name the lab identities, four loopback addresses, and this lab's certificate files.
func (l *labEnv) loadMirageConfig() (labMirageFile, error) {
	var cfg labMirageFile
	if _, err := toml.DecodeFile(l.mirageConfig(), &cfg); err != nil {
		return cfg, err
	}
	if cfg.ID != labMirageID || cfg.LocalGhostID != labGhostID {
		return cfg, errors.New("config names other identities")
	}
	addrs := map[string]string{
		"addr": cfg.Addr, "admin_listen_addr": cfg.AdminListenAddr,
		"api_v1_addr": cfg.APIv1Addr, "local_ghost_admin_addr": cfg.LocalGhostAdminAddr,
	}
	for key, addr := range addrs {
		if strings.TrimSpace(addr) == "" || !adminauth.IsLoopbackAddr(addr) {
			return cfg, fmt.Errorf("config %s is missing or not loopback", key)
		}
	}
	certs := map[string]string{
		cfg.AdminTLSCertFile:           labMirageID + ".crt",
		cfg.AdminTLSKeyFile:            labMirageID + ".key",
		cfg.AdminTLSCAFile:             "ca.crt",
		cfg.LocalGhostAdminTLSCertFile: labGhostID + ".crt",
		cfg.LocalGhostAdminTLSKeyFile:  labGhostID + ".key",
	}
	if len(certs) != 5 {
		return cfg, errors.New("config certificate paths are missing")
	}
	for got, name := range certs {
		if got != filepath.Join(l.certsDir(), name) {
			return cfg, fmt.Errorf("config does not use this lab's %s", name)
		}
	}
	return cfg, nil
}

// Replaces the lab PKI with a fresh root and the fixed lab identities, discarding any child Ghost
// certificates from the old root. The lab directory is the only place written; a deployment's
// certs/ is never read or touched.
func (l *labEnv) resetCerts() error {
	if err := os.RemoveAll(l.certsDir()); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(l.certsDir(), "root"), 0o700); err != nil {
		return err
	}
	ca, der, err := pki.NewRoot("edgectl-lab-root", labCertDays, time.Now())
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
	for _, p := range labLeaves() {
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
	leaf, err := l.ca.IssueLeaf(p, []string{"localhost"}, []net.IP{net.ParseIP("127.0.0.1")}, labCertDays, time.Now())
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
		"# Written by client-tm lab bootstrap and kept between starts. Delete this file to regenerate it.",
		"id = " + q(labMirageID),
		"addr = " + q(l.sessionAddr),
		"admin_listen_addr = " + q(l.mirageAdminAddr),
		"api_v1_addr = " + q(l.apiV1Addr),
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

// Starts Mirage for the lab menu: output goes to the lab log, uncolored for later display.
func (l *labEnv) start() error {
	logFile, err := os.Create(l.logPath())
	if err != nil {
		return err
	}
	if err := l.launch(logFile, logFile, logging.EnvLogNoColor+"=true", logging.EnvLogLevel+"=info"); err != nil {
		logFile.Close()
		return err
	}
	l.logFile = logFile
	return nil
}

// Starts Mirage for serve mode: output goes to this terminal as the runtime prints it, and to the
// lab log with color codes removed, so the log reads the same as one written by the lab menu.
func (l *labEnv) startTee() error {
	logFile, err := os.Create(l.logPath())
	if err != nil {
		return err
	}
	plain := &plainLogWriter{out: logFile}
	if err := l.launch(io.MultiWriter(os.Stdout, plain), io.MultiWriter(os.Stderr, plain)); err != nil {
		plain.Close()
		return err
	}
	l.logFile = plain
	return nil
}

// Copies colored runtime output to a file as plain text. Lines are completed before color codes
// are removed, so a code split across two writes is still removed. Safe for concurrent writers.
type plainLogWriter struct {
	mu      sync.Mutex
	out     *os.File
	pending []byte
}

func (w *plainLogWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pending = append(w.pending, p...)
	end := bytes.LastIndexByte(w.pending, '\n')
	if end < 0 {
		return len(p), nil
	}
	_, err := w.out.WriteString(logs.StripANSI(string(w.pending[:end+1])))
	w.pending = append(w.pending[:0], w.pending[end+1:]...)
	return len(p), err
}

// Writes any unfinished last line, then closes the file.
func (w *plainLogWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.pending) > 0 {
		_, _ = w.out.WriteString(logs.StripANSI(string(w.pending)))
		w.pending = nil
	}
	return w.out.Close()
}

// Starts Mirage (and its managed Ghost) writing to the given sinks, with the lab directory as
// working directory so seed.fs data stays inside it. env entries are added to the environment.
func (l *labEnv) launch(stdout, stderr io.Writer, env ...string) error {
	cmd := exec.Command(l.mirageBinary(), "-config", l.mirageConfig())
	cmd.Dir = l.dir
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Env = append(os.Environ(), env...)
	// Own process group: Ctrl-C in the terminal reaches the client, which then stops Mirage itself.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	l.proc, l.exited = cmd, make(chan struct{})
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
		l.logFile = nil
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
	return loopbackAddrFree(loopbackAddr(port))
}

func loopbackAddrFree(addr string) bool {
	ln, err := net.Listen("tcp", addr)
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
