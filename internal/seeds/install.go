package seeds

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/danmuck/edgectl/internal/tools"
	logs "github.com/danmuck/smplog"
)

var (
	ErrInstallInvalidSpec      = errors.New("seeds: invalid install spec")
	ErrInstallSandboxViolation = errors.New("seeds: sandbox violation")
	ErrInstallNotWhitelisted   = errors.New("seeds: seed not whitelisted for install")
	ErrInstallUnsupportedRepo  = errors.New("seeds: unsupported repository")
	ErrInstallInvalidRoot      = errors.New("seeds: invalid install root")
	ErrInstallBrewMissing      = errors.New("seeds: brew not installed")
	ErrInstallChecksumMismatch = errors.New("seeds: downloaded artifact checksum mismatch")
	ErrInstallDownloadFailed   = errors.New("seeds: artifact download failed")
)

// InstallMethod names one allowlisted dependency install channel.
type InstallMethod string

const (
	InstallMethodGitHub InstallMethod = "github"
	InstallMethodCurl   InstallMethod = "curl"
	InstallMethodBrew   InstallMethod = "brew"
	InstallMethodStream InstallMethod = "stream"
)

// Seeds package dependency install plan for one seed dependency.
type InstallSpec struct {
	SeedID      string        `json:"seed_id"`
	Method      InstallMethod `json:"method"`
	RepoURL     string        `json:"repo_url,omitempty"`
	Branch      string        `json:"branch,omitempty"`
	Ref         string        `json:"ref,omitempty"`
	URL         string        `json:"url,omitempty"`
	Version     string        `json:"version,omitempty"`
	SHA256      string        `json:"sha256,omitempty"`
	Destination string        `json:"destination,omitempty"`
	Package     string        `json:"package,omitempty"`
	Tap         string        `json:"tap,omitempty"`
	Binary      string        `json:"binary,omitempty"`
	Direction   string        `json:"direction,omitempty"`
	// Host-local bootstrap controls; never decoded from the wire so a plan cannot carry commands.
	BootstrapIfMissing bool     `json:"-"`
	BootstrapCommand   []string `json:"-"`
	InstallToBin       bool     `json:"install_to_bin,omitempty"`
}

var defaultBrewBootstrapCommand = []string{
	"/bin/bash",
	"-c",
	`NONINTERACTIVE=1 /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"`,
}

// Seeds package installer configuration for workspace-rooted install execution.
type InstallerConfig struct {
	WorkspaceRoot string
	InstallRoot   string
	BinRoot       string
	Whitelist     []string
	Runner        tools.CommandRunner
	// HTTP client for curl installs; defaults to a 5-minute-timeout client.
	HTTPClient *http.Client
}

// Seeds package installer enforcing install policy, whitelist, and filesystem sandbox boundaries.
type Installer struct {
	workspaceRoot string
	installRoot   string
	binRoot       string
	whitelist     map[string]struct{}
	runner        tools.CommandRunner
	httpClient    *http.Client
}

// Seeds package installer constructor; install and bin roots must sit under <workspace>/local.
func NewInstaller(cfg InstallerConfig) (*Installer, error) {
	workspaceRoot := strings.TrimSpace(cfg.WorkspaceRoot)
	if workspaceRoot == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		workspaceRoot = wd
	}
	workspaceAbs, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return nil, err
	}

	localRoot := filepath.Join(workspaceAbs, "local")
	installRoot := strings.TrimSpace(cfg.InstallRoot)
	if installRoot == "" {
		installRoot = filepath.Join("local", "seeds")
	}
	if !filepath.IsAbs(installRoot) {
		installRoot = filepath.Join(workspaceAbs, installRoot)
	}
	installRoot = filepath.Clean(installRoot)
	if !isWithin(installRoot, localRoot) {
		return nil, fmt.Errorf("%w: install_root=%q must be under %q", ErrInstallInvalidRoot, installRoot, localRoot)
	}
	if err := os.MkdirAll(installRoot, 0o755); err != nil {
		return nil, err
	}

	binRoot := strings.TrimSpace(cfg.BinRoot)
	if binRoot == "" {
		binRoot = filepath.Join("local", "bin")
	}
	if !filepath.IsAbs(binRoot) {
		binRoot = filepath.Join(workspaceAbs, binRoot)
	}
	binRoot = filepath.Clean(binRoot)
	if !isWithin(binRoot, localRoot) {
		return nil, fmt.Errorf("%w: bin_root=%q must be under %q", ErrInstallInvalidRoot, binRoot, localRoot)
	}
	if err := os.MkdirAll(binRoot, 0o755); err != nil {
		return nil, err
	}

	runner := cfg.Runner
	if runner == nil {
		runner = tools.EnvRunner{ExtraPaths: []string{binRoot}}
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 5 * time.Minute}
	}

	return &Installer{
		workspaceRoot: workspaceAbs,
		installRoot:   installRoot,
		binRoot:       binRoot,
		whitelist:     normalizeWhitelist(cfg.Whitelist),
		runner:        runner,
		httpClient:    httpClient,
	}, nil
}

