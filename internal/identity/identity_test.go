package identity

import (
	"crypto/x509"
	"errors"
	"math/big"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func TestParsePrincipal(t *testing.T) {
	valid := map[string]Principal{
		"edgectl://ghost/ghost.pi":       {Role: RoleGhost, ID: "ghost.pi"},
		"edgectl://mirage/mirage.mac":    {Role: RoleMirage, ID: "mirage.mac"},
		"edgectl://operator/dan":         {Role: RoleOperator, ID: "dan"},
		" edgectl://ghost/ghost-01_a.b ": {Role: RoleGhost, ID: "ghost-01_a.b"},
	}
	for raw, want := range valid {
		got, err := ParsePrincipal(raw)
		if err != nil || got != want {
			t.Errorf("%q: got %+v err=%v, want %+v", raw, got, err, want)
		}
	}
	invalid := []string{
		"", "ghost/ghost.pi", "https://ghost/ghost.pi", "edgectl://admin/x", "edgectl://ghost/",
		"edgectl://ghost/Ghost.Pi", "edgectl://ghost/-x", "edgectl://ghost/a/b", "edgectl://ghost/a?x=1",
		"edgectl://ghost/a#f", "edgectl://user@ghost/a", "edgectl://ghost:7/a", "edgectl:ghost/a",
	}
	for _, raw := range invalid {
		if _, err := ParsePrincipal(raw); !errors.Is(err, ErrInvalidPrincipal) {
			t.Errorf("%q: expected ErrInvalidPrincipal, got %v", raw, err)
		}
	}
}

func certWithURIs(raw ...string) *x509.Certificate {
	c := &x509.Certificate{}
	for _, r := range raw {
		u, _ := url.Parse(r)
		c.URIs = append(c.URIs, u)
	}
	return c
}

func TestPrincipalFromCertRequiresExactlyOneURI(t *testing.T) {
	if _, err := PrincipalFromCert(certWithURIs()); !errors.Is(err, ErrNoPrincipal) {
		t.Fatalf("no URI: expected ErrNoPrincipal, got %v", err)
	}
	if _, err := PrincipalFromCert(certWithURIs("edgectl://ghost/a", "edgectl://ghost/b")); !errors.Is(err, ErrMultiplePrincipals) {
		t.Fatalf("two URIs: expected ErrMultiplePrincipals, got %v", err)
	}
	// Common name is ignored even when it looks like an identity.
	c := certWithURIs()
	c.Subject.CommonName = "ghost.pi"
	if _, err := PrincipalFromCert(c); !errors.Is(err, ErrNoPrincipal) {
		t.Fatalf("CN-only: expected ErrNoPrincipal, got %v", err)
	}
	p, err := RequireRole(certWithURIs("edgectl://ghost/ghost.pi"), RoleGhost)
	if err != nil || p.ID != "ghost.pi" {
		t.Fatalf("ghost role: %+v %v", p, err)
	}
	if _, err := RequireRole(certWithURIs("edgectl://operator/dan"), RoleGhost); !errors.Is(err, ErrWrongRole) {
		t.Fatalf("operator as ghost: expected ErrWrongRole, got %v", err)
	}
}

func TestCheckPrivateKeyFile(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good.key")
	bad := filepath.Join(dir, "bad.key")
	if err := os.WriteFile(good, []byte("k"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bad, []byte("k"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := CheckPrivateKeyFile(good); err != nil {
		t.Fatalf("0600 key rejected: %v", err)
	}
	if err := CheckPrivateKeyFile(bad); !errors.Is(err, ErrKeyFilePermissions) {
		t.Fatalf("0640 key: expected ErrKeyFilePermissions, got %v", err)
	}
}

func TestDeniedSerials(t *testing.T) {
	d, err := ParseDeniedSerials([]string{"1A:2B", "0x00ff", " "})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !d.Contains(big.NewInt(0x1a2b)) || !d.Contains(big.NewInt(0xff)) || d.Contains(big.NewInt(7)) {
		t.Fatalf("unexpected membership: %v", d)
	}
	if _, err := ParseDeniedSerials([]string{"xyz"}); !errors.Is(err, ErrInvalidSerial) {
		t.Fatalf("expected ErrInvalidSerial, got %v", err)
	}
}
