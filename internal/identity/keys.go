package identity

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"math/big"
	"os"
	"strings"
)

var (
	// Private key file is readable or writable by group or other.
	ErrKeyFilePermissions = errors.New("identity: private key file permissions too open")
	// Peer certificate serial is on the denied list.
	ErrCertificateRevoked = errors.New("identity: certificate serial is denied")
	// Denied-serial entry is not a hex serial number.
	ErrInvalidSerial = errors.New("identity: invalid denied serial")
)

// CheckPrivateKeyFile refuses a key file that group or other can access
// (admin_security.toml trust.private_key_file_mode).
func CheckPrivateKeyFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		return fmt.Errorf("%w: %s has mode %04o, want 0600", ErrKeyFilePermissions, path, perm)
	}
	return nil
}

// LoadKeyPair checks private key file mode, then loads the certificate and key.
func LoadKeyPair(certFile, keyFile string) (tls.Certificate, error) {
	if err := CheckPrivateKeyFile(keyFile); err != nil {
		return tls.Certificate{}, err
	}
	return tls.LoadX509KeyPair(certFile, keyFile)
}

// DeniedSerials is a revocation list of certificate serial numbers (admin_security.toml trust.revocation).
type DeniedSerials map[string]struct{}

// ParseDeniedSerials accepts hex serials as printed by openssl or edgecert ("1A2B", "1a:2b").
func ParseDeniedSerials(in []string) (DeniedSerials, error) {
	out := make(DeniedSerials, len(in))
	for _, raw := range in {
		s := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(raw), ":", ""))
		s = strings.TrimPrefix(s, "0x")
		if s == "" {
			continue
		}
		n, ok := new(big.Int).SetString(s, 16)
		if !ok {
			return nil, fmt.Errorf("%w: %q", ErrInvalidSerial, raw)
		}
		out[n.Text(16)] = struct{}{}
	}
	return out, nil
}

// Contains reports whether serial is denied.
func (d DeniedSerials) Contains(serial *big.Int) bool {
	if len(d) == 0 || serial == nil {
		return false
	}
	_, ok := d[serial.Text(16)]
	return ok
}

// VerifyPeerNotDenied is a tls.Config.VerifyPeerCertificate hook rejecting denied leaf serials.
// It runs after standard chain verification, on every handshake.
func (d DeniedSerials) VerifyPeerNotDenied(rawCerts [][]byte, _ [][]*x509.Certificate) error {
	if len(d) == 0 || len(rawCerts) == 0 {
		return nil
	}
	leaf, err := x509.ParseCertificate(rawCerts[0])
	if err != nil {
		return err
	}
	if d.Contains(leaf.SerialNumber) {
		return fmt.Errorf("%w: serial %s", ErrCertificateRevoked, leaf.SerialNumber.Text(16))
	}
	return nil
}
