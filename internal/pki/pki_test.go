package pki

import (
	"crypto/x509"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/danmuck/edgectl/internal/identity"
)

func TestIssueLeafMatchesContract(t *testing.T) {
	now := time.Now()
	ca, _, err := NewRoot("edgectl-root", 3650, now)
	if err != nil {
		t.Fatalf("root: %v", err)
	}
	p := identity.Principal{Role: identity.RoleGhost, ID: "ghost.pi"}
	leaf, err := ca.IssueLeaf(p, []string{"ghost.pi"}, nil, 90, now)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	got, err := identity.PrincipalFromCert(leaf.Cert)
	if err != nil || got != p {
		t.Fatalf("principal = %+v err=%v", got, err)
	}
	if len(leaf.Cert.ExtKeyUsage) != 2 {
		t.Fatalf("want serverAuth+clientAuth, got %v", leaf.Cert.ExtKeyUsage)
	}
	pool := x509.NewCertPool()
	pool.AddCert(ca.Cert)
	for _, usage := range []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth} {
		if _, err := leaf.Cert.Verify(x509.VerifyOptions{Roots: pool, KeyUsages: []x509.ExtKeyUsage{usage}}); err != nil {
			t.Fatalf("leaf does not chain for usage %v: %v", usage, err)
		}
	}
	if leaf.Cert.NotAfter.Sub(now) > 90*24*time.Hour+time.Minute {
		t.Fatalf("leaf lifetime exceeds 90 days: %v", leaf.Cert.NotAfter)
	}
}

func TestIssueLeafRejectsLongLifetimeAndBadPrincipal(t *testing.T) {
	ca, _, err := NewRoot("edgectl-root", 3650, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ca.IssueLeaf(identity.Principal{Role: identity.RoleGhost, ID: "g"}, nil, nil, 91, time.Now()); !errors.Is(err, ErrLeafLifetime) {
		t.Fatalf("91 days: expected ErrLeafLifetime, got %v", err)
	}
	if _, err := ca.IssueLeaf(identity.Principal{Role: "admin", ID: "g"}, nil, nil, 30, time.Now()); !errors.Is(err, identity.ErrInvalidPrincipal) {
		t.Fatalf("bad role: expected ErrInvalidPrincipal, got %v", err)
	}
}

func TestWriteAndLoadAuthority(t *testing.T) {
	dir := t.TempDir()
	ca, der, err := NewRoot("edgectl-root", 3650, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	certPath, keyPath := filepath.Join(dir, "ca.crt"), filepath.Join(dir, "ca.key")
	if err := WriteCertPEM(certPath, der); err != nil {
		t.Fatal(err)
	}
	if err := WriteKeyPEM(keyPath, ca.Key); err != nil {
		t.Fatal(err)
	}
	if info, _ := os.Stat(keyPath); info.Mode().Perm() != 0o600 {
		t.Fatalf("key mode = %o, want 600", info.Mode().Perm())
	}
	if err := WriteKeyPEM(keyPath, ca.Key); err == nil {
		t.Fatalf("existing key must not be overwritten")
	}
	if _, err := LoadAuthority(certPath, keyPath); err != nil {
		t.Fatalf("load: %v", err)
	}
	if err := os.Chmod(keyPath, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadAuthority(certPath, keyPath); !errors.Is(err, identity.ErrKeyFilePermissions) {
		t.Fatalf("world-readable CA key: expected ErrKeyFilePermissions, got %v", err)
	}
}
