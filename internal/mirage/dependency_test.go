package mirage

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"github.com/danmuck/edgectl/internal/approval"
	"github.com/danmuck/edgectl/internal/depcatalog"
	"github.com/danmuck/edgectl/internal/identity"
	"net"
	"testing"
	"time"

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

func TestDependencyProposalBuiltFromCatalogWithUnresolved(t *testing.T) {
	testlog.Start(t)
	l := NewDependencyLedger()
	p, err := l.Propose("ghost.alpha", []string{"seed.docker", "seed.mongod"}, "user:ops")
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	if p.State != ProposalPending || len(p.Steps) != 2 {
		t.Fatalf("unexpected proposal: %+v", p)
	}
	if len(p.Unresolved) != 1 || p.Unresolved[0] != "seed.mongod:mongod" {
		t.Fatalf("unresolved = %v, want [seed.mongod:mongod]", p.Unresolved)
	}
	if k := auditKinds(p); len(k) != 1 || k[0] != AuditProposal {
		t.Fatalf("audit = %v", k)
	}

	if _, err := l.Propose("ghost.alpha", []string{"seed.flow"}, "user:ops"); !errors.Is(err, ErrInvalidDependencyProposal) {
		t.Fatalf("seed without catalog sources: expected ErrInvalidDependencyProposal, got %v", err)
	}
}

// Execution requires approval first; states cannot be skipped or replayed.
func TestDependencyProposalStateMachine(t *testing.T) {
	testlog.Start(t)
	l := NewDependencyLedger()
	p, _ := l.Propose("ghost.alpha", []string{"seed.docker"}, "user:ops")

	if _, err := l.BeginExecution(p.ID, "user:ops"); !errors.Is(err, ErrDependencyProposalState) {
		t.Fatalf("execute before approval: expected ErrDependencyProposalState, got %v", err)
	}
	signed := signedFor(t, operatorSigner(t, tlstest.NewPKI(t), "lead"), p)
	if _, err := l.Approve(p.ID, "", signed); !errors.Is(err, ErrInvalidDependencyProposal) {
		t.Fatalf("approval without approver: expected ErrInvalidDependencyProposal, got %v", err)
	}
	if _, err := l.Approve(p.ID, "user:ops", signed); !errors.Is(err, ErrInvalidDependencyProposal) {
		t.Fatalf("proposer approving own proposal: expected ErrInvalidDependencyProposal, got %v", err)
	}
	mismatch := signed
	mismatch.Payload.ProposedBy = "user:someone"
	if _, err := l.Approve(p.ID, "edgectl://operator/lead", mismatch); !errors.Is(err, ErrInvalidDependencyProposal) {
		t.Fatalf("payload not matching proposal: expected ErrInvalidDependencyProposal, got %v", err)
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

func operatorSigner(t *testing.T, p *tlstest.PKI, id string) *approval.Signer {
	t.Helper()
	cert, key, _ := p.Issue(t, identity.RoleOperator, id)
	s, err := approval.LoadSigner(cert, key)
	if err != nil {
		t.Fatalf("load signer: %v", err)
	}
	return s
}

// Operator approval over exactly the proposal content, valid for ten minutes.
func signedFor(t *testing.T, s *approval.Signer, p DependencyProposal) approval.Signed {
	t.Helper()
	nonce, _ := approval.NewNonce()
	signed, err := s.Sign(approval.Payload{
		ProposalID: p.ID, GhostID: p.GhostID, ProposedBy: p.ProposedBy, Steps: p.Steps,
		ExpiresAtMS: uint64(time.Now().Add(10 * time.Minute).UnixMilli()), Nonce: nonce,
	})
	if err != nil {
		t.Fatal(err)
	}
	return signed
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
	l := svc.server.Dependencies()
	p, err := l.Propose("ghost.alpha", []string{"seed.docker"}, "user:ops")
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	if _, err := l.Approve(p.ID, "edgectl://operator/lead", signedFor(t, operatorSigner(t, tlstest.NewPKI(t), "lead"), p)); err != nil {
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

	done, err := svc.ExecuteDependencyProposal(context.Background(), p.ID, "user:ops")
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
	if _, err := svc.ExecuteDependencyProposal(context.Background(), p.ID, "user:ops"); !errors.Is(err, ErrDependencyProposalState) {
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

	done, err := svc.ExecuteDependencyProposal(context.Background(), p.ID, "user:ops")
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
	done, err := svc.ExecuteDependencyProposal(context.Background(), p.ID, "user:ops")
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
	pki := tlstest.NewPKI(t)
	svc.cfg.AdminTLS.CAFile = pki.CAFile()
	lead := operatorSigner(t, pki, "lead")

	resp := svc.handleAdminControlRequest(adminControlRequest{Action: "propose_dependencies", Dependency: AdminDependencyRequest{
		GhostID: "ghost.alpha", SeedIDs: []string{"seed.docker"}, Actor: "user:ops",
	}})
	if !resp.OK {
		t.Fatalf("propose: %s", resp.Error)
	}
	p := resp.Data.(DependencyProposal)

	resp = svc.handleAdminControlRequest(adminControlRequest{Action: "execute_dependencies", Dependency: AdminDependencyRequest{ProposalID: p.ID, Actor: "user:ops"}})
	if resp.OK || resp.ErrorCode != schema.ErrCodeSemanticValidation {
		t.Fatalf("execute before approve: ok=%v code=%d", resp.OK, resp.ErrorCode)
	}
	// Approval needs a signature; an unsigned or wrongly-attributed approval is refused (1300).
	resp = svc.handleAdminControlRequest(adminControlRequest{Action: "approve_dependencies", Dependency: AdminDependencyRequest{ProposalID: p.ID, Actor: "edgectl://operator/lead"}})
	if resp.OK || resp.ErrorCode != schema.ErrCodeSemanticValidation {
		t.Fatalf("unsigned approval: ok=%v code=%d %s", resp.OK, resp.ErrorCode, resp.Error)
	}
	signed := signedFor(t, lead, p)
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
		ID: "depprop.ghost.alpha.9", GhostID: "ghost.alpha", ProposedBy: "user:ops", ApprovedBy: "edgectl://operator/lead",
		Steps: []seeds.InstallSpec{depcatalog.CatalogFor("seed.docker")[0].Spec},
	}
	signed := signedFor(t, operatorSigner(t, tlstest.NewPKI(t), "lead"), proposal)
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
