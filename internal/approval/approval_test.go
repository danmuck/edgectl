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

func payload(now time.Time) Payload {
	return Payload{
		ProposalID:  "depprop.ghost.alpha.1",
		GhostID:     "ghost.alpha",
		ProposedBy:  "edgectl://operator/ops",
		Steps:       []seeds.InstallSpec{{SeedID: "seed.docker", Method: seeds.InstallMethodBrew, Package: "docker", Binary: "docker", InstallToBin: true}},
		ExpiresAtMS: uint64(now.Add(10 * time.Minute).UnixMilli()),
		Nonce:       "n-1",
	}
}

func operatorSigner(t *testing.T, p *tlstest.PKI, id string) *Signer {
	t.Helper()
	cert, key, _ := p.Issue(t, identity.RoleOperator, id)
	s, err := LoadSigner(cert, key)
	if err != nil {
		t.Fatalf("load signer: %v", err)
	}
	return s
}

func TestSignVerifyRoundTrip(t *testing.T) {
	p := tlstest.NewPKI(t)
	now := time.Now()
	signed, err := operatorSigner(t, p, "lead").Sign(payload(now))
	if err != nil {
		t.Fatal(err)
	}
	approver, err := Verify(signed, rootsFor(t, p), now)
	if err != nil || approver.String() != "edgectl://operator/lead" {
		t.Fatalf("verify: %v %v", approver, err)
	}
}

func TestVerifyRejections(t *testing.T) {
	p := tlstest.NewPKI(t)
	now := time.Now()
	roots := rootsFor(t, p)
	signer := operatorSigner(t, p, "lead")

	tampered, _ := signer.Sign(payload(now))
	tampered.Payload.Steps[0].Package = "evil"
	if _, err := Verify(tampered, roots, now); !errors.Is(err, ErrInvalidApproval) {
		t.Errorf("tampered steps: expected ErrInvalidApproval, got %v", err)
	}

	foreign := tlstest.NewPKI(t)
	fsigned, _ := operatorSigner(t, foreign, "lead").Sign(payload(now))
	if _, err := Verify(fsigned, roots, now); !errors.Is(err, ErrInvalidApproval) {
		t.Errorf("foreign root: expected ErrInvalidApproval, got %v", err)
	}

	expired, _ := signer.Sign(payload(now.Add(-20 * time.Minute)))
	if _, err := Verify(expired, roots, now); !errors.Is(err, ErrApprovalExpired) {
		t.Errorf("expired: expected ErrApprovalExpired, got %v", err)
	}
	long := payload(now)
	long.ExpiresAtMS = uint64(now.Add(time.Hour).UnixMilli())
	longSigned, _ := signer.Sign(long)
	if _, err := Verify(longSigned, roots, now); !errors.Is(err, ErrApprovalExpired) {
		t.Errorf("lifetime over max: expected ErrApprovalExpired, got %v", err)
	}

	self := payload(now)
	self.ProposedBy = "edgectl://operator/lead"
	selfSigned, _ := signer.Sign(self)
	if _, err := Verify(selfSigned, roots, now); !errors.Is(err, ErrSeparationOfDuties) {
		t.Errorf("self approval: expected ErrSeparationOfDuties, got %v", err)
	}

	empty := payload(now)
	empty.Nonce = ""
	emptySigned, _ := signer.Sign(empty)
	if _, err := Verify(emptySigned, roots, now); !errors.Is(err, ErrInvalidApproval) {
		t.Errorf("missing nonce: expected ErrInvalidApproval, got %v", err)
	}
}

// Only operators can sign, and a non-operator certificate fails verification even if crafted.
func TestNonOperatorCannotSign(t *testing.T) {
	p := tlstest.NewPKI(t)
	cert, key, _ := p.Issue(t, identity.RoleMirage, "mirage.main")
	if _, err := LoadSigner(cert, key); !errors.Is(err, identity.ErrWrongRole) {
		t.Fatalf("mirage signer: expected ErrWrongRole, got %v", err)
	}
	op := operatorSigner(t, p, "lead")
	signed, _ := op.Sign(payload(time.Now()))
	mirageDER := mustCertDER(t, cert)
	signed.ApproverCert = mirageDER
	if _, err := Verify(signed, rootsFor(t, p), time.Now()); !errors.Is(err, ErrInvalidApproval) {
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
