package seeds

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validSHA = "0000000000000000000000000000000000000000000000000000000000000000"

func TestValidateSpecMethodAllowlist(t *testing.T) {
	for _, m := range []InstallMethod{"apt", "workspace_copy", "pip", ""} {
		err := ValidateSpec(InstallSpec{SeedID: "seed.x", Method: m})
		if !errors.Is(err, ErrInstallMethodNotAllowed) {
			t.Errorf("method %q: expected ErrInstallMethodNotAllowed, got %v", m, err)
		}
	}
	allowed := []InstallSpec{
		{SeedID: "seed.x", Method: InstallMethodGitHub, RepoURL: "https://github.com/o/r", Ref: "v1"},
		{SeedID: "seed.x", Method: InstallMethodCurl, URL: "https://example.com/x", Version: "1.0", SHA256: validSHA, Destination: "x/bin"},
		{SeedID: "seed.x", Method: InstallMethodBrew, Package: "docker"},
		{SeedID: "seed.x", Method: InstallMethodStream, Direction: StreamDirectionMirageToGhost, SHA256: validSHA},
	}
	for _, spec := range allowed {
		if err := ValidateSpec(spec); err != nil {
			t.Errorf("method %q: expected valid, got %v", spec.Method, err)
		}
	}
}

func TestValidateSpecRequiresPinnedVerifiedSources(t *testing.T) {
	cases := []struct {
		name string
		spec InstallSpec
	}{
		{"github without ref", InstallSpec{SeedID: "s", Method: InstallMethodGitHub, RepoURL: "https://github.com/o/r"}},
		{"github non-github host", InstallSpec{SeedID: "s", Method: InstallMethodGitHub, RepoURL: "https://gitlab.com/o/r", Ref: "v1"}},
		{"curl http", InstallSpec{SeedID: "s", Method: InstallMethodCurl, URL: "http://example.com/x", Version: "1", SHA256: validSHA, Destination: "x"}},
		{"curl no sha", InstallSpec{SeedID: "s", Method: InstallMethodCurl, URL: "https://example.com/x", Version: "1", Destination: "x"}},
		{"curl bad sha", InstallSpec{SeedID: "s", Method: InstallMethodCurl, URL: "https://example.com/x", Version: "1", SHA256: "zz", Destination: "x"}},
		{"curl no version", InstallSpec{SeedID: "s", Method: InstallMethodCurl, URL: "https://example.com/x", SHA256: validSHA, Destination: "x"}},
		{"curl no destination", InstallSpec{SeedID: "s", Method: InstallMethodCurl, URL: "https://example.com/x", Version: "1", SHA256: validSHA}},
		{"brew no package", InstallSpec{SeedID: "s", Method: InstallMethodBrew}},
	}
	for _, tc := range cases {
		if err := ValidateSpec(tc.spec); err == nil {
			t.Errorf("%s: expected rejection", tc.name)
		}
	}
}

// stream payloads may only flow mirage -> ghost.
func TestValidateSpecStreamDirection(t *testing.T) {
	for _, dir := range []string{"", "ghost->mirage", "ghost->ghost"} {
		err := ValidateSpec(InstallSpec{SeedID: "s", Method: InstallMethodStream, Direction: dir, SHA256: validSHA})
		if !errors.Is(err, ErrInstallTransferDirection) {
			t.Errorf("direction %q: expected ErrInstallTransferDirection, got %v", dir, err)
		}
	}
}

// Destinations cannot escape the project-managed install root.
func TestValidateSpecRejectsUnmanagedDestinations(t *testing.T) {
	for _, dest := range []string{"/usr/local/bin/docker", "../outside", "a/../../outside"} {
		err := ValidateSpec(InstallSpec{SeedID: "s", Method: InstallMethodBrew, Package: "p", Destination: dest})
		if !errors.Is(err, ErrInstallSandboxViolation) {
			t.Errorf("destination %q: expected ErrInstallSandboxViolation, got %v", dest, err)
		}
	}
}

func newCurlInstaller(t *testing.T, server *httptest.Server) (*Installer, string) {
	t.Helper()
	workspace := t.TempDir()
	installer, err := NewInstaller(InstallerConfig{
		WorkspaceRoot: workspace,
		Whitelist:     []string{"seed.tool"},
		Runner:        &installFakeRunner{},
		HTTPClient:    server.Client(),
	})
	if err != nil {
		t.Fatalf("new installer: %v", err)
	}
	return installer, workspace
}

func curlSpec(url string, sum string) InstallSpec {
	return InstallSpec{
		SeedID: "seed.tool", Method: InstallMethodCurl, URL: url, Version: "1.0.0", SHA256: sum,
		Destination: "seed.tool/tool", Binary: "tool", InstallToBin: true,
	}
}

