package mirage

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/danmuck/edgectl/internal/approval"
	"github.com/danmuck/edgectl/internal/depcatalog"
	"github.com/danmuck/edgectl/internal/identity"
	"github.com/danmuck/edgectl/internal/protocol/schema"
	"github.com/danmuck/edgectl/internal/seeds"
	"github.com/danmuck/edgectl/internal/testutil/testlog"
	"github.com/danmuck/edgectl/internal/testutil/tlstest"
)

// Every catalog entry must satisfy install policy and be recognized by depcatalog.Contains.
func TestDependencyCatalogEntriesPassPolicy(t *testing.T) {
	testlog.Start(t)
	entries := depcatalog.DependencyCatalog()
	if len(entries) == 0 {
		t.Fatalf("catalog is empty")
	}
	for _, e := range entries {
		if err := seeds.ValidateSpec(e.Spec); err != nil {
			t.Errorf("%s/%s violates policy: %v", e.SeedID, e.Name, err)
		}
		if !depcatalog.Contains(e.Spec) {
			t.Errorf("%s/%s not recognized by Contains", e.SeedID, e.Name)
		}
		if e.Spec.SeedID != e.SeedID {
			t.Errorf("%s/%s spec seed_id %q mismatch", e.SeedID, e.Name, e.Spec.SeedID)
		}
	}
}

func auditKinds(p DependencyProposal) []DependencyAuditKind {
	out := make([]DependencyAuditKind, 0, len(p.Audit))
	for _, a := range p.Audit {
		out = append(out, a.Kind)
	}
	return out
}

// Proposing (ops) and approving (lead) operators under one test root.
type operators struct {
	pki       *tlstest.PKI
	ops, lead *approval.Signer
	opsSerial string
}

func newOperators(t *testing.T) operators {
	t.Helper()
	p := tlstest.NewPKI(t)
	ops, opsSerial := operatorSigner(t, p, "ops")
	lead, _ := operatorSigner(t, p, "lead")
	return operators{pki: p, ops: ops, lead: lead, opsSerial: opsSerial}
}

func operatorSigner(t *testing.T, p *tlstest.PKI, id string) (*approval.Signer, string) {
	t.Helper()
	cert, key, serial := p.Issue(t, identity.RoleOperator, id)
	s, err := approval.LoadSigner(cert, key)
	if err != nil {
		t.Fatalf("load signer: %v", err)
	}
	return s, serial
}

// Proposer-signed intent for ghostID and seedIDs, proposed now.
func intentFor(t *testing.T, s *approval.Signer, ghostID string, seedIDs ...string) approval.SignedIntent {
	t.Helper()
	nonce, _ := approval.NewNonce()
	si, err := s.SignIntent(approval.Intent{GhostID: ghostID, SeedIDs: seedIDs, ProposedAtMS: uint64(time.Now().UnixMilli()), Nonce: nonce})
	if err != nil {
		t.Fatal(err)
	}
	return si
}

// Operator approval over exactly the proposal content and its intent, valid for ten minutes.
func signedFor(t *testing.T, s *approval.Signer, p DependencyProposal) approval.Signed {
	t.Helper()
	nonce, _ := approval.NewNonce()
	signed, err := s.Sign(approval.Payload{
		ProposalID: p.ID, GhostID: p.GhostID, ProposerIntent: p.Intent, Steps: p.Steps,
		ExpiresAtMS: uint64(time.Now().Add(10 * time.Minute).UnixMilli()), Nonce: nonce,
	})
	if err != nil {
		t.Fatal(err)
	}
	return signed
}

func TestDependencyProposalBuiltFromCatalogWithUnresolved(t *testing.T) {
	testlog.Start(t)
	op := newOperators(t)
	l := NewDependencyLedger()
	intent := intentFor(t, op.ops, "ghost.alpha", "seed.docker", "seed.mongod")
	p, err := l.Propose(intent, "edgectl://operator/ops")
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	if p.State != ProposalPending || len(p.Steps) != 2 || p.GhostID != "ghost.alpha" || p.ProposedBy != "edgectl://operator/ops" {
		t.Fatalf("unexpected proposal: %+v", p)
	}
	if len(p.Unresolved) != 1 || p.Unresolved[0] != "seed.mongod:mongod" {
		t.Fatalf("unresolved = %v, want [seed.mongod:mongod]", p.Unresolved)
	}
	if k := auditKinds(p); len(k) != 1 || k[0] != AuditProposal {
		t.Fatalf("audit = %v", k)
	}
	if _, err := l.Propose(intent, "edgectl://operator/ops"); !errors.Is(err, ErrInvalidDependencyProposal) {
		t.Fatalf("reused intent: expected ErrInvalidDependencyProposal, got %v", err)
	}
	if _, err := l.Propose(intentFor(t, op.ops, "ghost.alpha", "seed.flow"), "edgectl://operator/ops"); !errors.Is(err, ErrInvalidDependencyProposal) {
		t.Fatalf("seed without catalog sources: expected ErrInvalidDependencyProposal, got %v", err)
	}
}

