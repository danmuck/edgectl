package ghost

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/danmuck/edgectl/internal/adminauth"
	"github.com/danmuck/edgectl/internal/approval"
	"github.com/danmuck/edgectl/internal/identity"
	"github.com/danmuck/edgectl/internal/protocol/schema"
	"github.com/danmuck/edgectl/internal/seeds"
	"github.com/danmuck/edgectl/internal/testutil/testlog"
	"github.com/danmuck/edgectl/internal/testutil/tlstest"
)

// installHarness is a Ghost that accepts install plans, a proposing operator (ops), an approving
// operator (lead), and a test catalog mapping seed ids to steps.
type installHarness struct {
	svc         *Service
	workspace   string
	pki         *tlstest.PKI
	proposer    *approval.Signer
	signer      *approval.Signer
	leadSerial  string
	testCatalog map[string][]seeds.InstallSpec
}

// Ghost with installs enabled, its admin CA set to a temp root, and installer HTTP pointed at server.
func newInstallGhost(t *testing.T, server *httptest.Server) *installHarness {
	t.Helper()
	p := tlstest.NewPKI(t)
	workspace := t.TempDir()
	cfg := DefaultServiceConfig()
	cfg.AdminTLS = adminauth.Credentials{CAFile: p.CAFile()}
	cfg.SeedInstall = SeedInstallConfig{
		Enabled:       true,
		WorkspaceRoot: workspace,
		InstallRoot:   filepath.Join("local", "seeds"),
		BinRoot:       filepath.Join("local", "bin"),
		Whitelist:     []string{"seed.tool"},
	}
	svc := NewServiceWithConfig(cfg)
	if server != nil {
		svc.installerConfigHook = func(c *seeds.InstallerConfig) { c.HTTPClient = server.Client() }
	}
	lead, leadSerial := operatorSigner(t, p, "lead")
	ops, _ := operatorSigner(t, p, "ops")
	h := &installHarness{svc: svc, workspace: workspace, pki: p, proposer: ops, signer: lead, leadSerial: leadSerial,
		testCatalog: map[string][]seeds.InstallSpec{}}
	// Test catalog for seed.tool curl specs; the real catalog is exercised separately.
	svc.approvalStepsFor = func(seedIDs []string) []seeds.InstallSpec {
		var out []seeds.InstallSpec
		for _, id := range seedIDs {
			out = append(out, h.testCatalog[id]...)
		}
		return out
	}
	return h
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

func toolServer(t *testing.T, payload []byte) (*httptest.Server, string) {
	t.Helper()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(payload) }))
	t.Cleanup(server.Close)
	sum := sha256.Sum256(payload)
	return server, hex.EncodeToString(sum[:])
}

func toolSpec(url, sum, dest string) seeds.InstallSpec {
	return seeds.InstallSpec{
		SeedID: "seed.tool", Method: seeds.InstallMethodCurl, URL: url, Version: "1.0.0",
		SHA256: sum, Destination: dest, Binary: filepath.Base(dest), InstallToBin: true,
	}
}

// Plan whose intent (by ops) proposes seed.tool with test catalog steps specs, approved by lead.
func (h *installHarness) plan(t *testing.T, specs ...seeds.InstallSpec) InstallPlanRequest {
	t.Helper()
	h.testCatalog["seed.tool"] = specs
	return h.planSignedBy(t, h.proposer, h.signer, h.svc.cfg.GhostID, time.Now().Add(10*time.Minute), []string{"seed.tool"}, specs...)
}

// Plan with full control: proposer signs an intent for ghostID and seedIDs, approver signs specs.
func (h *installHarness) planSignedBy(t *testing.T, proposer, approver *approval.Signer, ghostID string, expires time.Time, seedIDs []string, specs ...seeds.InstallSpec) InstallPlanRequest {
	t.Helper()
	intentNonce, _ := approval.NewNonce()
	intent, err := proposer.SignIntent(approval.Intent{GhostID: ghostID, SeedIDs: seedIDs, ProposedAtMS: uint64(time.Now().UnixMilli()), Nonce: intentNonce})
	if err != nil {
		t.Fatal(err)
	}
	nonce, _ := approval.NewNonce()
	signed, err := approver.Sign(approval.Payload{
		ProposalID: "depprop.1", GhostID: ghostID, ProposerIntent: intent,
		Steps: specs, ExpiresAtMS: uint64(expires.UnixMilli()), Nonce: nonce,
	})
	if err != nil {
		t.Fatal(err)
	}
	return InstallPlanRequest{ProposalID: "depprop.1", ApprovedBy: approver.Principal.String(), Specs: specs, Approval: &signed}
}

