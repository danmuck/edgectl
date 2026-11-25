package seeds

import (
	"errors"
	"strings"
	"testing"

	"github.com/danmuck/edgectl/internal/tools"
)

type installFakeRunner struct {
	commands [][]string
	results  []installRunResult
	err      error
}

type installRunResult struct {
	stdout   []byte
	stderr   []byte
	exitCode uint32
	err      error
}

func (r *installFakeRunner) Run(name string, args ...string) ([]byte, []byte, uint32, error) {
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
		Ref:     "v1.2.3",
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
		Ref:         "v1.2.3",
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

func TestNewInstallerDefaultRunnerUsesEnvRunner(t *testing.T) {
	workspace := t.TempDir()
	installer, err := NewInstaller(InstallerConfig{
		WorkspaceRoot: workspace,
		InstallRoot:   "local/seeds",
		BinRoot:       "local/bin",
		Whitelist:     []string{"seed.flow"},
	})
	if err != nil {
		t.Fatalf("new installer: %v", err)
	}
	if _, ok := installer.runner.(tools.EnvRunner); !ok {
		t.Fatalf("expected default runner type tools.EnvRunner, got %T", installer.runner)
	}
}