// Execution requires approval first; states cannot be skipped or replayed.
func TestDependencyProposalStateMachine(t *testing.T) {
	testlog.Start(t)
	op := newOperators(t)
	l := NewDependencyLedger()
	p, _ := l.Propose(intentFor(t, op.ops, "ghost.alpha", "seed.docker"), "edgectl://operator/ops")

	if _, err := l.BeginExecution(p.ID, "edgectl://operator/ops"); !errors.Is(err, ErrDependencyProposalState) {
		t.Fatalf("execute before approval: expected ErrDependencyProposalState, got %v", err)
	}
	signed := signedFor(t, op.lead, p)
	if _, err := l.Approve(p.ID, "", signed); !errors.Is(err, ErrInvalidDependencyProposal) {
		t.Fatalf("approval without approver: expected ErrInvalidDependencyProposal, got %v", err)
	}
	if _, err := l.Approve(p.ID, "edgectl://operator/ops", signed); !errors.Is(err, ErrInvalidDependencyProposal) {
		t.Fatalf("proposer approving own proposal: expected ErrInvalidDependencyProposal, got %v", err)
	}
	// Approval embedding a different (validly signed) intent than the stored proposal.
	mismatch := signed
	mismatch.Payload.ProposerIntent = intentFor(t, op.ops, "ghost.alpha", "seed.docker")
	if _, err := l.Approve(p.ID, "edgectl://operator/lead", mismatch); !errors.Is(err, ErrInvalidDependencyProposal) {
		t.Fatalf("payload intent not matching proposal: expected ErrInvalidDependencyProposal, got %v", err)
	}
	approved, err := l.Approve(p.ID, "edgectl://operator/lead", signed)
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if approved.Approval == nil || approved.ApprovedBy != "edgectl://operator/lead" {
		t.Fatalf("approval not recorded: %+v", approved)
	}
	if _, err := l.Approve(p.ID, "edgectl://operator/lead", signed); !errors.Is(err, ErrDependencyProposalState) {
		t.Fatalf("double approval: expected ErrDependencyProposalState, got %v", err)
	}
	if _, err := l.Approve("depprop.none", "edgectl://operator/lead", signed); !errors.Is(err, ErrDependencyProposalNotFound) {
		t.Fatalf("unknown id: expected ErrDependencyProposalNotFound, got %v", err)
	}
}

// Mirage service with a ghost admin route and a scripted plan dispatcher.
func newDependencyService(t *testing.T, install dependencyInstaller) *Service {
	t.Helper()
	svc := NewServiceWithConfig(ServiceConfig{ListenAddr: "127.0.0.1:0"})
	svc.bindGhostAdmin("ghost.alpha", "127.0.0.1:7999")
	svc.dependencyInstaller = install
	return svc
}

func proposeAndApprove(t *testing.T, svc *Service) DependencyProposal {
	t.Helper()
	op := newOperators(t)
	l := svc.server.Dependencies()
	p, err := l.Propose(intentFor(t, op.ops, "ghost.alpha", "seed.docker"), "edgectl://operator/ops")
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	if _, err := l.Approve(p.ID, "edgectl://operator/lead", signedFor(t, op.lead, p)); err != nil {
		t.Fatalf("approve: %v", err)
	}
	return p
}

func TestExecuteDependencyProposalSuccessAudits(t *testing.T) {
	testlog.Start(t)
	var sent DependencyProposal
	svc := newDependencyService(t, func(_ context.Context, _ string, p DependencyProposal) (DependencyExecution, error) {
		sent = p
		return DependencyExecution{Succeeded: true, Steps: []DependencyStepResult{{SeedID: "seed.docker", Method: "brew", Status: "installed"}}}, nil
	})
	p := proposeAndApprove(t, svc)

	done, err := svc.ExecuteDependencyProposal(context.Background(), p.ID, "edgectl://operator/ops")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if sent.ApprovedBy != "edgectl://operator/lead" || sent.Approval == nil || len(sent.Steps) != 1 {
		t.Fatalf("dispatched plan missing approval or steps: %+v", sent)
	}
	if done.State != ProposalSucceeded {
		t.Fatalf("state = %s, want succeeded", done.State)
	}
	want := []DependencyAuditKind{AuditProposal, AuditApproval, AuditExecutionStart, AuditExecutionResult}
	got := auditKinds(done)
	if len(got) != len(want) {
		t.Fatalf("audit = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("audit = %v, want %v", got, want)
		}
	}
	if _, err := svc.ExecuteDependencyProposal(context.Background(), p.ID, "edgectl://operator/ops"); !errors.Is(err, ErrDependencyProposalState) {
		t.Fatalf("re-execution: expected ErrDependencyProposalState, got %v", err)
	}
}

