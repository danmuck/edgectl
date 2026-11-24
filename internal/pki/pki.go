// Package pki issues edgectl root and leaf certificates for operator tooling and tests.
//
// Runtimes (Mirage, Ghost) MUST NOT import this package: certificates are minted only by
// operator tooling (cmd/edgecert via make certs-*), per admin_security.toml [trust].
//
// Canonical references (consult before changes):
// - docs/architecture/definitions/admin_security.toml
package pki

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/url"
	"os"
	"time"

	"github.com/danmuck/edgectl/internal/identity"
)

// MaxLeafDays is the contract cap on leaf certificate lifetime.
const MaxLeafDays = 90

// ErrLeafLifetime reports a requested leaf lifetime outside 1..MaxLeafDays.
var ErrLeafLifetime = errors.New("pki: leaf lifetime must be 1..90 days")

// Authority is a CA certificate and its private key.
type Authority struct {
	Cert *x509.Certificate
	Key  *ecdsa.PrivateKey
}

// Leaf is an issued leaf certificate and key.
type Leaf struct {
	Cert *x509.Certificate
	Key  *ecdsa.PrivateKey
	DER  []byte
}

// NewRoot creates a self-signed ECDSA P-256 root CA valid for days.
func NewRoot(commonName string, days int, now time.Time) (*Authority, []byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	serial, err := randomSerial()
	if err != nil {
		return nil, nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: commonName},
		NotBefore:             now.Add(-5 * time.Minute),
		NotAfter:              now.AddDate(0, 0, days),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            0,
		MaxPathLenZero:        true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, nil, err
	}
	return &Authority{Cert: cert, Key: key}, der, nil
}

// IssueLeaf issues an ECDSA P-256 leaf for principal with serverAuth+clientAuth, exactly one URI
// SAN (the principal), and optional DNS/IP SANs for TLS hostname verification.
func (a *Authority) IssueLeaf(p identity.Principal, dnsNames []string, ips []net.IP, days int, now time.Time) (*Leaf, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	if days < 1 || days > MaxLeafDays {
		return nil, fmt.Errorf("%w: got %d", ErrLeafLifetime, days)
	}
	uri, err := url.Parse(p.String())
	if err != nil {
		return nil, err
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	serial, err := randomSerial()
	if err != nil {
		return nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: p.ID},
		NotBefore:    now.Add(-5 * time.Minute),
		NotAfter:     now.AddDate(0, 0, days),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		URIs:         []*url.URL{uri},
		DNSNames:     dnsNames,
		IPAddresses:  ips,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, a.Cert, &key.PublicKey, a.Key)
	if err != nil {
		return nil, err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	return &Leaf{Cert: cert, Key: key, DER: der}, nil
}

// LoadAuthority reads a PEM CA certificate and PEM EC private key (key must be mode 0600).
func LoadAuthority(certPath, keyPath string) (*Authority, error) {
	if err := identity.CheckPrivateKeyFile(keyPath); err != nil {
		return nil, err
	}
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(certPEM)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("pki: %s is not a PEM certificate", certPath)
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, err
	}
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}
	kb, _ := pem.Decode(keyPEM)
	if kb == nil || kb.Type != "EC PRIVATE KEY" {
		return nil, fmt.Errorf("pki: %s is not a PEM EC private key", keyPath)
	}
	key, err := x509.ParseECPrivateKey(kb.Bytes)
	if err != nil {
		return nil, err
	}
	if !cert.IsCA {
		return nil, fmt.Errorf("pki: %s is not a CA certificate", certPath)
	}
	return &Authority{Cert: cert, Key: key}, nil
}

// WriteCertPEM writes a certificate world-readable (0644).
func WriteCertPEM(path string, der []byte) error {
	return writeNew(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644)
}

// WriteKeyPEM writes an EC private key owner-only (0600).
func WriteKeyPEM(path string, key *ecdsa.PrivateKey) error {
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return err
	}
	return writeNew(path, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der}), 0o600)
}

// Refuses to overwrite existing files so credentials are never silently replaced.
func writeNew(path string, data []byte, perm os.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// 128-bit random positive serial.
func randomSerial() (*big.Int, error) {
	return rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
}