func TestInstallCurlVerifiesChecksumAndLinksBinary(t *testing.T) {
	payload := []byte("#!/bin/sh\necho tool\n")
	sum := sha256.Sum256(payload)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(payload) }))
	defer server.Close()
	installer, workspace := newCurlInstaller(t, server)

	if err := installer.Install(curlSpec(server.URL+"/tool", hex.EncodeToString(sum[:]))); err != nil {
		t.Fatalf("curl install: %v", err)
	}
	dest := filepath.Join(workspace, "local", "seeds", "seed.tool", "tool")
	got, err := os.ReadFile(dest)
	if err != nil || string(got) != string(payload) {
		t.Fatalf("installed file: %q err=%v", got, err)
	}
	link := filepath.Join(workspace, "local", "bin", "tool")
	if target, err := os.Readlink(link); err != nil || target != dest {
		t.Fatalf("bin symlink: target=%q err=%v", target, err)
	}
}

func TestInstallCurlRejectsChecksumMismatchWithoutWriting(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("tampered")) }))
	defer server.Close()
	installer, workspace := newCurlInstaller(t, server)

	err := installer.Install(curlSpec(server.URL+"/tool", validSHA))
	if !errors.Is(err, ErrInstallChecksumMismatch) {
		t.Fatalf("expected ErrInstallChecksumMismatch, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspace, "local", "seeds", "seed.tool", "tool")); !os.IsNotExist(err) {
		t.Fatalf("mismatched artifact must not be installed, stat err=%v", err)
	}
	entries, _ := os.ReadDir(filepath.Join(workspace, "local", "seeds", "seed.tool"))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".download-") {
			t.Fatalf("temporary download left behind: %s", e.Name())
		}
	}
}

func TestUninstallRemovesManagedArtifactsOnly(t *testing.T) {
	payload := []byte("bin")
	sum := sha256.Sum256(payload)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(payload) }))
	defer server.Close()
	installer, workspace := newCurlInstaller(t, server)
	spec := curlSpec(server.URL+"/tool", hex.EncodeToString(sum[:]))
	if err := installer.Install(spec); err != nil {
		t.Fatalf("install: %v", err)
	}

	ok, err := installer.Uninstall(spec)
	if err != nil || !ok {
		t.Fatalf("uninstall: ok=%v err=%v", ok, err)
	}
	if _, err := os.Lstat(filepath.Join(workspace, "local", "bin", "tool")); !os.IsNotExist(err) {
		t.Fatalf("bin symlink should be removed, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(workspace, "local", "seeds", "seed.tool", "tool")); !os.IsNotExist(err) {
		t.Fatalf("artifact should be removed, err=%v", err)
	}

	// brew lives outside the managed roots and is never rolled back.
	ok, err = installer.Uninstall(InstallSpec{SeedID: "seed.tool", Method: InstallMethodBrew, Package: "p"})
	if err != nil || ok {
		t.Fatalf("brew uninstall: ok=%v err=%v, want false/nil", ok, err)
	}
}

func TestInstallStreamIsDeferred(t *testing.T) {
	installer, err := NewInstaller(InstallerConfig{WorkspaceRoot: t.TempDir(), Whitelist: []string{"seed.tool"}, Runner: &installFakeRunner{}})
	if err != nil {
		t.Fatalf("new installer: %v", err)
	}
	err = installer.Install(InstallSpec{SeedID: "seed.tool", Method: InstallMethodStream, Direction: StreamDirectionMirageToGhost, SHA256: validSHA})
	if !errors.Is(err, ErrInstallMethodDeferred) {
		t.Fatalf("expected ErrInstallMethodDeferred, got %v", err)
	}
}

// Values passed to git/brew as arguments cannot start with '-' or carry shell characters.
func TestValidateSpecRejectsOptionInjection(t *testing.T) {
	cases := []InstallSpec{
		{SeedID: "s", Method: InstallMethodGitHub, RepoURL: "https://github.com/o/r", Ref: "--upload-pack=touch /tmp/x"},
		{SeedID: "s", Method: InstallMethodGitHub, RepoURL: "https://github.com/o/r", Ref: "v1", Branch: "-b"},
		{SeedID: "s", Method: InstallMethodBrew, Package: "--HEAD"},
		{SeedID: "s", Method: InstallMethodBrew, Package: "docker; rm -rf /"},
		{SeedID: "s", Method: InstallMethodBrew, Package: "docker", Tap: "-x"},
	}
	for _, spec := range cases {
		if err := ValidateSpec(spec); !errors.Is(err, ErrInstallInvalidSpec) {
			t.Errorf("%+v: expected ErrInstallInvalidSpec, got %v", spec, err)
		}
	}
	ok := InstallSpec{SeedID: "s", Method: InstallMethodBrew, Package: "mongodb-community@7.0", Tap: "mongodb/brew"}
	if err := ValidateSpec(ok); err != nil {
		t.Fatalf("legitimate brew spec rejected: %v", err)
	}
}
