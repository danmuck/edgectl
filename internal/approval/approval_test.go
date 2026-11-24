package approval

import (
	"crypto/x509"
	"encoding/pem"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/danmuck/edgectl/internal/identity"
	"github.com/danmuck/edgectl/internal/seeds"
	"github.com/danmuck/edgectl/internal/testutil/tlstest"
)

var testSteps = []seeds.InstallSpec{{SeedID: "seed.docker", Method: seeds.InstallMethodBrew, Package: "docker", Binary: "docker", InstallToBin: true}}

// Fixed step derivation so tests do not depend on the live catalog contents.
func fixedSteps(seedIDs []string) []seeds.InstallSpec {
	if len(seedIDs) == 1 && seedIDs[0] == "seed.docker" {
		return append([]seeds.InstallSpec{}, testSteps...)
	}
	return nil
}

func rootsFor(t *testing.T, p *tlstest.PKI) *x509.CertPool {
	t.Helper()
	raw, err := os.ReadFile(p.CAFile())
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(raw)
	return pool
}

func trustFor(t *testing.T, p *tlstest.PKI, now time.Time) Trust {
	return Trust{Roots: rootsFor(t, p), Now: now, StepsFor: fixedSteps}
}

func intent(now time.Time) Intent {
	return Intent{GhostID: "ghost.alpha", SeedIDs: []string{"seed.docker"}, ProposedAtMS: uint64(now.UnixMilli()), Nonce: "i-1"}
}

func payload(t *testing.T, proposer *Signer, now time.Time) Payload {
	t.Helper()
	si, err := proposer.SignIntent(intent(now))
	if err != nil {
		t.Fatal(err)
	}
	return Payload{
		ProposalID:     "depprop.ghost.alpha.1",
		GhostID:        "ghost.alpha",
		ProposerIntent: si,
		Steps:          append([]seeds.InstallSpec{}, testSteps...),
		ExpiresAtMS:    uint64(now.Add(10 * time.Minute).UnixMilli()),
		Nonce:          "n-1",
	}
}

type signerSet struct {
	ops, lead *Signer
	opsSerial string
}

// Issues each operator once; the test PKI refuses to overwrite issued files.
func signers(t *testing.T, p *tlstest.PKI) signerSet {
	t.Helper()
	opsCert, opsKey, opsSerial := p.Issue(t, identity.RoleOperator, "ops")
	ops, err := LoadSigner(opsCert, opsKey)
	if err != nil {
		t.Fatal(err)
	}
	leadCert, leadKey, _ := p.Issue(t, identity.RoleOperator, "lead")
	lead, err := LoadSigner(leadCert, leadKey)
	if err != nil {
		t.Fatal(err)
	}
	return signerSet{ops: ops, lead: lead, opsSerial: opsSerial}
}

func TestSignVerifyRoundTrip(t *testing.T) {
	p := tlstest.NewPKI(t)
	now := time.Now()
	s := signers(t, p)
	signed, err := s.lead.Sign(payload(t, s.ops, now))
	if err != nil {
		t.Fatal(err)
	}
	res, err := Verify(signed, trustFor(t, p, now))
	if err != nil || res.Approver.String() != "edgectl://operator/lead" || res.Proposer.String() != "edgectl://operator/ops" {
		t.Fatalf("verify: %+v %v", res, err)
	}
}