func (h *installHarness) installed(name string) bool {
	_, err := os.Stat(filepath.Join(h.workspace, "local", "seeds", "seed.tool", name))
	return err == nil
}

func TestInstallPlanRequiresEnabledCAAndApproval(t *testing.T) {
	testlog.Start(t)
	disabled := NewServiceWithConfig(DefaultServiceConfig())
	if _, err := disabled.ExecuteInstallPlan(InstallPlanRequest{}); !errors.Is(err, ErrDependencyInstallDisabled) {
		t.Fatalf("installs disabled: expected ErrDependencyInstallDisabled, got %v", err)
	}

	h := newInstallGhost(t, nil)
	resp := h.svc.handleControlRequest(controlRequest{Action: "install_dependencies", Install: InstallPlanRequest{
		ProposalID: "depprop.1",
		Specs:      []seeds.InstallSpec{{SeedID: "seed.tool", Method: seeds.InstallMethodBrew, Package: "x"}},
	}}, "")
	if resp.OK || resp.ErrorCode != schema.ErrCodeSemanticValidation {
		t.Fatalf("unsigned plan: ok=%v code=%d err=%s", resp.OK, resp.ErrorCode, resp.Error)
	}

	// Fail closed: without an admin CA, approvals cannot be verified and no plan is accepted.
	h.svc.cfg.AdminTLS.CAFile = ""
	if _, err := h.svc.ExecuteInstallPlan(h.plan(t, seeds.InstallSpec{SeedID: "seed.tool", Method: seeds.InstallMethodBrew, Package: "x"})); !errors.Is(err, ErrDependencyInstallDisabled) {
		t.Fatalf("no admin CA: expected ErrDependencyInstallDisabled, got %v", err)
	}
}

// A signed plan with any non-compliant step installs nothing.
func TestInstallPlanPolicyViolationInstallsNothing(t *testing.T) {
	testlog.Start(t)
	server, sum := toolServer(t, []byte("tool"))
	h := newInstallGhost(t, server)

	resp := h.svc.handleControlRequest(controlRequest{Action: "install_dependencies", Install: h.plan(t,
		toolSpec(server.URL, sum, "seed.tool/tool"),
		seeds.InstallSpec{SeedID: "seed.tool", Method: "apt", Package: "docker.io"},
	)}, "")
	if resp.OK || resp.ErrorCode != schema.ErrCodeSemanticValidation {
		t.Fatalf("expected semantic rejection, got ok=%v code=%d err=%s", resp.OK, resp.ErrorCode, resp.Error)
	}
	if h.installed("tool") {
		t.Fatalf("no step may run when the plan violates policy")
	}
}

func TestInstallPlanSuccessViaAdminAction(t *testing.T) {
	testlog.Start(t)
	server, sum := toolServer(t, []byte("tool-binary"))
	h := newInstallGhost(t, server)

	resp := h.svc.handleControlRequest(controlRequest{Action: "install_dependencies", Install: h.plan(t, toolSpec(server.URL, sum, "seed.tool/tool"))}, "")
	if !resp.OK {
		t.Fatalf("install failed: %s", resp.Error)
	}
	raw, _ := json.Marshal(resp.Data)
	var result InstallPlanResult
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if !result.Succeeded || len(result.Steps) != 1 || result.Steps[0].Status != "installed" {
		t.Fatalf("unexpected result: %+v", result)
	}

	status := h.svc.DependencyStatus([]string{"tool", "definitely-not-installed-xyz"})
	if len(status) != 2 || !status[0].Present || status[0].Path != filepath.Join(h.workspace, "local", "bin", "tool") || status[1].Present {
		t.Fatalf("unexpected dependency status: %+v", status)
	}
}

// A failed step stops the plan and rolls back steps this plan already installed.
func TestInstallPlanFailureRollsBackPriorSteps(t *testing.T) {
	testlog.Start(t)
	server, sum := toolServer(t, []byte("tool-binary"))
	h := newInstallGhost(t, server)

	result, err := h.svc.ExecuteInstallPlan(h.plan(t,
		toolSpec(server.URL, sum, "seed.tool/first"),
		toolSpec(server.URL, "1111111111111111111111111111111111111111111111111111111111111111", "seed.tool/second"),
		toolSpec(server.URL, sum, "seed.tool/third"),
	))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.Succeeded {
		t.Fatalf("expected failure")
	}
	statuses := []string{result.Steps[0].Status, result.Steps[1].Status, result.Steps[2].Status}
	if statuses[0] != "installed" || statuses[1] != "failed" || statuses[2] != "not_attempted" {
		t.Fatalf("step statuses = %v", statuses)
	}
	if len(result.Rollback) != 1 || !result.Rollback[0].RolledBack {
		t.Fatalf("expected first step rolled back, got %+v", result.Rollback)
	}
	if h.installed("first") {
		t.Fatalf("rolled-back artifact still present")
	}
}