// Seeds package installer execution for a batch of install specs.
func (i *Installer) InstallAll(specs []InstallSpec) error {
	for _, spec := range specs {
		if err := i.Install(spec); err != nil {
			return fmt.Errorf("seed_id=%q: %w", strings.TrimSpace(spec.SeedID), err)
		}
	}
	return nil
}

// Seeds package installer execution for one install spec: policy, whitelist, then method.
func (i *Installer) Install(spec InstallSpec) error {
	if err := ValidateSpec(spec); err != nil {
		return err
	}
	seedID := strings.TrimSpace(spec.SeedID)
	if _, ok := i.whitelist[seedID]; !ok {
		return fmt.Errorf("%w: %s", ErrInstallNotWhitelisted, seedID)
	}

	method := InstallMethod(strings.ToLower(strings.TrimSpace(string(spec.Method))))
	dest, err := i.resolveDestination(seedID, spec.Destination)
	if err != nil {
		return err
	}

	switch method {
	case InstallMethodGitHub:
		return i.installGitHub(spec, dest)
	case InstallMethodCurl:
		return i.installCurl(spec, dest)
	case InstallMethodBrew:
		if err := i.installBrew(spec); err != nil {
			return err
		}
		if spec.InstallToBin {
			return i.symlinkBrewToBin(spec)
		}
		return nil
	case InstallMethodStream:
		return fmt.Errorf("%w: stream transfer is deferred to Phase 9", ErrInstallMethodDeferred)
	}
	return fmt.Errorf("%w: %q", ErrInstallMethodNotAllowed, spec.Method)
}

// Uninstall removes what Install placed under the managed roots for github and curl specs
// (destination plus its bin symlink). brew and stream are not rolled back; returns false for them.
func (i *Installer) Uninstall(spec InstallSpec) (bool, error) {
	method := InstallMethod(strings.ToLower(strings.TrimSpace(string(spec.Method))))
	if method != InstallMethodGitHub && method != InstallMethodCurl {
		return false, nil
	}
	dest, err := i.resolveDestination(strings.TrimSpace(spec.SeedID), spec.Destination)
	if err != nil {
		return false, err
	}
	if spec.InstallToBin {
		link := filepath.Join(i.binRoot, binaryName(spec, dest))
		if target, err := os.Readlink(link); err == nil && filepath.Clean(target) == dest {
			_ = os.Remove(link)
		}
	}
	if err := os.RemoveAll(dest); err != nil {
		return false, err
	}
	return true, nil
}

func (i *Installer) resolveDestination(seedID string, destination string) (string, error) {
	rel := strings.TrimSpace(destination)
	if rel == "" {
		rel = strings.ReplaceAll(seedID, ".", string(os.PathSeparator))
	}
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("%w: destination must be relative: %q", ErrInstallInvalidSpec, destination)
	}
	dest := filepath.Clean(filepath.Join(i.installRoot, rel))
	if !isWithin(dest, i.installRoot) {
		return "", fmt.Errorf("%w: destination=%q outside install root", ErrInstallSandboxViolation, destination)
	}
	return dest, nil
}