func TestVerifyRejections(t *testing.T) {
	p := tlstest.NewPKI(t)
	now := time.Now()
	tr := trustFor(t, p, now)
	s := signers(t, p)

	tampered, _ := s.lead.Sign(payload(t, s.ops, now))
	tampered.Payload.Steps[0].Package = "evil"
	if _, err := Verify(tampered, tr); !errors.Is(err, ErrInvalidApproval) {
		t.Errorf("tampered steps: expected ErrInvalidApproval, got %v", err)
	}

	// A signed payload whose steps differ from the catalog for the intent seeds.
	extra := payload(t, s.ops, now)
	extra.Steps = append(extra.Steps, seeds.InstallSpec{SeedID: "seed.docker", Method: seeds.InstallMethodBrew, Package: "netcat"})
	extraSigned, _ := s.lead.Sign(extra)
	if _, err := Verify(extraSigned, tr); !errors.Is(err, ErrStepsMismatch) {
		t.Errorf("steps beyond catalog: expected ErrStepsMismatch, got %v", err)
	}

	// Approver swaps the intent's seeds: the proposer signature no longer verifies.
	swapped := payload(t, s.ops, now)
	swapped.ProposerIntent.Intent.SeedIDs = []string{"seed.other"}
	swappedSigned, _ := s.lead.Sign(swapped)
	if _, err := Verify(swappedSigned, tr); !errors.Is(err, ErrInvalidApproval) {
		t.Errorf("altered intent: expected ErrInvalidApproval, got %v", err)
	}

	otherGhost := payload(t, s.ops, now)
	otherGhost.GhostID = "ghost.beta"
	otherSigned, _ := s.lead.Sign(otherGhost)
	if _, err := Verify(otherSigned, tr); !errors.Is(err, ErrInvalidApproval) {
		t.Errorf("ghost differs from intent: expected ErrInvalidApproval, got %v", err)
	}

	foreign := tlstest.NewPKI(t)
	fs := signers(t, foreign)
	fsigned, _ := fs.lead.Sign(payload(t, fs.ops, now))
	if _, err := Verify(fsigned, tr); !errors.Is(err, ErrInvalidApproval) {
		t.Errorf("foreign root: expected ErrInvalidApproval, got %v", err)
	}

	expired := payload(t, s.ops, now)
	expired.ExpiresAtMS = uint64(now.Add(-time.Minute).UnixMilli())
	expiredSigned, _ := s.lead.Sign(expired)
	if _, err := Verify(expiredSigned, tr); !errors.Is(err, ErrApprovalExpired) {
		t.Errorf("expired: expected ErrApprovalExpired, got %v", err)
	}
	long := payload(t, s.ops, now)
	long.ExpiresAtMS = uint64(now.Add(time.Hour).UnixMilli())
	longSigned, _ := s.lead.Sign(long)
	if _, err := Verify(longSigned, tr); !errors.Is(err, ErrApprovalExpired) {
		t.Errorf("lifetime over max: expected ErrApprovalExpired, got %v", err)
	}

	stale := payload(t, s.ops, now.Add(-25*time.Hour))
	stale.ExpiresAtMS = uint64(now.Add(10 * time.Minute).UnixMilli())
	staleSigned, _ := s.lead.Sign(stale)
	if _, err := Verify(staleSigned, tr); !errors.Is(err, ErrApprovalExpired) {
		t.Errorf("stale intent: expected ErrApprovalExpired, got %v", err)
	}

	// One operator proposes and approves: separation is decided by the two signatures, not a field.
	selfSigned, _ := s.ops.Sign(payload(t, s.ops, now))
	if _, err := Verify(selfSigned, tr); !errors.Is(err, ErrSeparationOfDuties) {
		t.Errorf("self approval: expected ErrSeparationOfDuties, got %v", err)
	}

	empty := payload(t, s.ops, now)
	empty.Nonce = ""
	emptySigned, _ := s.lead.Sign(empty)
	if _, err := Verify(emptySigned, tr); !errors.Is(err, ErrInvalidApproval) {
		t.Errorf("missing nonce: expected ErrInvalidApproval, got %v", err)
	}
}

// A denied serial revokes that operator's signatures, as proposer or approver.
func TestVerifyRejectsDeniedSigner(t *testing.T) {
	p := tlstest.NewPKI(t)
	now := time.Now()
	s := signers(t, p)
	denied, err := identity.ParseDeniedSerials([]string{s.opsSerial})
	if err != nil {
		t.Fatal(err)
	}
	tr := trustFor(t, p, now)
	tr.Denied = denied

	asProposer, _ := s.lead.Sign(payload(t, s.ops, now))
	if _, err := Verify(asProposer, tr); !errors.Is(err, ErrInvalidApproval) {
		t.Errorf("denied proposer: expected ErrInvalidApproval, got %v", err)
	}
	if _, err := VerifyIntent(asProposer.Payload.ProposerIntent, tr); !errors.Is(err, ErrInvalidApproval) {
		t.Errorf("denied intent signer: expected ErrInvalidApproval, got %v", err)
	}
	asApprover, _ := s.ops.Sign(payload(t, s.lead, now))
	if _, err := Verify(asApprover, tr); !errors.Is(err, ErrInvalidApproval) {
		t.Errorf("denied approver: expected ErrInvalidApproval, got %v", err)
	}
}

// Only operators can sign, and a non-operator certificate fails verification even if crafted.
func TestNonOperatorCannotSign(t *testing.T) {
	p := tlstest.NewPKI(t)
	cert, key, _ := p.Issue(t, identity.RoleMirage, "mirage.main")
	if _, err := LoadSigner(cert, key); !errors.Is(err, identity.ErrWrongRole) {
		t.Fatalf("mirage signer: expected ErrWrongRole, got %v", err)
	}
	s := signers(t, p)
	now := time.Now()
	signed, _ := s.lead.Sign(payload(t, s.ops, now))
	signed.ApproverCert = mustCertDER(t, cert)
	if _, err := Verify(signed, trustFor(t, p, now)); !errors.Is(err, ErrInvalidApproval) {
		t.Fatalf("mirage certificate as approver: expected ErrInvalidApproval, got %v", err)
	}
}

func mustCertDER(t *testing.T, certFile string) []byte {
	t.Helper()
	raw, err := os.ReadFile(certFile)
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(raw)
	return block.Bytes
}