// Ghost verifies approvals itself: foreign roots, other ghosts, changed steps, expiry, and replays fail.
func TestInstallPlanVerifiesApprovalIndependently(t *testing.T) {
	testlog.Start(t)
	server, sum := toolServer(t, []byte("tool-binary"))
	h := newInstallGhost(t, server)
	spec := toolSpec(server.URL, sum, "seed.tool/tool")
	h.testCatalog["seed.tool"] = []seeds.InstallSpec{spec}
	future := time.Now().Add(10 * time.Minute)
	self := h.svc.cfg.GhostID
	tool := []string{"seed.tool"}

	foreignPKI := tlstest.NewPKI(t)
	foreignLead, _ := operatorSigner(t, foreignPKI, "lead")
	foreignOps, _ := operatorSigner(t, foreignPKI, "ops")
	if _, err := h.svc.ExecuteInstallPlan(h.planSignedBy(t, foreignOps, foreignLead, self, future, tool, spec)); !errors.Is(err, approval.ErrInvalidApproval) {
		t.Errorf("foreign root: expected ErrInvalidApproval, got %v", err)
	}
	if _, err := h.svc.ExecuteInstallPlan(h.planSignedBy(t, h.proposer, h.signer, "ghost.other", future, tool, spec)); !errors.Is(err, ErrDependencyPlanUnapproved) {
		t.Errorf("other ghost: expected ErrDependencyPlanUnapproved, got %v", err)
	}
	changed := h.plan(t, spec)
	changed.Specs = []seeds.InstallSpec{toolSpec(server.URL, sum, "seed.tool/other")}
	if _, err := h.svc.ExecuteInstallPlan(changed); !errors.Is(err, ErrDependencyPlanUnapproved) {
		t.Errorf("changed steps: expected ErrDependencyPlanUnapproved, got %v", err)
	}
	h.svc.approvalNow = func() time.Time { return future.Add(time.Minute) }
	if _, err := h.svc.ExecuteInstallPlan(h.planSignedBy(t, h.proposer, h.signer, self, future, tool, spec)); !errors.Is(err, approval.ErrApprovalExpired) {
		t.Errorf("expired: expected ErrApprovalExpired, got %v", err)
	}
	h.svc.approvalNow = nil
	if h.installed("tool") || h.installed("other") {
		t.Fatalf("rejected approvals must not install")
	}

	once := h.plan(t, spec)
	if _, err := h.svc.ExecuteInstallPlan(once); err != nil {
		t.Fatalf("first use: %v", err)
	}
	if _, err := h.svc.ExecuteInstallPlan(once); !errors.Is(err, ErrApprovalReplayed) {
		t.Fatalf("replay: expected ErrApprovalReplayed, got %v", err)
	}

	// The approver alone re-wraps the consumed intent in a fresh approval: still one intent, one install.
	rewrapped, err := h.signer.Sign(approval.Payload{
		ProposalID: "depprop.1", GhostID: self, ProposerIntent: once.Approval.Payload.ProposerIntent,
		Steps: []seeds.InstallSpec{spec}, ExpiresAtMS: uint64(future.UnixMilli()), Nonce: "fresh-nonce",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.ExecuteInstallPlan(InstallPlanRequest{ProposalID: "depprop.1", Approval: &rewrapped}); !errors.Is(err, ErrApprovalReplayed) {
		t.Fatalf("re-wrapped intent: expected ErrApprovalReplayed, got %v", err)
	}

	// After a restart the in-memory record is empty, so intents proposed before start are refused.
	h.svc.startedAt = time.Now().Add(time.Hour)
	if _, err := h.svc.ExecuteInstallPlan(h.plan(t, spec)); !errors.Is(err, ErrApprovalReplayed) {
		t.Fatalf("intent before start: expected ErrApprovalReplayed, got %v", err)
	}
}

// Separation of duties is enforced at the Ghost from the two signatures, independent of Mirage:
// one operator signing both the intent and the approval is refused, as is an approval whose
// proposer intent names another Ghost, and any approval from a denied operator certificate.
func TestInstallPlanEnforcesTwoOperatorsAtGhost(t *testing.T) {
	testlog.Start(t)
	server, sum := toolServer(t, []byte("tool-binary"))
	h := newInstallGhost(t, server)
	spec := toolSpec(server.URL, sum, "seed.tool/tool")
	h.testCatalog["seed.tool"] = []seeds.InstallSpec{spec}
	future := time.Now().Add(10 * time.Minute)
	tool := []string{"seed.tool"}

	solo := h.planSignedBy(t, h.signer, h.signer, h.svc.cfg.GhostID, future, tool, spec)
	resp := h.svc.handleControlRequest(controlRequest{Action: "install_dependencies", Install: solo}, "")
	if resp.OK || resp.ErrorCode != schema.ErrCodeSemanticValidation {
		t.Fatalf("single-operator plan: ok=%v code=%d err=%s", resp.OK, resp.ErrorCode, resp.Error)
	}
	if _, err := h.svc.ExecuteInstallPlan(solo); !errors.Is(err, approval.ErrSeparationOfDuties) {
		t.Errorf("single operator: expected ErrSeparationOfDuties, got %v", err)
	}

	// Intent for another Ghost re-wrapped in an approval for this Ghost.
	other := h.planSignedBy(t, h.proposer, h.signer, "ghost.other", future, tool, spec)
	rewrapped, err := h.signer.Sign(approval.Payload{
		ProposalID: "depprop.1", GhostID: h.svc.cfg.GhostID, ProposerIntent: other.Approval.Payload.ProposerIntent,
		Steps: []seeds.InstallSpec{spec}, ExpiresAtMS: uint64(future.UnixMilli()), Nonce: "rewrap",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.ExecuteInstallPlan(InstallPlanRequest{ProposalID: "depprop.1", Approval: &rewrapped}); !errors.Is(err, approval.ErrInvalidApproval) {
		t.Errorf("intent for other ghost: expected ErrInvalidApproval, got %v", err)
	}

	h.svc.cfg.AdminTLS.DeniedSerials = []string{h.leadSerial}
	if _, err := h.svc.ExecuteInstallPlan(h.plan(t, spec)); !errors.Is(err, approval.ErrInvalidApproval) {
		t.Errorf("denied approver: expected ErrInvalidApproval, got %v", err)
	}
	if h.installed("tool") {
		t.Fatalf("rejected approvals must not install")
	}
}

// With the real catalog, signed steps must be exactly the catalog steps for the intent seeds.
func TestInstallPlanRejectsUncatalogedStepWithRealCatalog(t *testing.T) {
	testlog.Start(t)
	server, sum := toolServer(t, []byte("payload"))
	h := newInstallGhost(t, server)
	h.svc.approvalStepsFor = nil
	future := time.Now().Add(10 * time.Minute)

	// seed.tool has no catalog entries, so no signed step can match.
	resp := h.svc.handleControlRequest(controlRequest{Action: "install_dependencies",
		Install: h.planSignedBy(t, h.proposer, h.signer, h.svc.cfg.GhostID, future, []string{"seed.tool"}, toolSpec(server.URL, sum, "seed.tool/tool"))}, "")
	if resp.OK || resp.ErrorCode != schema.ErrCodeSemanticValidation {
		t.Fatalf("uncataloged step: ok=%v code=%d err=%s", resp.OK, resp.ErrorCode, resp.Error)
	}
	if h.installed("tool") {
		t.Fatalf("uncataloged step must not install")
	}

	// The proposer asked for seed.docker; a brew step with an extra tap is not its catalog step.
	tampered := seeds.InstallSpec{SeedID: "seed.docker", Method: seeds.InstallMethodBrew, Package: "docker", Binary: "docker", InstallToBin: true, Tap: "evil/tap"}
	if _, err := h.svc.ExecuteInstallPlan(h.planSignedBy(t, h.proposer, h.signer, h.svc.cfg.GhostID, future, []string{"seed.docker"}, tampered)); !errors.Is(err, ErrDependencyNotCataloged) {
		t.Fatalf("tampered catalog step: expected ErrDependencyNotCataloged, got %v", err)
	}
}

// Bootstrap command fields never decode from the wire, so a plan cannot carry commands to run.
func TestInstallPlanWireDropsBootstrapCommand(t *testing.T) {
	raw := []byte(`{"action":"install_dependencies","install":{"proposal_id":"p","approved_by":"a","specs":[` +
		`{"seed_id":"seed.docker","method":"brew","package":"docker","bootstrap_if_missing":true,` +
		`"bootstrap_command":["/bin/sh","-c","touch /tmp/pwned"]}]}}`)
	var req controlRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		t.Fatalf("decode: %v", err)
	}
	spec := req.Install.Specs[0]
	if spec.BootstrapIfMissing || len(spec.BootstrapCommand) != 0 {
		t.Fatalf("bootstrap fields decoded from wire: %+v", spec)
	}
}