func TestExecuteDependencyProposalFailureAuditsRollback(t *testing.T) {
	testlog.Start(t)
	svc := newDependencyService(t, func(context.Context, string, DependencyProposal) (DependencyExecution, error) {
		return DependencyExecution{
			Succeeded: false,
			Steps:     []DependencyStepResult{{SeedID: "seed.docker", Method: "curl", Status: "installed"}, {SeedID: "seed.docker", Method: "curl", Status: "failed", Error: "checksum"}},
			Rollback:  []DependencyRollbackResult{{SeedID: "seed.docker", Method: "curl", RolledBack: true}},
		}, nil
	})
	p := proposeAndApprove(t, svc)

	done, err := svc.ExecuteDependencyProposal(context.Background(), p.ID, "edgectl://operator/ops")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if done.State != ProposalFailed {
		t.Fatalf("state = %s, want failed", done.State)
	}
	last := done.Audit[len(done.Audit)-1]
	if last.Kind != AuditRollback || last.Detail != "rolled_back 1/1" {
		t.Fatalf("last audit = %+v, want rollback rolled_back 1/1", last)
	}
}

func TestExecuteDependencyProposalWithoutRouteFails(t *testing.T) {
	testlog.Start(t)
	svc := NewServiceWithConfig(ServiceConfig{ListenAddr: "127.0.0.1:0"})
	svc.dependencyInstaller = func(context.Context, string, DependencyProposal) (DependencyExecution, error) {
		t.Fatalf("must not dispatch without a ghost admin route")
		return DependencyExecution{}, nil
	}
	p := proposeAndApprove(t, svc)
	done, err := svc.ExecuteDependencyProposal(context.Background(), p.ID, "edgectl://operator/ops")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if done.State != ProposalFailed || done.Execution == nil || done.Execution.Error == "" {
		t.Fatalf("expected failed execution with error, got %+v", done)
	}
}

