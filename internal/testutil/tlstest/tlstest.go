package tlstest

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type Authority struct {
	cert   *x509.Certificate
	key    *rsa.PrivateKey
	caPath string
}

func NewAuthority(t testing.TB, dir string, commonName string) *Authority {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate ca key: %v", err)
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: commonName},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            1,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create ca cert: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse ca cert: %v", err)
	}

	caPath := filepath.Join(dir, "ca.crt")
	if err := writePEM(caPath, "CERTIFICATE", der, 0o644); err != nil {
		t.Fatalf("write ca cert: %v", err)
	}

	return &Authority{
		cert:   cert,
		key:    key,
		caPath: caPath,
	}
}

func (a *Authority) CAFile() string {
	return a.caPath
}

func (a *Authority) IssueServerCert(t testing.TB, dir string, commonName string, dnsNames []string, ips []net.IP) (string, string) {
	t.Helper()
	return a.issueCert(t, dir, commonName, x509.ExtKeyUsageServerAuth, dnsNames, ips)
}

func (a *Authority) IssueClientCert(t testing.TB, dir string, commonName string) (string, string) {
	t.Helper()
	return a.issueCert(t, dir, commonName, x509.ExtKeyUsageClientAuth, nil, nil)
}

func (a *Authority) issueCert(
	t testing.TB,
	dir string,
	commonName string,
	usage x509.ExtKeyUsage,
	dnsNames []string,
	ips []net.IP,
) (string, string) {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(now.UnixNano()),
		Subject:      pkix.Name{CommonName: commonName},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{usage},
		DNSNames:     dnsNames,
		IPAddresses:  ips,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, a.cert, &key.PublicKey, a.key)
	if err != nil {
		t.Fatalf("create signed cert: %v", err)
	}

	base := sanitize(commonName)
	certPath := filepath.Join(dir, fmt.Sprintf("%s.crt", base))
	keyPath := filepath.Join(dir, fmt.Sprintf("%s.key", base))

	if err := writePEM(certPath, "CERTIFICATE", der, 0o644); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	keyDER := x509.MarshalPKCS1PrivateKey(key)
	if err := writePEM(keyPath, "RSA PRIVATE KEY", keyDER, 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	return certPath, keyPath
}

func writePEM(path string, blockType string, der []byte, perm os.FileMode) error {
	data := pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: der})
	return os.WriteFile(path, data, perm)
}

func sanitize(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "cert"
	}
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, ":", "_")
	return s
}
