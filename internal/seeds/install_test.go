package seeds

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type installFakeRunner struct {
	commands [][]string
	results  []installRunResult
	err      error
}

type installRunResult struct {
	stdout   []byte
	stderr   []byte
	exitCode int32
	err      error
}

func (r *installFakeRunner) Run(name string, args ...string) ([]byte, []byte, int32, error) {
	cmd := []string{name}
	cmd = append(cmd, args...)
	r.commands = append(r.commands, cmd)
	if len(r.results) > 0 {
		next := r.results[0]
		r.results = r.results[1:]
		return next.stdout, next.stderr, next.exitCode, next.err
	}
	if r.err != nil {
		return nil, []byte("runner error"), 1, r.err
	}
	return nil, nil, 0, nil
}

func TestNewInstallerRejectsInstallRootOutsideLocal(t *testing.T) {
	workspace := t.TempDir()
	if _, err := NewInstaller(InstallerConfig{
		WorkspaceRoot: workspace,
		InstallRoot:   "tmp/seeds",
		Whitelist:     []string{"seed.flow"},
	}); !errors.Is(err, ErrInstallInvalidRoot) {
		t.Fatalf("expected ErrInstallInvalidRoot, got %v", err)
	}
}

func TestInstallWorkspaceCopyFromBuildlog(t *testing.T) {
	workspace := t.TempDir()
	srcPath := filepath.Join(workspace, "local", "buildlogs", "2026-02-07_13:00.toml")
	if err := os.MkdirAll(filepath.Dir(srcPath), 0o755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	if err := os.WriteFile(srcPath, []byte("buildlog data"), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}

	installer, err := NewInstaller(InstallerConfig{
		WorkspaceRoot: workspace,
		InstallRoot:   "local/seeds",
		Whitelist:     []string{"seed.archive"},
		Runner:        &installFakeRunner{},
	})
	if err != nil {
		t.Fatalf("new installer: %v", err)
	}

	err = installer.Install(InstallSpec{
		SeedID:      "seed.archive",
		Method:      InstallMethodWorkspaceCopy,
		SourcePath:  "local/buildlogs/2026-02-07_13:00.toml",
		Destination: "seed.archive/data/buildlog.toml",
	})
	if err != nil {
		t.Fatalf("install workspace copy: %v", err)
	}

	destPath := filepath.Join(workspace, "local", "seeds", "seed.archive", "data", "buildlog.toml")
	out, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if string(out) != "buildlog data" {
		t.Fatalf("unexpected dest content: %q", string(out))
	}
}

func TestInstallRejectsSourceOutsideSandbox(t *testing.T) {
	workspace := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("x"), 0o644); err != nil {
		t.Fatalf("write outside: %v", err)
	}

	installer, err := NewInstaller(InstallerConfig{
		WorkspaceRoot: workspace,
		InstallRoot:   "local/seeds",
		Whitelist:     []string{"seed.archive"},
		Runner:        &installFakeRunner{},
	})
	if err != nil {
		t.Fatalf("new installer: %v", err)
	}

	err = installer.Install(InstallSpec{
		SeedID:      "seed.archive",
		Method:      InstallMethodWorkspaceCopy,
		SourcePath:  outside,
		Destination: "seed.archive/data/outside.txt",
	})
	if !errors.Is(err, ErrInstallSandboxViolation) {
		t.Fatalf("expected ErrInstallSandboxViolation, got %v", err)
	}
}

func TestInstallRejectsUnwhitelistedSeed(t *testing.T) {
	workspace := t.TempDir()
	installer, err := NewInstaller(InstallerConfig{
		WorkspaceRoot: workspace,
		InstallRoot:   "local/seeds",
		Whitelist:     []string{"seed.flow"},
		Runner:        &installFakeRunner{},
	})
	if err != nil {
		t.Fatalf("new installer: %v", err)
	}

	err = installer.Install(InstallSpec{
		SeedID:  "seed.mongod",
		Method:  InstallMethodGitHub,
		RepoURL: "https://github.com/example/mongod-seed.git",
	})
	if !errors.Is(err, ErrInstallNotWhitelisted) {
		t.Fatalf("expected ErrInstallNotWhitelisted, got %v", err)
	}
}

