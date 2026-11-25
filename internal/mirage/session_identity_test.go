package mirage

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net"
	"os"
	"testing"
	"time"

	"github.com/danmuck/edgectl/internal/ghost"
	"github.com/danmuck/edgectl/internal/identity"
	"github.com/danmuck/edgectl/internal/protocol/session"
	"github.com/danmuck/edgectl/internal/testutil/testlog"
	"github.com/danmuck/edgectl/internal/testutil/tlstest"
)

// mTLS Mirage session server; returns the address, the CA, and a stop func.
func startTLSSessionServer(t *testing.T, dir string, ca *tlstest.Authority, denied []string) string {
	t.Helper()
	serverCert, serverKey := ca.IssuePrincipalCert(t, dir, "edgectl://mirage/mirage.local", []string{"mirage.local"}, []net.IP{net.ParseIP("127.0.0.1")})
	cfg := DefaultServiceConfig()
	cfg.RequireIdentityBinding = true
	cfg.Session.SecurityMode = session.SecurityModeProduction
	cfg.Session.TLS = session.TLSConfig{
		Enabled: true, Mutual: true, CertFile: serverCert, KeyFile: serverKey, CAFile: ca.CAFile(), DeniedSerials: denied,
	}
	cfg.Session.HandshakeTimeout = 2 * time.Second
	cfg.Session.ReadTimeout = 2 * time.Second
	cfg.Session.WriteTimeout = 2 * time.Second
	svc := NewServiceWithConfig(cfg)
	tlsCfg, err := svc.serverTLSConfig()
	if err != nil {
		t.Fatalf("server tls config: %v", err)
	}
	ln, err := tls.Listen("tcp", "127.0.0.1:0", tlsCfg)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- svc.Serve(ctx, ln) }()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	return ln.Addr().String()
}

// Attempts registration as ghost.alpha with the given client certificate.
func registerWithCert(t *testing.T, addr string, ca *tlstest.Authority, certFile, keyFile string) error {
	t.Helper()
	sc := session.DefaultConfig()
	sc.SecurityMode = session.SecurityModeProduction
	sc.TLS = session.TLSConfig{Enabled: true, Mutual: true, CertFile: certFile, KeyFile: keyFile, CAFile: ca.CAFile(), ServerName: "mirage.local"}
	sc.ConnectTimeout, sc.HandshakeTimeout = 2*time.Second, 2*time.Second
	client, err := ghost.NewMirageClient(ghost.MirageClientConfig{
		Address: addr, GhostID: "ghost.alpha", PeerIdentity: "ghost.alpha",
		SeedList: []session.SeedInfo{}, Session: sc, MaxConnectAttempts: 1,
		MiragePrincipal: "edgectl://mirage/mirage.local",
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	gs, err := client.ConnectAndRegister(ctx)
	if err == nil {
		gs.Close()
	}
	return err
}

func certSerialHex(t *testing.T, certFile string) string {
	t.Helper()
	raw, err := os.ReadFile(certFile)
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(raw)
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	return cert.SerialNumber.Text(16)
}

// Session identity comes only from a ghost-role URI SAN principal.
func TestSessionRejectsCertificatesWithoutGhostPrincipal(t *testing.T) {
	testlog.Start(t)
	dir := t.TempDir()
	ca := tlstest.NewAuthority(t, dir, "edgectl-test-ca")
	addr := startTLSSessionServer(t, dir, ca, nil)

	// CN equals the ghost_id, but there is no URI SAN: rejected.
	cnCert, cnKey := ca.IssueClientCert(t, dir, "ghost.alpha")
	if err := registerWithCert(t, addr, ca, cnCert, cnKey); err == nil {
		t.Fatalf("CN-only certificate must not register")
	}
	// Valid principal with the wrong role: rejected.
	opCert, opKey := ca.IssuePrincipalCert(t, dir, "edgectl://operator/ghost.alpha", nil, nil)
	if err := registerWithCert(t, addr, ca, opCert, opKey); err == nil {
		t.Fatalf("operator principal must not register as a ghost")
	}
	// Correct ghost principal: accepted.
	ghCert, ghKey := ca.IssuePrincipalCert(t, dir, "edgectl://ghost/ghost.alpha", nil, nil)
	if err := registerWithCert(t, addr, ca, ghCert, ghKey); err != nil {
		t.Fatalf("ghost principal should register: %v", err)
	}
}

// A denied serial fails the TLS handshake even though the certificate chains to the CA.
func TestSessionRejectsDeniedSerial(t *testing.T) {
	testlog.Start(t)
	dir := t.TempDir()
	ca := tlstest.NewAuthority(t, dir, "edgectl-test-ca")
	ghCert, ghKey := ca.IssuePrincipalCert(t, dir, "edgectl://ghost/ghost.alpha", nil, nil)
	addr := startTLSSessionServer(t, dir, ca, []string{certSerialHex(t, ghCert)})

	err := registerWithCert(t, addr, ca, ghCert, ghKey)
	if err == nil || errors.Is(err, ghost.ErrRegistrationRejected) {
		t.Fatalf("revoked certificate must fail at the handshake, got %v", err)
	}
}

// Runtimes refuse to load a private key that group or other can read.
func TestServerTLSConfigRefusesOpenKeyFile(t *testing.T) {
	testlog.Start(t)
	dir := t.TempDir()
	ca := tlstest.NewAuthority(t, dir, "edgectl-test-ca")
	cert, key := ca.IssueServerCert(t, dir, "mirage.local", []string{"mirage.local"}, nil)
	if err := os.Chmod(key, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := DefaultServiceConfig()
	cfg.Session.TLS = session.TLSConfig{Enabled: true, CertFile: cert, KeyFile: key}
	if _, err := NewServiceWithConfig(cfg).serverTLSConfig(); !errors.Is(err, identity.ErrKeyFilePermissions) {
		t.Fatalf("expected ErrKeyFilePermissions, got %v", err)
	}
}
