package main

import (
	"bufio"
	"errors"
	"strings"
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

// Approval payloads are built only for pending proposals whose intent verifies from another
// operator and whose steps equal the local catalog steps; the signed result verifies end to end.
func TestApprovalPayloadChecksProposal(t *testing.T) {
	p := tlstest.NewPKI(t)
	leadCert, leadKey, _ := p.Issue(t, identity.RoleOperator, "lead")
	opsCert, opsKey, _ := p.Issue(t, identity.RoleOperator, "ops")
	ops, err := approval.LoadSigner(opsCert, opsKey)
	if err != nil {
		t.Fatal(err)
	}
	a := &App{}
	if _, _, err := a.operatorSigner(); err == nil {
		t.Fatalf("signing without operator credentials must fail")
	}
	a.mirageCfg.operatorTLSConfig = operatorTLSConfig{OperatorCertFile: leadCert, OperatorKeyFile: leadKey, CAFile: p.CAFile()}
	lead, roots, err := a.operatorSigner()
	if err != nil {
		t.Fatalf("operator signer: %v", err)
	}

	now := time.Now()
	in, err := newIntent("ghost.alpha", []string{"seed.docker"}, now)
	if err != nil {
		t.Fatal(err)
	}
	intent, err := ops.SignIntent(in)
	if err != nil {
		t.Fatal(err)
	}
	proposal := mirage.DependencyProposal{
		ID: "depprop.ghost.alpha.1", GhostID: "ghost.alpha", ProposedBy: "edgectl://operator/ops",
		State: mirage.ProposalPending, Intent: intent, Steps: approval.CatalogSteps([]string{"seed.docker"}),
	}
	payload, err := approvalPayload(proposal, lead, roots, now)
	if err != nil {
		t.Fatalf("payload: %v", err)
	}
	signed, err := lead.Sign(payload)
	if err != nil {
		t.Fatal(err)
	}
	res, err := approval.Verify(signed, approval.Trust{Roots: roots, Now: now})
	if err != nil || res.Approver.String() != "edgectl://operator/lead" || res.Proposer.String() != "edgectl://operator/ops" {
		t.Fatalf("verify: %+v %v", res, err)
	}

	approved := proposal
	approved.State = mirage.ProposalApproved
	if _, err := approvalPayload(approved, lead, roots, now); err == nil {
		t.Errorf("non-pending proposal must not be signed")
	}
	extra := proposal
	extra.Steps = append(append([]seeds.InstallSpec{}, proposal.Steps...), seeds.InstallSpec{SeedID: "seed.docker", Method: seeds.InstallMethodBrew, Package: "netcat"})
	if _, err := approvalPayload(extra, lead, roots, now); !errors.Is(err, approval.ErrStepsMismatch) {
		t.Errorf("steps beyond catalog: expected ErrStepsMismatch, got %v", err)
	}
	relabeled := proposal
	relabeled.ProposedBy = "edgectl://operator/someone"
	if _, err := approvalPayload(relabeled, lead, roots, now); err == nil {
		t.Errorf("proposer label not matching the intent signature must be refused")
	}
	if _, err := approvalPayload(proposal, ops, roots, now); !errors.Is(err, approval.ErrSeparationOfDuties) {
		t.Errorf("proposer approving: expected ErrSeparationOfDuties, got %v", err)
	}
}

// Only an explicit y/yes confirms signing.
func TestPromptConfirm(t *testing.T) {
	for in, want := range map[string]bool{"y\n": true, "YES\n": true, "\n": false, "n\n": false, "sure\n": false} {
		a := &App{reader: bufio.NewReader(strings.NewReader(in))}
		got, err := a.promptConfirm("Sign")
		if err != nil || got != want {
			t.Errorf("input %q: got %v %v, want %v", in, got, err, want)
		}
	}
}