func (i *Installer) installGitHub(spec InstallSpec, dest string) error {
	repo := strings.TrimSpace(spec.RepoURL)
	branch := strings.TrimSpace(spec.Branch)
	ref := strings.TrimSpace(spec.Ref)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}

	if _, err := os.Stat(dest); errors.Is(err, os.ErrNotExist) {
		args := []string{"clone"}
		if branch != "" {
			args = append(args, "--branch", branch, "--single-branch")
		}
		args = append(args, repo, dest)
		if err := i.runCommand("git", args...); err != nil {
			return err
		}
		if err := i.runCommand("git", "-C", dest, "fetch", "origin", ref); err != nil {
			return err
		}
		return i.runCommand("git", "-C", dest, "checkout", "FETCH_HEAD")
	} else if err != nil {
		return err
	}

	if _, err := os.Stat(filepath.Join(dest, ".git")); err != nil {
		return fmt.Errorf("%w: destination exists but is not a git repository: %s", ErrInstallInvalidSpec, dest)
	}
	if err := i.runCommand("git", "-C", dest, "fetch", "origin", ref); err != nil {
		return err
	}
	return i.runCommand("git", "-C", dest, "checkout", "FETCH_HEAD")
}

// Downloads spec.URL to dest, verifying sha256 before the file becomes visible at dest.
// With InstallToBin the file is made executable and symlinked into the managed bin root.
func (i *Installer) installCurl(spec InstallSpec, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	resp, err := i.httpClient.Get(strings.TrimSpace(spec.URL))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInstallDownloadFailed, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: status %d", ErrInstallDownloadFailed, resp.StatusCode)
	}

	tmp, err := os.CreateTemp(filepath.Dir(dest), ".download-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	hasher := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmp, hasher), resp.Body); err != nil {
		tmp.Close()
		return fmt.Errorf("%w: %v", ErrInstallDownloadFailed, err)
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	got := hex.EncodeToString(hasher.Sum(nil))
	if want := strings.ToLower(strings.TrimSpace(spec.SHA256)); got != want {
		return fmt.Errorf("%w: got %s want %s", ErrInstallChecksumMismatch, got, want)
	}

	perm := os.FileMode(0o644)
	if spec.InstallToBin {
		perm = 0o755
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		return err
	}
	if err := os.Rename(tmpName, dest); err != nil {
		return err
	}
	if spec.InstallToBin {
		link := filepath.Join(i.binRoot, binaryName(spec, dest))
		_ = os.Remove(link)
		return os.Symlink(dest, link)
	}
	return nil
}

// Seeds package brew installer; brew is the single allowed host-global exception.
func (i *Installer) installBrew(spec InstallSpec) error {
	pkg := strings.TrimSpace(spec.Package)
	if err := i.ensureBrewAvailable(spec); err != nil {
		return err
	}

	tap := strings.TrimSpace(spec.Tap)
	if tap != "" {
		if err := i.runCommand("brew", "tap", tap); err != nil {
			return err
		}
	}
	return i.runCommand("brew", "install", pkg)
}

// Seeds package brew availability guard with optional bootstrap execution.
func (i *Installer) ensureBrewAvailable(spec InstallSpec) error {
	checkErr := i.checkBrewVersion()
	if checkErr == nil {
		return nil
	}
	if !errors.Is(checkErr, ErrInstallBrewMissing) {
		return checkErr
	}

	if !spec.BootstrapIfMissing {
		return fmt.Errorf("%w: set bootstrap_if_missing=true or install brew before startup", ErrInstallBrewMissing)
	}

	bootstrapCmd := spec.BootstrapCommand
	if len(bootstrapCmd) == 0 {
		bootstrapCmd = defaultBrewBootstrapCommand
	}
	name := strings.TrimSpace(bootstrapCmd[0])
	if name == "" {
		return fmt.Errorf("%w: invalid bootstrap command", ErrInstallInvalidSpec)
	}
	if err := i.runCommand(name, bootstrapCmd[1:]...); err != nil {
		return err
	}
	if err := i.checkBrewVersion(); err != nil {
		return fmt.Errorf("%w: bootstrap completed but brew is still unavailable", ErrInstallBrewMissing)
	}
	return nil
}

