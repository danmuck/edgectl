package tlstest

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"github.com/danmuck/edgectl/internal/identity"
	"github.com/danmuck/edgectl/internal/pki"
)

// PKI is a temporary edgectl root issuing principal certificates exactly as cmd/edgecert does.
type PKI struct {
	dir    string
	ca     *pki.Authority
	caFile string
}

// NewPKI creates a root CA in a temp directory.
func NewPKI(t testing.TB) *PKI {
	t.Helper()
	dir := t.TempDir()
	ca, der, err := pki.NewRoot("edgectl-test-root", 1, time.Now())
	if err != nil {
		t.Fatalf("new root: %v", err)
	}
	caFile := filepath.Join(dir, "ca.crt")
	if err := pki.WriteCertPEM(caFile, der); err != nil {
		t.Fatalf("write root: %v", err)
	}
	return &PKI{dir: dir, ca: ca, caFile: caFile}
}

// CAFile returns the root certificate path.
func (p *PKI) CAFile() string { return p.caFile }

// Dir returns the directory issued certificates are written to.
func (p *PKI) Dir() string { return p.dir }

// Issue writes <dir>/<id>.crt and <dir>/<id>.key (0600) for edgectl://<role>/<id>; returns paths and serial (hex).
func (p *PKI) Issue(t testing.TB, role identity.Role, id string, ips ...net.IP) (string, string, string) {
	t.Helper()
	leaf, err := p.ca.IssueLeaf(identity.Principal{Role: role, ID: id}, nil, ips, 1, time.Now())
	if err != nil {
		t.Fatalf("issue %s/%s: %v", role, id, err)
	}
	certFile := filepath.Join(p.dir, id+".crt")
	keyFile := filepath.Join(p.dir, id+".key")
	if err := pki.WriteCertPEM(certFile, leaf.DER); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	if err := pki.WriteKeyPEM(keyFile, leaf.Key); err != nil {
		t.Fatalf("write key: %v", err)
	}
	return certFile, keyFile, leaf.Cert.SerialNumber.Text(16)
}

// IssueExpired writes a leaf for edgectl://<role>/<id> from this root whose validity ended a week ago.
// Files are named <id>.crt/.key like Issue.
func (p *PKI) IssueExpired(t testing.TB, role identity.Role, id string) (string, string) {
	t.Helper()
	leaf, err := p.ca.IssueLeaf(identity.Principal{Role: role, ID: id}, nil, nil, 1, time.Now().AddDate(0, 0, -8))
	if err != nil {
		t.Fatalf("issue expired %s/%s: %v", role, id, err)
	}
	return p.write(t, id, leaf.DER, leaf.Key)
}

// IssueMultiURI writes a leaf from this root carrying every principal as a URI SAN; edgectl
// identity rejects such certificates (admin_security.toml identity.multiple_uri_sans).
func (p *PKI) IssueMultiURI(t testing.TB, name string, principals ...identity.Principal) (string, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	uris := make([]*url.URL, 0, len(principals))
	for _, pr := range principals {
		u, err := url.Parse(pr.String())
		if err != nil {
			t.Fatal(err)
		}
		uris = append(uris, u)
	}
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(now.UnixNano()),
		Subject:      pkix.Name{CommonName: name},
		NotBefore:    now.Add(-5 * time.Minute),
		NotAfter:     now.Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		URIs:         uris,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, p.ca.Cert, &key.PublicKey, p.ca.Key)
	if err != nil {
		t.Fatal(err)
	}
	return p.write(t, name, der, key)
}

// Writes <dir>/<name>.crt and <dir>/<name>.key (0600).
func (p *PKI) write(t testing.TB, name string, der []byte, key *ecdsa.PrivateKey) (string, string) {
	t.Helper()
	certFile := filepath.Join(p.dir, name+".crt")
	keyFile := filepath.Join(p.dir, name+".key")
	if err := pki.WriteCertPEM(certFile, der); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	if err := pki.WriteKeyPEM(keyFile, key); err != nil {
		t.Fatalf("write key: %v", err)
	}
	return certFile, keyFile
}
