package main

import (
	"crypto/x509"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/danmuck/edgectl/internal/approval"
	"github.com/danmuck/edgectl/internal/identity"
	"github.com/danmuck/edgectl/internal/mirage"
	"github.com/danmuck/edgectl/internal/seeds"
	"github.com/danmuck/edgectl/internal/testutil/tlstest"
)

// Operator credentials must carry an operator principal; dial configs pin the target role/id.
func TestOperatorDialConfig(t *testing.T) {
	p := tlstest.NewPKI(t)
	opCert, opKey, _ := p.Issue(t, identity.RoleOperator, "dan")
	ghCert, ghKey, _ := p.Issue(t, identity.RoleGhost, "ghost.alpha")

	a := &App{}
	if cfg, err := a.operatorDialConfig(identity.RoleGhost, "ghost.alpha", false); cfg != nil || err != nil {
		t.Fatalf("no operator cert: want plaintext (nil, nil), got %v %v", cfg, err)
	}

	a.ghostCfg.operatorTLSConfig = operatorTLSConfig{OperatorCertFile: opCert, OperatorKeyFile: opKey, CAFile: p.CAFile()}
	if cfg, err := a.operatorDialConfig(identity.RoleGhost, "ghost.alpha", false); cfg == nil || err != nil {
		t.Fatalf("operator cert: %v %v", cfg, err)
	}

	a.ghostCfg.operatorTLSConfig = operatorTLSConfig{OperatorCertFile: ghCert, OperatorKeyFile: ghKey, CAFile: p.CAFile()}
	if _, err := a.operatorDialConfig(identity.RoleGhost, "ghost.alpha", false); !errors.Is(err, identity.ErrWrongRole) {
		t.Fatalf("ghost cert as operator: expected ErrWrongRole, got %v", err)
	}
}

// client-tm signs exactly the proposal content with the operator key; the result verifies.
func TestSignProposalVerifies(t *testing.T) {
	p := tlstest.NewPKI(t)
	opCert, opKey, _ := p.Issue(t, identity.RoleOperator, "lead")
	a := &App{}
	proposal := mirage.DependencyProposal{
		ID: "depprop.ghost.alpha.1", GhostID: "ghost.alpha", ProposedBy: "edgectl://operator/ops",
		Steps: []seeds.InstallSpec{{SeedID: "seed.docker", Method: seeds.InstallMethodBrew, Package: "docker"}},
	}
	if _, err := a.signProposal(proposal, time.Now()); err == nil {
		t.Fatalf("signing without operator credentials must fail")
	}
	a.mirageCfg.operatorTLSConfig = operatorTLSConfig{OperatorCertFile: opCert, OperatorKeyFile: opKey, CAFile: p.CAFile()}
	signed, err := a.signProposal(proposal, time.Now())
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	raw, _ := os.ReadFile(p.CAFile())
	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM(raw)
	approver, err := approval.Verify(signed, roots, time.Now())
	if err != nil || approver.String() != "edgectl://operator/lead" {
		t.Fatalf("verify: %v %v", approver, err)
	}
	if signed.Payload.ProposalID != proposal.ID || signed.Payload.GhostID != proposal.GhostID || len(signed.Payload.Steps) != 1 {
		t.Fatalf("payload does not bind the proposal: %+v", signed.Payload)
	}
}
