// Package approval implements operator-signed dependency install approvals
// (docs/architecture/definitions/admin_security.toml [install_approval]).
//
// Ownership boundary:
// - canonical approval payload and its sha256 digest
//
// - ECDSA P-256 signing with an operator key (operator tooling only)
//
// - verification: chain to the root, operator role, signature, expiry, separation of duties
//
// Mirage verifies before recording an approval; Ghost verifies again before installing, so a
// compromised Mirage cannot forge an approval.
//
// Canonical references (consult before changes):
// - docs/index.md
//
// - docs/architecture/definitions/admin_security.toml
//
// - docs/progress/admin_security.md
package approval

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/danmuck/edgectl/internal/identity"
	"github.com/danmuck/edgectl/internal/seeds"
)

// MaxLifetime caps approval validity (admin_security.toml install_approval.expiry_max_minutes).
const MaxLifetime = 15 * time.Minute

// clockSkew tolerates small clock differences between operator, Mirage, and Ghost hosts.
const clockSkew = time.Minute

var (
	// Approval is malformed, does not verify, or is not from an operator under the root.
	ErrInvalidApproval = errors.New("approval: invalid approval")
	// Approval is past expires_at_ms or claims a lifetime beyond MaxLifetime.
	ErrApprovalExpired = errors.New("approval: expired or lifetime too long")
	// Approver is the proposer (admin_security.toml install_approval.separation_of_duties).
	ErrSeparationOfDuties = errors.New("approval: approver must differ from proposer")
)

// Payload is the signed content (install_approval.signed_payload). Field order is fixed, so its
// JSON encoding is canonical for the digest.
type Payload struct {
	ProposalID  string              `json:"proposal_id"`
	GhostID     string              `json:"ghost_id"`
	ProposedBy  string              `json:"proposed_by"`
	Steps       []seeds.InstallSpec `json:"steps"`
	ExpiresAtMS uint64              `json:"expires_at_ms"`
	Nonce       string              `json:"nonce"`
}

// Digest is sha256 over the canonical JSON encoding of p.
func (p Payload) Digest() ([32]byte, error) {
	raw, err := json.Marshal(p)
	if err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(raw), nil
}

// Signed is a payload with the approver's signature and certificate.
type Signed struct {
	Payload Payload `json:"payload"`
	// ASN.1 ECDSA signature over Payload.Digest().
	Signature []byte `json:"signature"`
	// Approver leaf certificate (DER).
	ApproverCert []byte `json:"approver_cert"`
}

// SignatureDigest is a short audit identifier for the signature (hex sha256, first 16 chars).
func (s Signed) SignatureDigest() string {
	sum := sha256.Sum256(s.Signature)
	return hex.EncodeToString(sum[:])[:16]
}

// Signer holds an operator's key and certificate.
type Signer struct {
	key       *ecdsa.PrivateKey
	certDER   []byte
	Principal identity.Principal
}

// LoadSigner reads an operator certificate and EC private key (key must be mode 0600).
func LoadSigner(certFile, keyFile string) (*Signer, error) {
	if err := identity.CheckPrivateKeyFile(keyFile); err != nil {
		return nil, err
	}
	certPEM, err := os.ReadFile(certFile)
	if err != nil {
		return nil, err
	}
	cb, _ := pem.Decode(certPEM)
	if cb == nil || cb.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("%w: %s is not a PEM certificate", ErrInvalidApproval, certFile)
	}
	cert, err := x509.ParseCertificate(cb.Bytes)
	if err != nil {
		return nil, err
	}
	p, err := identity.RequireRole(cert, identity.RoleOperator)
	if err != nil {
		return nil, err
	}
	keyPEM, err := os.ReadFile(keyFile)
	if err != nil {
		return nil, err
	}
	kb, _ := pem.Decode(keyPEM)
	if kb == nil || kb.Type != "EC PRIVATE KEY" {
		return nil, fmt.Errorf("%w: %s is not a PEM EC private key", ErrInvalidApproval, keyFile)
	}
	key, err := x509.ParseECPrivateKey(kb.Bytes)
	if err != nil {
		return nil, err
	}
	return &Signer{key: key, certDER: cb.Bytes, Principal: p}, nil
}

// NewNonce returns a random 128-bit hex nonce.
func NewNonce() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// Sign signs p; the caller sets ExpiresAtMS (at most MaxLifetime ahead) and Nonce.
func (s *Signer) Sign(p Payload) (Signed, error) {
	digest, err := p.Digest()
	if err != nil {
		return Signed{}, err
	}
	sig, err := ecdsa.SignASN1(rand.Reader, s.key, digest[:])
	if err != nil {
		return Signed{}, err
	}
	return Signed{Payload: p, Signature: sig, ApproverCert: append([]byte{}, s.certDER...)}, nil
}

// Verify checks the approver certificate chains to roots and is an operator, the signature is
// valid over the payload digest, the approval is unexpired with lifetime <= MaxLifetime, and the
// approver differs from the proposer. Returns the approver principal.
func Verify(s Signed, roots *x509.CertPool, now time.Time) (identity.Principal, error) {
	if strings.TrimSpace(s.Payload.ProposalID) == "" || strings.TrimSpace(s.Payload.GhostID) == "" ||
		strings.TrimSpace(s.Payload.Nonce) == "" || len(s.Payload.Steps) == 0 {
		return identity.Principal{}, fmt.Errorf("%w: incomplete payload", ErrInvalidApproval)
	}
	cert, err := x509.ParseCertificate(s.ApproverCert)
	if err != nil {
		return identity.Principal{}, fmt.Errorf("%w: approver certificate: %v", ErrInvalidApproval, err)
	}
	if _, err := cert.Verify(x509.VerifyOptions{
		Roots:       roots,
		CurrentTime: now,
		KeyUsages:   []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}); err != nil {
		return identity.Principal{}, fmt.Errorf("%w: approver certificate: %v", ErrInvalidApproval, err)
	}
	approver, err := identity.RequireRole(cert, identity.RoleOperator)
	if err != nil {
		return identity.Principal{}, fmt.Errorf("%w: %v", ErrInvalidApproval, err)
	}
	pub, ok := cert.PublicKey.(*ecdsa.PublicKey)
	if !ok {
		return identity.Principal{}, fmt.Errorf("%w: approver key is not ECDSA", ErrInvalidApproval)
	}
	digest, err := s.Payload.Digest()
	if err != nil {
		return identity.Principal{}, err
	}
	if !ecdsa.VerifyASN1(pub, digest[:], s.Signature) {
		return identity.Principal{}, fmt.Errorf("%w: signature does not verify", ErrInvalidApproval)
	}
	expires := time.UnixMilli(int64(s.Payload.ExpiresAtMS))
	if !now.Before(expires) {
		return identity.Principal{}, fmt.Errorf("%w: expired at %s", ErrApprovalExpired, expires.Format(time.RFC3339))
	}
	if expires.Sub(now) > MaxLifetime+clockSkew {
		return identity.Principal{}, fmt.Errorf("%w: lifetime exceeds %s", ErrApprovalExpired, MaxLifetime)
	}
	if approver.String() == strings.TrimSpace(s.Payload.ProposedBy) {
		return identity.Principal{}, ErrSeparationOfDuties
	}
	return approver, nil
}
