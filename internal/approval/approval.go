// Package approval implements two-person, operator-signed dependency install approvals
// (docs/architecture/definitions/admin_security.toml [install_approval]).
//
// Ownership boundary:
// - proposer intent: {ghost_id, seed_ids, proposed_at_ms, nonce} signed by the proposing operator
//
// - approval payload: {proposal_id, ghost_id, proposer_intent, steps, expires_at_ms, nonce} signed by a
// different approving operator
//
// - verification of both signatures, roles, separation of duties, expiry, intent age, and that the
// steps are exactly the dependency-catalog steps for the intent's seed ids
//
// Mirage verifies before recording; every Ghost verifies again before installing, so neither a
// compromised Mirage nor a single operator can produce an installable plan.
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
	"reflect"
	"strings"
	"time"

	"github.com/danmuck/edgectl/internal/depcatalog"
	"github.com/danmuck/edgectl/internal/identity"
	"github.com/danmuck/edgectl/internal/seeds"
)

// MaxLifetime caps approval validity (admin_security.toml install_approval.expiry_max_minutes).
const MaxLifetime = 15 * time.Minute

// MaxIntentAge caps how long a proposer intent stays approvable (proposer_intent_max_age_hours).
const MaxIntentAge = 24 * time.Hour

// ClockSkew tolerates small clock differences between operator, Mirage, and Ghost hosts.
const ClockSkew = time.Minute

var (
	// Approval or intent is malformed, does not verify, or is not from an operator under the root.
	ErrInvalidApproval = errors.New("approval: invalid approval")
	// Approval is past expires_at_ms, claims a lifetime beyond MaxLifetime, or the intent is too old.
	ErrApprovalExpired = errors.New("approval: expired or lifetime too long")
	// Approver is the proposer (admin_security.toml install_approval.separation_of_duties).
	ErrSeparationOfDuties = errors.New("approval: approver must differ from proposer")
	// Approved steps are not exactly the catalog steps for the intent's seed ids.
	ErrStepsMismatch = errors.New("approval: steps do not match the catalog for the proposed seeds")
)

// Intent is what the proposer signs: the target ghost and the seeds whose catalog dependencies to install.
type Intent struct {
	GhostID      string   `json:"ghost_id"`
	SeedIDs      []string `json:"seed_ids"`
	ProposedAtMS uint64   `json:"proposed_at_ms"`
	Nonce        string   `json:"nonce"`
}

// SignedIntent is an intent with the proposer's signature and certificate.
type SignedIntent struct {
	Intent       Intent `json:"intent"`
	Signature    []byte `json:"signature"`
	ProposerCert []byte `json:"proposer_cert"`
}

// Payload is what the approver signs (install_approval.signed_payload). Field order is fixed, so its
// JSON encoding is canonical for the digest.
type Payload struct {
	ProposalID     string              `json:"proposal_id"`
	GhostID        string              `json:"ghost_id"`
	ProposerIntent SignedIntent        `json:"proposer_intent"`
	Steps          []seeds.InstallSpec `json:"steps"`
	ExpiresAtMS    uint64              `json:"expires_at_ms"`
	Nonce          string              `json:"nonce"`
}

// Signed is a payload with the approver's signature and certificate.
type Signed struct {
	Payload      Payload `json:"payload"`
	Signature    []byte  `json:"signature"`
	ApproverCert []byte  `json:"approver_cert"`
}

// Result names both verified principals.
type Result struct {
	Proposer identity.Principal
	Approver identity.Principal
}

// StepsFunc derives install steps for seed ids; nil means CatalogSteps.
type StepsFunc func(seedIDs []string) []seeds.InstallSpec

// Trust is what a verifier checks signatures against.
// Roots is the edgectl root; Denied revokes signer certificates by serial (trust.revocation);
// Now is the verification clock; StepsFor derives the expected steps (nil means CatalogSteps).
type Trust struct {
	Roots    *x509.CertPool
	Denied   identity.DeniedSerials
	Now      time.Time
	StepsFor StepsFunc
}

// CatalogSteps returns depcatalog steps for seedIDs in order, exactly as Mirage builds proposals.
func CatalogSteps(seedIDs []string) []seeds.InstallSpec {
	var out []seeds.InstallSpec
	for _, id := range seedIDs {
		for _, e := range depcatalog.CatalogFor(strings.TrimSpace(id)) {
			out = append(out, e.Spec)
		}
	}
	return out
}

func digestJSON(v any) ([32]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(raw), nil
}

// Digest is sha256 over the canonical JSON encoding of i.
func (i Intent) Digest() ([32]byte, error) { return digestJSON(i) }

// Digest is sha256 over the canonical JSON encoding of p.
func (p Payload) Digest() ([32]byte, error) { return digestJSON(p) }

// SignatureDigest is a short audit identifier for the approval signature.
func (s Signed) SignatureDigest() string { return shortDigest(s.Signature) }

// ReplayKey identifies an intent by proposer and content (not signature bytes, which are malleable),
// so one intent is consumed at most once regardless of how its signature is encoded.
func (s SignedIntent) ReplayKey(proposer identity.Principal) (string, error) {
	d, err := s.Intent.Digest()
	if err != nil {
		return "", err
	}
	return proposer.String() + "|" + hex.EncodeToString(d[:]), nil
}

// SignatureDigest is a short audit identifier for the intent signature.
func (s SignedIntent) SignatureDigest() string { return shortDigest(s.Signature) }

