package seeds

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/danmuck/edgectl/internal/tools"
	logs "github.com/danmuck/smplog"
)

var (
	ErrInstallInvalidSpec       = errors.New("seeds: invalid install spec")
	ErrInstallSandboxViolation  = errors.New("seeds: sandbox violation")
	ErrInstallNotWhitelisted    = errors.New("seeds: seed not whitelisted for install")
	ErrInstallUnsupportedMethod = errors.New("seeds: unsupported install method")
	ErrInstallUnsupportedRepo   = errors.New("seeds: unsupported repository")
	ErrInstallInvalidRoot       = errors.New("seeds: invalid install root")
	ErrInstallBrewMissing       = errors.New("seeds: brew not installed")
)

type InstallMethod string

const (
	InstallMethodGitHub        InstallMethod = "github"
	InstallMethodWorkspaceCopy InstallMethod = "workspace_copy"
	InstallMethodBrew          InstallMethod = "brew"
)

// Seeds package dependency install plan for one seed id.
type InstallSpec struct {
	SeedID             string
	Method             InstallMethod
	RepoURL            string
	Branch             string
	Ref                string
	SourcePath         string
	Destination        string
	Package            string
	Tap                string
	BootstrapIfMissing bool
	BootstrapCommand   []string
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
	Whitelist     []string
	Runner        tools.CommandRunner
}

// Seeds package installer enforcing whitelist and filesystem sandbox boundaries.
type Installer struct {
	workspaceRoot string
	installRoot   string
	whitelist     map[string]struct{}
	runner        tools.CommandRunner
}

// Seeds package installer constructor with whitelist and sandbox validation.
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

	runner := cfg.Runner
	if runner == nil {
		runner = tools.ExecRunner{}
	}

	return &Installer{
		workspaceRoot: workspaceAbs,
		installRoot:   installRoot,
		whitelist:     normalizeWhitelist(cfg.Whitelist),
		runner:        runner,
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

// Seeds package installer execution for one install spec.
func (i *Installer) Install(spec InstallSpec) error {
	seedID := strings.TrimSpace(spec.SeedID)
	if seedID == "" {
		return fmt.Errorf("%w: missing seed_id", ErrInstallInvalidSpec)
	}
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
	case InstallMethodWorkspaceCopy:
		return i.installWorkspaceCopy(spec, dest)
	case InstallMethodBrew:
		return i.installBrew(spec)
	default:
		return fmt.Errorf("%w: %q", ErrInstallUnsupportedMethod, spec.Method)
	}
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
	if repo == "" {
		return fmt.Errorf("%w: missing repo for github install", ErrInstallInvalidSpec)
	}
	if err := validateGitHubRepo(repo); err != nil {
		return err
	}

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
		if ref != "" {
			if err := i.runCommand("git", "-C", dest, "fetch", "origin", ref); err != nil {
				return err
			}
			if err := i.runCommand("git", "-C", dest, "checkout", "FETCH_HEAD"); err != nil {
				return err
			}
		}
		return nil
	} else if err != nil {
		return err
	}

	if _, err := os.Stat(filepath.Join(dest, ".git")); err != nil {
		return fmt.Errorf("%w: destination exists but is not a git repository: %s", ErrInstallInvalidSpec, dest)
	}

	if err := i.runCommand("git", "-C", dest, "fetch", "--all", "--prune"); err != nil {
		return err
	}
	if branch != "" {
		if err := i.runCommand("git", "-C", dest, "checkout", branch); err != nil {
			return err
		}
		if err := i.runCommand("git", "-C", dest, "pull", "--ff-only", "origin", branch); err != nil {
			return err
		}
	}
	if ref != "" {
		if err := i.runCommand("git", "-C", dest, "fetch", "origin", ref); err != nil {
			return err
		}
		if err := i.runCommand("git", "-C", dest, "checkout", "FETCH_HEAD"); err != nil {
			return err
		}
	}
	if branch == "" && ref == "" {
		if err := i.runCommand("git", "-C", dest, "pull", "--ff-only"); err != nil {
			return err
		}
	}
	return nil
}

func (i *Installer) installWorkspaceCopy(spec InstallSpec, dest string) error {
	source := strings.TrimSpace(spec.SourcePath)
	if source == "" {
		return fmt.Errorf("%w: missing source_path for workspace_copy", ErrInstallInvalidSpec)
	}

	src := source
	if !filepath.IsAbs(src) {
		src = filepath.Join(i.workspaceRoot, source)
	}
	srcAbs, err := filepath.Abs(src)
	if err != nil {
		return err
	}
	if !i.allowedWorkspaceSource(srcAbs) {
		return fmt.Errorf("%w: source=%q", ErrInstallSandboxViolation, source)
	}

	info, err := os.Lstat(srcAbs)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: symlinks are not allowed for source=%q", ErrInstallSandboxViolation, source)
	}

	if info.IsDir() {
		return copyDir(srcAbs, dest)
	}
	return copyFile(srcAbs, dest, info.Mode().Perm())
}

// Seeds package brew installer for seed dependency packages on host.
func (i *Installer) installBrew(spec InstallSpec) error {
	pkg := strings.TrimSpace(spec.Package)
	if pkg == "" {
		return fmt.Errorf("%w: missing package for brew install", ErrInstallInvalidSpec)
	}
	if err := i.ensureBrewAvailable(spec); err != nil {
		return err
	}

	tap := strings.TrimSpace(spec.Tap)
	if tap != "" {
		if err := i.runCommand("brew", "tap", tap); err != nil {
			return err
		}
	}
	if err := i.runCommand("brew", "install", pkg); err != nil {
		return err
	}
	return nil
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

func (i *Installer) allowedWorkspaceSource(srcAbs string) bool {
	roots := []string{
		filepath.Join(i.workspaceRoot, "local"),
		filepath.Join(i.workspaceRoot, "docs", "progress", "buildlog"),
		filepath.Join(i.workspaceRoot, "docs", "buildlog"),
	}
	for _, root := range roots {
		if isWithin(srcAbs, root) {
			return true
		}
	}
	return false
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

func copyDir(src string, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: symlinks are not allowed in source tree: %s", ErrInstallSandboxViolation, path)
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		return copyFile(path, target, info.Mode().Perm())
	})
}

func copyFile(src string, dst string, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return nil
}
