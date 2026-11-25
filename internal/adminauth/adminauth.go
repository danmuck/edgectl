// Package adminauth secures the Mirage and Ghost admin boundaries with mutual TLS
// (docs/architecture/definitions/admin_security.toml [transport]).
//
// Ownership boundary:
// - admin server and client TLS configs (TLS 1.3, client certificates required)
//
// - listener rule: plaintext only on loopback in development; never in production
//
// - per-connection peer principal for authorization
//
// - privileged action set that always requires an authenticated (mTLS) peer
//
// - audit records (admin_security.toml [audit]): written at any log level; write failures reported
//
// - bounded admin request reads: line size cap and idle deadline (request.go)
//
// Canonical references (consult before changes):
// - docs/index.md
//
// - docs/architecture/definitions/admin_security.toml
//
// - docs/progress/admin_security.md
package adminauth

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/danmuck/edgectl/internal/identity"
)

var (
	// Plaintext admin listener requested where the contract forbids it.
	ErrPlaintextNotAllowed = errors.New("adminauth: plaintext admin listener not allowed")
	// Action requires an mTLS-authenticated peer.
	ErrAuthRequired = errors.New("adminauth: authenticated peer required")
	// Remote principal is not the one this dialer expects.
	ErrUnexpectedPeer = errors.New("adminauth: unexpected peer principal")
	// Local certificate does not carry the principal this runtime runs as.
	ErrSelfIdentity = errors.New("adminauth: certificate principal does not match runtime identity")
)

// Credentials are the TLS material for one admin identity. Empty CertFile means TLS disabled.
type Credentials struct {
	CertFile      string
	KeyFile       string
	CAFile        string
	DeniedSerials []string
}

// Enabled reports whether admin TLS is configured.
func (c Credentials) Enabled() bool {
	return strings.TrimSpace(c.CertFile) != ""
}

// Peer is the caller on one admin connection; Authenticated is false for plaintext connections.
type Peer struct {
	Principal     identity.Principal
	Authenticated bool
	Addr          string
}

// privileged lists actions that require mTLS in every mode (admin_security.toml authorization.privileged).
var privileged = map[string]struct{}{
	"execute_envelope":     {},
	"install_dependencies": {},
	"spawn_ghost":          {},
	"bind_mirage":          {},
	"deploy_ghost":         {},
	"approve_dependencies": {},
	"execute_dependencies": {},
}

// Privileged reports whether action always requires an authenticated peer.
func Privileged(action string) bool {
	_, ok := privileged[strings.TrimSpace(action)]
	return ok
}

// Loads the key pair (0600 enforced) and checks the leaf carries the expected self principal.
func loadSelf(creds Credentials, self identity.Principal) (tls.Certificate, error) {
	cert, err := identity.LoadKeyPair(creds.CertFile, creds.KeyFile)
	if err != nil {
		return tls.Certificate{}, err
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return tls.Certificate{}, err
	}
	got, err := identity.PrincipalFromCert(leaf)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("%w: %v", ErrSelfIdentity, err)
	}
	if got != self {
		return tls.Certificate{}, fmt.Errorf("%w: certificate is %s, runtime is %s", ErrSelfIdentity, got, self)
	}
	return cert, nil
}

func loadPool(caFile string) (*x509.CertPool, error) {
	pem, err := os.ReadFile(caFile)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("adminauth: no certificates in %s", caFile)
	}
	return pool, nil
}

// ServerTLSConfig requires and verifies client certificates against the root, rejects denied
// serials and certificates without a valid principal, and serves as self.
func ServerTLSConfig(creds Credentials, self identity.Principal) (*tls.Config, error) {
	cert, err := loadSelf(creds, self)
	if err != nil {
		return nil, err
	}
	pool, err := loadPool(creds.CAFile)
	if err != nil {
		return nil, err
	}
	denied, err := identity.ParseDeniedSerials(creds.DeniedSerials)
	if err != nil {
		return nil, err
	}
	return &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    pool,
		VerifyPeerCertificate: func(raw [][]byte, chains [][]*x509.Certificate) error {
			if err := denied.VerifyPeerNotDenied(raw, chains); err != nil {
				return err
			}
			leaf, err := x509.ParseCertificate(raw[0])
			if err != nil {
				return err
			}
			_, err = identity.PrincipalFromCert(leaf)
			return err
		},
	}, nil
}