func TestInstallGitHubUsesCloneAndBranch(t *testing.T) {
	workspace := t.TempDir()
	runner := &installFakeRunner{}
	installer, err := NewInstaller(InstallerConfig{
		WorkspaceRoot: workspace,
		InstallRoot:   "local/seeds",
		Whitelist:     []string{"seed.mongod"},
		Runner:        runner,
	})
	if err != nil {
		t.Fatalf("new installer: %v", err)
	}

	if err := installer.Install(InstallSpec{
		SeedID:      "seed.mongod",
		Method:      InstallMethodGitHub,
		RepoURL:     "https://github.com/example/mongod-seed.git",
		Branch:      "main",
		Destination: "seed.mongod",
	}); err != nil {
		t.Fatalf("install github: %v", err)
	}

	if len(runner.commands) == 0 {
		t.Fatalf("expected git command execution")
	}
	first := strings.Join(runner.commands[0], " ")
	if !strings.Contains(first, "git clone") || !strings.Contains(first, "--branch main") {
		t.Fatalf("unexpected clone command: %q", first)
	}
}

func TestInstallBrewRequiresBootstrapWhenMissing(t *testing.T) {
	workspace := t.TempDir()
	runner := &installFakeRunner{
		results: []installRunResult{
			{exitCode: 127, err: errors.New("brew missing")},
		},
	}
	installer, err := NewInstaller(InstallerConfig{
		WorkspaceRoot: workspace,
		InstallRoot:   "local/seeds",
		Whitelist:     []string{"seed.mongod.pkg"},
		Runner:        runner,
	})
	if err != nil {
		t.Fatalf("new installer: %v", err)
	}

	err = installer.Install(InstallSpec{
		SeedID:  "seed.mongod.pkg",
		Method:  InstallMethodBrew,
		Package: "mongodb-community@7.0",
	})
	if !errors.Is(err, ErrInstallBrewMissing) {
		t.Fatalf("expected ErrInstallBrewMissing, got %v", err)
	}
}

func TestInstallBrewBootstrapsAndInstalls(t *testing.T) {
	workspace := t.TempDir()
	runner := &installFakeRunner{
		results: []installRunResult{
			{exitCode: 127, err: errors.New("brew missing")},
			{exitCode: 0},
			{exitCode: 0},
			{exitCode: 0},
			{exitCode: 0},
		},
	}
	installer, err := NewInstaller(InstallerConfig{
		WorkspaceRoot: workspace,
		InstallRoot:   "local/seeds",
		Whitelist:     []string{"seed.mongod.pkg"},
		Runner:        runner,
	})
	if err != nil {
		t.Fatalf("new installer: %v", err)
	}

	if err := installer.Install(InstallSpec{
		SeedID:             "seed.mongod.pkg",
		Method:             InstallMethodBrew,
		Package:            "mongodb-community@7.0",
		Tap:                "mongodb/brew",
		BootstrapIfMissing: true,
		BootstrapCommand:   []string{"echo", "bootstrap"},
	}); err != nil {
		t.Fatalf("install brew: %v", err)
	}

	if len(runner.commands) != 5 {
		t.Fatalf("unexpected command count: %d", len(runner.commands))
	}
	if got := strings.Join(runner.commands[0], " "); got != "brew --version" {
		t.Fatalf("unexpected brew check command: %q", got)
	}
	if got := strings.Join(runner.commands[1], " "); got != "echo bootstrap" {
		t.Fatalf("unexpected bootstrap command: %q", got)
	}
	if got := strings.Join(runner.commands[2], " "); got != "brew --version" {
		t.Fatalf("unexpected second brew check command: %q", got)
	}
	if got := strings.Join(runner.commands[3], " "); got != "brew tap mongodb/brew" {
		t.Fatalf("unexpected tap command: %q", got)
	}
	if got := strings.Join(runner.commands[4], " "); got != "brew install mongodb-community@7.0" {
		t.Fatalf("unexpected install command: %q", got)
	}
}