// Seeds package brew version probe used for host dependency checks.
func (i *Installer) checkBrewVersion() error {
	stdout, stderr, exitCode, err := i.runner.Run("brew", "--version")
	if err == nil {
		return nil
	}
	if exitCode == 127 {
		return ErrInstallBrewMissing
	}
	return fmt.Errorf(
		"seeds install command failed cmd=brew args=%q exit=%d stdout=%q stderr=%q: %w",
		"--version",
		exitCode,
		strings.TrimSpace(string(stdout)),
		strings.TrimSpace(string(stderr)),
		err,
	)
}

// BinRoot returns the resolved binary install directory path.
func (i *Installer) BinRoot() string {
	return i.binRoot
}

// symlinkBrewToBin exposes the brew-installed binary to Ghost through the managed bin root.
func (i *Installer) symlinkBrewToBin(spec InstallSpec) error {
	pkg := strings.TrimSpace(spec.Package)
	stdout, _, _, err := i.runner.Run("brew", "--prefix", pkg)
	if err != nil {
		logs.Warnf("seeds.install symlink_brew_to_bin: could not resolve prefix for %q", pkg)
		return nil
	}
	prefix := strings.TrimSpace(string(stdout))
	name := strings.TrimSpace(spec.Binary)
	if name == "" {
		name = pkg
	}
	binPath := filepath.Join(prefix, "bin", name)
	if _, err := os.Stat(binPath); err != nil {
		binDir := filepath.Join(prefix, "bin")
		entries, dirErr := os.ReadDir(binDir)
		if dirErr != nil || len(entries) == 0 {
			logs.Warnf("seeds.install symlink_brew_to_bin: no binaries found under %q", prefix)
			return nil
		}
		binPath = filepath.Join(binDir, entries[0].Name())
	}
	link := filepath.Join(i.binRoot, filepath.Base(binPath))
	_ = os.Remove(link)
	logs.Infof("seeds.install symlink %s -> %s", link, binPath)
	return os.Symlink(binPath, link)
}

func (i *Installer) runCommand(name string, args ...string) error {
	logs.Infof("seeds.install exec cmd=%s args=%q", name, strings.Join(args, " "))
	stdout, stderr, exitCode, err := i.runner.Run(name, args...)
	if err == nil {
		return nil
	}
	return fmt.Errorf(
		"seeds install command failed cmd=%s args=%q exit=%d stdout=%q stderr=%q: %w",
		name,
		strings.Join(args, " "),
		exitCode,
		strings.TrimSpace(string(stdout)),
		strings.TrimSpace(string(stderr)),
		err,
	)
}

// Bin symlink name for a managed-root install: explicit Binary, else the destination base name.
func binaryName(spec InstallSpec, dest string) string {
	if name := strings.TrimSpace(spec.Binary); name != "" {
		return filepath.Base(name)
	}
	return filepath.Base(dest)
}

func validateGitHubRepo(repo string) error {
	u, err := url.Parse(repo)
	if err != nil {
		return fmt.Errorf("%w: repo=%q parse error: %v", ErrInstallUnsupportedRepo, repo, err)
	}
	if u.Scheme != "https" || !strings.EqualFold(u.Host, "github.com") {
		return fmt.Errorf("%w: repo=%q must be https://github.com/*", ErrInstallUnsupportedRepo, repo)
	}
	if strings.TrimSpace(u.Path) == "" || u.Path == "/" {
		return fmt.Errorf("%w: repo=%q missing repository path", ErrInstallUnsupportedRepo, repo)
	}
	return nil
}

func normalizeWhitelist(in []string) map[string]struct{} {
	out := make(map[string]struct{})
	for _, raw := range in {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		out[id] = struct{}{}
	}
	return out
}

func isWithin(path string, root string) bool {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && rel != "..")
}
