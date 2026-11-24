package tlstest

import (
	"net"
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
