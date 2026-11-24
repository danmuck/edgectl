package ghost

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"os"
	"testing"

	"github.com/danmuck/edgectl/internal/adminauth"
	"github.com/danmuck/edgectl/internal/identity"
	"github.com/danmuck/edgectl/internal/protocol/schema"
	"github.com/danmuck/edgectl/internal/testutil/testlog"
	"github.com/danmuck/edgectl/internal/testutil/tlstest"
)

// Raw client presenting cert/key (if any) without any client-side identity checks, so the test
// observes only what the server enforces.
func rawClient(t *testing.T, certFile, keyFile string) *tls.Config {
	t.Helper()
	cfg := &tls.Config{MinVersion: tls.VersionTLS13, InsecureSkipVerify: true}
	if certFile != "" {
		pair, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			t.Fatal(err)
		}
		cfg.Certificates = []tls.Certificate{pair}
	}
	return cfg
}

// Rejected at the transport: the connection fails or the server answers 1000 (transport_failure).
func refused(resp controlResponse, err error) bool {
	return err != nil || (!resp.OK && resp.ErrorCode == schema.ErrCodeTransportFailure)
}

// Slice F negative matrix for the Ghost admin endpoint (admin_security.toml [transport],
// [authorization.ghost_admin], [trust], [identity]): every case sends a privileged execute_envelope
// and must be refused without executing.
func TestGhostAdminEndpointNegativeMatrix(t *testing.T) {
	testlog.Start(t)
	p := tlstest.NewPKI(t)
	gCert, gKey, _ := p.Issue(t, identity.RoleGhost, "ghost.alpha")
	mCert, mKey, _ := p.Issue(t, identity.RoleMirage, "mirage.main")
	otherMirage, otherMirageKey, _ := p.Issue(t, identity.RoleMirage, "mirage.other")
	peerGhost, peerGhostKey, _ := p.Issue(t, identity.RoleGhost, "ghost.beta")
	revoked, revokedKey, revokedSerial := p.Issue(t, identity.RoleOperator, "revoked")
	expired, expiredKey := p.IssueExpired(t, identity.RoleMirage, "mirage.expired")
	multi, multiKey := p.IssueMultiURI(t, "multi",
		identity.Principal{Role: identity.RoleMirage, ID: "mirage.main"}, identity.Principal{Role: identity.RoleOperator, ID: "dan"})
	foreign := tlstest.NewPKI(t)
	fCert, fKey, _ := foreign.Issue(t, identity.RoleMirage, "mirage.main")

	cfg := DefaultServiceConfig()
	cfg.GhostID = "ghost.alpha"
	cfg.AdminTLS = adminauth.Credentials{CertFile: gCert, KeyFile: gKey, CAFile: p.CAFile(), DeniedSerials: []string{revokedSerial}}
	cfg.MiragePrincipal = "edgectl://mirage/mirage.main"
	svc := NewServiceWithConfig(cfg)
	svc.server = newRadiatingServer(t, "ghost.alpha")
	addr := serveAdmin(t, svc, true)

	cases := []struct {
		name string
		cfg  *tls.Config
	}{
		{"no client certificate", rawClient(t, "", "")},
		{"wrong CA", rawClient(t, fCert, fKey)},
		{"expired", rawClient(t, expired, expiredKey)},
		{"revoked serial", rawClient(t, revoked, revokedKey)},
		{"wrong role (peer ghost)", rawClient(t, peerGhost, peerGhostKey)},
		{"wrong pinned mirage", rawClient(t, otherMirage, otherMirageKey)},
		{"multiple URI SANs", rawClient(t, multi, multiKey)},
		{"plaintext client", nil},
	}
	for i, c := range cases {
		id := "cmd.neg." + string(rune('a'+i))
		resp, err := adminCall(t, addr, c.cfg, controlRequest{Action: "execute_envelope", CommandFrame: flowCommandFrame(t, id)})
		if !refused(resp, err) {
			t.Errorf("%s: not refused: ok=%v code=%d err=%v %s", c.name, resp.OK, resp.ErrorCode, err, resp.Error)
		}
		if _, ok := svc.server.ExecutionByCommandID(id); ok {
			t.Errorf("%s: refused command executed", c.name)
		}
	}

	// Control: the pinned Mirage principal executes over the same listener.
	resp, err := adminCall(t, addr, rawClient(t, mCert, mKey), controlRequest{Action: "execute_envelope", CommandFrame: flowCommandFrame(t, "cmd.neg.ok")})
	if err != nil || !resp.OK {
		t.Fatalf("pinned mirage: ok=%v err=%v %s", resp.OK, err, resp.Error)
	}

	// Plaintext on a non-loopback address never listens, in any mode.
	plain := NewServiceWithConfig(DefaultServiceConfig())
	if err := plain.serveAdminControl(context.Background(), "0.0.0.0:0"); !errors.Is(err, adminauth.ErrPlaintextNotAllowed) {
		t.Fatalf("plaintext non-loopback: expected ErrPlaintextNotAllowed, got %v", err)
	}
}

// Server-side expiry is enforced by the chain, not by the client: sanity-check the fixture.
func TestExpiredFixtureIsExpired(t *testing.T) {
	p := tlstest.NewPKI(t)
	certFile, _ := p.IssueExpired(t, identity.RoleMirage, "mirage.expired")
	raw, err := os.ReadFile(p.CAFile())
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM(raw)
	pair, err := tls.LoadX509KeyPair(certFile, certFile[:len(certFile)-4]+".key")
	if err != nil {
		t.Fatal(err)
	}
	leaf, _ := x509.ParseCertificate(pair.Certificate[0])
	if _, err := leaf.Verify(x509.VerifyOptions{Roots: roots, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}); err == nil {
		t.Fatalf("expired fixture verifies")
	}
}