// ClientTLSConfig dials as self and authenticates the server by chain (to the root, serverAuth)
// and principal rather than hostname; verifyServer decides which server principal is acceptable.
func ClientTLSConfig(creds Credentials, self identity.Principal, verifyServer func(identity.Principal) error) (*tls.Config, error) {
	cert, err := loadSelf(creds, self)
	if err != nil {
		return nil, err
	}
	pool, err := loadPool(creds.CAFile)
	if err != nil {
		return nil, err
	}
	denied, err := identity.ParseDeniedSerials(creds.DeniedSerials)
	if err != nil {
		return nil, err
	}
	return &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{cert},
		// Hostname verification is replaced by full chain + principal verification below.
		InsecureSkipVerify: true,
		VerifyConnection: func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) == 0 {
				return fmt.Errorf("%w: server presented no certificate", ErrUnexpectedPeer)
			}
			leaf := cs.PeerCertificates[0]
			inter := x509.NewCertPool()
			for _, c := range cs.PeerCertificates[1:] {
				inter.AddCert(c)
			}
			if _, err := leaf.Verify(x509.VerifyOptions{
				Roots:         pool,
				Intermediates: inter,
				KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
			}); err != nil {
				return err
			}
			if denied.Contains(leaf.SerialNumber) {
				return fmt.Errorf("%w: serial %s", identity.ErrCertificateRevoked, leaf.SerialNumber.Text(16))
			}
			p, err := identity.PrincipalFromCert(leaf)
			if err != nil {
				return err
			}
			return verifyServer(p)
		},
	}, nil
}

// ExpectPrincipal accepts exactly want.
func ExpectPrincipal(want identity.Principal) func(identity.Principal) error {
	return func(got identity.Principal) error {
		if got != want {
			return fmt.Errorf("%w: got %s, want %s", ErrUnexpectedPeer, got, want)
		}
		return nil
	}
}

// ExpectRole accepts any principal with role.
func ExpectRole(role identity.Role) func(identity.Principal) error {
	return func(got identity.Principal) error {
		if got.Role != role {
			return fmt.Errorf("%w: got %s, want role %s", ErrUnexpectedPeer, got, role)
		}
		return nil
	}
}

// Listen opens an admin listener: mTLS when credentials are set; otherwise plaintext only when
// not in production and bound to a loopback address.
func Listen(addr string, creds Credentials, self identity.Principal, production bool) (net.Listener, error) {
	addr = strings.TrimSpace(addr)
	if creds.Enabled() {
		cfg, err := ServerTLSConfig(creds, self)
		if err != nil {
			return nil, err
		}
		return tls.Listen("tcp", addr, cfg)
	}
	if production {
		return nil, fmt.Errorf("%w: production mode requires admin TLS (%s)", ErrPlaintextNotAllowed, addr)
	}
	if !IsLoopbackAddr(addr) {
		return nil, fmt.Errorf("%w: %s is not a loopback address; configure admin TLS", ErrPlaintextNotAllowed, addr)
	}
	return net.Listen("tcp", addr)
}

// IsLoopbackAddr reports whether host:port binds only to loopback (empty host means all interfaces).
func IsLoopbackAddr(addr string) bool {
	host, _, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil || host == "" {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// PeerFromConn completes the TLS handshake (bounded by timeout) and returns the verified caller
// principal; plaintext connections return an unauthenticated Peer.
func PeerFromConn(conn net.Conn, timeout time.Duration) (Peer, error) {
	peer := Peer{Addr: conn.RemoteAddr().String()}
	tlsConn, ok := conn.(*tls.Conn)
	if !ok {
		return peer, nil
	}
	_ = tlsConn.SetDeadline(time.Now().Add(timeout))
	if err := tlsConn.Handshake(); err != nil {
		return peer, err
	}
	_ = tlsConn.SetDeadline(time.Time{})
	state := tlsConn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		return peer, ErrAuthRequired
	}
	p, err := identity.PrincipalFromCert(state.PeerCertificates[0])
	if err != nil {
		return peer, err
	}
	peer.Principal = p
	peer.Authenticated = true
	return peer, nil
}

// Dial connects to an admin endpoint, over TLS when cfg is non-nil.
func Dial(ctx context.Context, addr string, cfg *tls.Config, timeout time.Duration) (net.Conn, error) {
	d := &net.Dialer{Timeout: timeout}
	if cfg == nil {
		return d.DialContext(ctx, "tcp", addr)
	}
	td := &tls.Dialer{NetDialer: d, Config: cfg}
	return td.DialContext(ctx, "tcp", addr)
}

// SelfFromCert reads the principal of creds' own certificate and requires role; used by callers
// (e.g. operator tooling) whose identity is defined by the certificate rather than runtime config.
func SelfFromCert(creds Credentials, role identity.Role) (identity.Principal, error) {
	pemBytes, err := os.ReadFile(creds.CertFile)
	if err != nil {
		return identity.Principal{}, err
	}
	block, _ := pem.Decode(pemBytes)
	if block == nil || block.Type != "CERTIFICATE" {
		return identity.Principal{}, fmt.Errorf("adminauth: %s is not a PEM certificate", creds.CertFile)
	}
	leaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return identity.Principal{}, err
	}
	return identity.RequireRole(leaf, role)
}