func TestDependencyAdminActionsAndCodes(t *testing.T) {
	testlog.Start(t)
	var dispatched DependencyProposal
	svc := newDependencyService(t, func(_ context.Context, _ string, p DependencyProposal) (DependencyExecution, error) {
		dispatched = p
		return DependencyExecution{Succeeded: true}, nil
	})
	op := newOperators(t)
	svc.cfg.AdminTLS.CAFile = op.pki.CAFile()
	propose := func(actor string, intent *approval.SignedIntent) adminControlResponse {
		return svc.handleAdminControlRequest(adminControlRequest{Action: "propose_dependencies", Dependency: AdminDependencyRequest{Actor: actor, Intent: intent}})
	}

	// Proposals need a proposer-signed intent from the calling operator (1300 otherwise).
	if resp := propose("edgectl://operator/ops", nil); resp.OK || resp.ErrorCode != schema.ErrCodeSemanticValidation {
		t.Fatalf("unsigned proposal: ok=%v code=%d %s", resp.OK, resp.ErrorCode, resp.Error)
	}
	intent := intentFor(t, op.ops, "ghost.alpha", "seed.docker")
	if resp := propose("edgectl://operator/lead", &intent); resp.OK || resp.ErrorCode != schema.ErrCodeSemanticValidation {
		t.Fatalf("intent submitted by a different operator: ok=%v code=%d %s", resp.OK, resp.ErrorCode, resp.Error)
	}
	resp := propose("edgectl://operator/ops", &intent)
	if !resp.OK {
		t.Fatalf("propose: %s", resp.Error)
	}
	p := resp.Data.(DependencyProposal)
	if p.ProposedBy != "edgectl://operator/ops" {
		t.Fatalf("proposed_by = %q, want signer principal", p.ProposedBy)
	}

	resp = svc.handleAdminControlRequest(adminControlRequest{Action: "execute_dependencies", Dependency: AdminDependencyRequest{ProposalID: p.ID, Actor: "edgectl://operator/ops"}})
	if resp.OK || resp.ErrorCode != schema.ErrCodeSemanticValidation {
		t.Fatalf("execute before approve: ok=%v code=%d", resp.OK, resp.ErrorCode)
	}
	// The proposer cannot approve their own proposal, even with a valid signature.
	selfSigned := signedFor(t, op.ops, p)
	resp = svc.handleAdminControlRequest(adminControlRequest{Action: "approve_dependencies", Dependency: AdminDependencyRequest{ProposalID: p.ID, Actor: "edgectl://operator/ops", Approval: &selfSigned}})
	if resp.OK || resp.ErrorCode != schema.ErrCodeSemanticValidation {
		t.Fatalf("self approval: ok=%v code=%d %s", resp.OK, resp.ErrorCode, resp.Error)
	}
	// Approval needs a signature; an unsigned or wrongly-attributed approval is refused (1300).
	resp = svc.handleAdminControlRequest(adminControlRequest{Action: "approve_dependencies", Dependency: AdminDependencyRequest{ProposalID: p.ID, Actor: "edgectl://operator/lead"}})
	if resp.OK || resp.ErrorCode != schema.ErrCodeSemanticValidation {
		t.Fatalf("unsigned approval: ok=%v code=%d %s", resp.OK, resp.ErrorCode, resp.Error)
	}
	signed := signedFor(t, op.lead, p)
	resp = svc.handleAdminControlRequest(adminControlRequest{Action: "approve_dependencies", Dependency: AdminDependencyRequest{ProposalID: p.ID, Actor: "edgectl://operator/other", Approval: &signed}})
	if resp.OK || resp.ErrorCode != schema.ErrCodeSemanticValidation {
		t.Fatalf("approval submitted by a different operator: ok=%v code=%d %s", resp.OK, resp.ErrorCode, resp.Error)
	}
	resp = svc.handleAdminControlRequest(adminControlRequest{Action: "approve_dependencies", Dependency: AdminDependencyRequest{ProposalID: p.ID, Actor: "edgectl://operator/lead", Approval: &signed}})
	if !resp.OK {
		t.Fatalf("approve: %s", resp.Error)
	}
	resp = svc.handleAdminControlRequest(adminControlRequest{Action: "execute_dependencies", Dependency: AdminDependencyRequest{ProposalID: p.ID, Actor: "edgectl://operator/lead"}})
	if !resp.OK {
		t.Fatalf("execute: %s", resp.Error)
	}
	if dispatched.Approval == nil || dispatched.Approval.SignatureDigest() != signed.SignatureDigest() {
		t.Fatalf("signed approval was not forwarded to the ghost")
	}
	resp = svc.handleAdminControlRequest(adminControlRequest{Action: "list_dependency_proposals", Dependency: AdminDependencyRequest{ProposalID: p.ID}})
	list, _ := resp.Data.([]DependencyProposal)
	if !resp.OK || len(list) != 1 || list[0].State != ProposalSucceeded {
		t.Fatalf("list: ok=%v data=%+v", resp.OK, resp.Data)
	}

	// A denied proposer certificate cannot open new proposals.
	svc.cfg.AdminTLS.DeniedSerials = []string{op.opsSerial}
	fresh := intentFor(t, op.ops, "ghost.alpha", "seed.docker")
	if resp := propose("edgectl://operator/ops", &fresh); resp.OK || resp.ErrorCode != schema.ErrCodeSemanticValidation {
		t.Fatalf("denied proposer: ok=%v code=%d %s", resp.OK, resp.ErrorCode, resp.Error)
	}
}

// The real Ghost admin client sends the approved plan (with approver and auth) to install_dependencies.
func TestGhostControlClientInstallDependencies(t *testing.T) {
	testlog.Start(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	got := make(chan ghostControlRequest, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		line, _ := bufio.NewReader(conn).ReadBytes('\n')
		var req ghostControlRequest
		_ = json.Unmarshal(line, &req)
		got <- req
		data, _ := json.Marshal(ghostInstallPlanResult{ProposalID: req.Install.ProposalID, Succeeded: true,
			Steps: []DependencyStepResult{{SeedID: "seed.docker", Method: "brew", Status: "installed"}}})
		resp, _ := json.Marshal(ghostControlResponse{OK: true, Data: data})
		_, _ = conn.Write(append(resp, '\n'))
	}()

	client := NewGhostControlClient(ln.Addr().String())
	proposal := DependencyProposal{
		ID: "depprop.ghost.alpha.9", GhostID: "ghost.alpha", ProposedBy: "edgectl://operator/ops", ApprovedBy: "edgectl://operator/lead",
		Steps: []seeds.InstallSpec{depcatalog.CatalogFor("seed.docker")[0].Spec},
	}
	op := newOperators(t)
	proposal.Intent = intentFor(t, op.ops, "ghost.alpha", "seed.docker")
	signed := signedFor(t, op.lead, proposal)
	proposal.Approval = &signed
	exec, err := client.InstallDependencies(context.Background(), proposal)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	req := <-got
	if req.Action != "install_dependencies" || req.Install == nil || req.Install.ApprovedBy != "edgectl://operator/lead" ||
		req.Install.Approval == nil || req.Install.Approval.SignatureDigest() != signed.SignatureDigest() || len(req.Install.Specs) != 1 {
		t.Fatalf("unexpected request: %+v", req)
	}
	if !exec.Succeeded || len(exec.Steps) != 1 {
		t.Fatalf("unexpected execution: %+v", exec)
	}
}
