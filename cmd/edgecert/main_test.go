package main

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"github.com/danmuck/edgectl/internal/identity"
)

func TestRootThenIssue(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "root")
	if err := run([]string{"root", "-out", root}); err != nil {
		t.Fatalf("root: %v", err)
	}
	out := filepath.Join(dir, "certs")
	if err := run([]string{"issue", "-ca-cert", filepath.Join(root, "ca.crt"), "-ca-key", filepath.Join(root, "ca.key"),
		"-role", "ghost", "-id", "ghost.pi", "-dns", "ghost.pi", "-ip", "10.0.0.5", "-out", out}); err != nil {
		t.Fatalf("issue: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(out, "ghost.pi.crt"))
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(raw)
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	p, err := identity.RequireRole(cert, identity.RoleGhost)
	if err != nil || p.ID != "ghost.pi" {
		t.Fatalf("principal: %+v %v", p, err)
	}
	if len(cert.IPAddresses) != 1 || cert.IPAddresses[0].String() != "10.0.0.5" {
		t.Fatalf("ip SANs: %v", cert.IPAddresses)
	}
	if err := identity.CheckPrivateKeyFile(filepath.Join(out, "ghost.pi.key")); err != nil {
		t.Fatalf("issued key mode: %v", err)
	}

	// Re-issuing the same id refuses to overwrite; an invalid id is rejected.
	if err := run([]string{"issue", "-ca-cert", filepath.Join(root, "ca.crt"), "-ca-key", filepath.Join(root, "ca.key"),
		"-role", "ghost", "-id", "ghost.pi", "-out", out}); err == nil {
		t.Fatalf("expected refusal to overwrite existing cert")
	}
	if err := run([]string{"issue", "-role", "ghost", "-id", "Bad ID", "-out", out}); err == nil {
		t.Fatalf("expected invalid id rejection")
	}
}