func shortDigest(b []byte) string {
	sum := sha256.Sum256(b)
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

func (s *Signer) sign(digest [32]byte) ([]byte, error) {
	return ecdsa.SignASN1(rand.Reader, s.key, digest[:])
}

// SignIntent signs a proposer intent.
func (s *Signer) SignIntent(i Intent) (SignedIntent, error) {
	d, err := i.Digest()
	if err != nil {
		return SignedIntent{}, err
	}
	sig, err := s.sign(d)
	if err != nil {
		return SignedIntent{}, err
	}
	return SignedIntent{Intent: i, Signature: sig, ProposerCert: append([]byte{}, s.certDER...)}, nil
}

// Sign signs an approval payload; the caller sets ExpiresAtMS (at most MaxLifetime ahead) and Nonce.
func (s *Signer) Sign(p Payload) (Signed, error) {
	d, err := p.Digest()
	if err != nil {
		return Signed{}, err
	}
	sig, err := s.sign(d)
	if err != nil {
		return Signed{}, err
	}
	return Signed{Payload: p, Signature: sig, ApproverCert: append([]byte{}, s.certDER...)}, nil
}

// Verifies certDER chains to the root for clientAuth at tr.Now, is not a denied serial, carries an
// operator principal, and signed digest.
func verifyOperatorSignature(certDER []byte, tr Trust, digest [32]byte, sig []byte) (identity.Principal, error) {
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return identity.Principal{}, fmt.Errorf("%w: certificate: %v", ErrInvalidApproval, err)
	}
	if _, err := cert.Verify(x509.VerifyOptions{
		Roots:       tr.Roots,
		CurrentTime: tr.Now,
		KeyUsages:   []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}); err != nil {
		return identity.Principal{}, fmt.Errorf("%w: certificate: %v", ErrInvalidApproval, err)
	}
	if tr.Denied.Contains(cert.SerialNumber) {
		return identity.Principal{}, fmt.Errorf("%w: signer certificate serial is denied", ErrInvalidApproval)
	}
	p, err := identity.RequireRole(cert, identity.RoleOperator)
	if err != nil {
		return identity.Principal{}, fmt.Errorf("%w: %v", ErrInvalidApproval, err)
	}
	pub, ok := cert.PublicKey.(*ecdsa.PublicKey)
	if !ok {
		return identity.Principal{}, fmt.Errorf("%w: key is not ECDSA", ErrInvalidApproval)
	}
	if !ecdsa.VerifyASN1(pub, digest[:], sig) {
		return identity.Principal{}, fmt.Errorf("%w: signature does not verify", ErrInvalidApproval)
	}
	return p, nil
}

// VerifyIntent checks the proposer's signature and role and that the intent is complete and not
// older than MaxIntentAge (or from the future). Returns the proposer principal.
func VerifyIntent(si SignedIntent, tr Trust) (identity.Principal, error) {
	i := si.Intent
	if strings.TrimSpace(i.GhostID) == "" || len(i.SeedIDs) == 0 || strings.TrimSpace(i.Nonce) == "" || i.ProposedAtMS == 0 {
		return identity.Principal{}, fmt.Errorf("%w: incomplete intent", ErrInvalidApproval)
	}
	d, err := i.Digest()
	if err != nil {
		return identity.Principal{}, err
	}
	proposer, err := verifyOperatorSignature(si.ProposerCert, tr, d, si.Signature)
	if err != nil {
		return identity.Principal{}, err
	}
	at := time.UnixMilli(int64(i.ProposedAtMS))
	if at.After(tr.Now.Add(ClockSkew)) || tr.Now.Sub(at) > MaxIntentAge {
		return identity.Principal{}, fmt.Errorf("%w: intent proposed at %s", ErrApprovalExpired, at.Format(time.RFC3339))
	}
	return proposer, nil
}

// Verify checks the proposer intent and the approval: both operator signatures under tr (root and
// denied serials), two different principals, intent and payload name the same ghost, steps are
// exactly tr.StepsFor(intent seed ids), and the approval is unexpired with lifetime <= MaxLifetime.
func Verify(s Signed, tr Trust) (Result, error) {
	pl := s.Payload
	if strings.TrimSpace(pl.ProposalID) == "" || strings.TrimSpace(pl.GhostID) == "" ||
		strings.TrimSpace(pl.Nonce) == "" || len(pl.Steps) == 0 {
		return Result{}, fmt.Errorf("%w: incomplete payload", ErrInvalidApproval)
	}
	proposer, err := VerifyIntent(pl.ProposerIntent, tr)
	if err != nil {
		return Result{}, err
	}
	d, err := pl.Digest()
	if err != nil {
		return Result{}, err
	}
	approver, err := verifyOperatorSignature(s.ApproverCert, tr, d, s.Signature)
	if err != nil {
		return Result{}, err
	}
	if approver == proposer {
		return Result{}, ErrSeparationOfDuties
	}
	if pl.ProposerIntent.Intent.GhostID != pl.GhostID {
		return Result{}, fmt.Errorf("%w: intent ghost %s, approval ghost %s", ErrInvalidApproval, pl.ProposerIntent.Intent.GhostID, pl.GhostID)
	}
	stepsFor := tr.StepsFor
	if stepsFor == nil {
		stepsFor = CatalogSteps
	}
	if !reflect.DeepEqual(pl.Steps, stepsFor(pl.ProposerIntent.Intent.SeedIDs)) {
		return Result{}, ErrStepsMismatch
	}
	expires := time.UnixMilli(int64(pl.ExpiresAtMS))
	now := tr.Now
	if !now.Before(expires) {
		return Result{}, fmt.Errorf("%w: expired at %s", ErrApprovalExpired, expires.Format(time.RFC3339))
	}
	if expires.Sub(now) > MaxLifetime+ClockSkew {
		return Result{}, fmt.Errorf("%w: lifetime exceeds %s", ErrApprovalExpired, MaxLifetime)
	}
	return Result{Proposer: proposer, Approver: approver}, nil
}
