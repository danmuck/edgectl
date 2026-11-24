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

// installHarness is a Ghost that accepts install plans plus an operator who signs approvals.
type installHarness struct {
	svc       *Service
	workspace string
	pki       *tlstest.PKI
	signer    *approval.Signer
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
	// Test catalog: accept the test's seed.tool curl specs; real catalog membership is tested separately.
	svc.catalogContains = func(spec seeds.InstallSpec) bool { return spec.SeedID == "seed.tool" }
	if server != nil {
		svc.installerConfigHook = func(c *seeds.InstallerConfig) { c.HTTPClient = server.Client() }
	}
	return &installHarness{svc: svc, workspace: workspace, pki: p, signer: operatorSigner(t, p, "lead")}
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

// Plan carrying an operator-signed approval for this Ghost over exactly specs.
func (h *installHarness) plan(t *testing.T, specs ...seeds.InstallSpec) InstallPlanRequest {
	t.Helper()
	return h.planSignedBy(t, h.signer, h.svc.cfg.GhostID, time.Now().Add(10*time.Minute), specs...)
}

func (h *installHarness) planSignedBy(t *testing.T, s *approval.Signer, ghostID string, expires time.Time, specs ...seeds.InstallSpec) InstallPlanRequest {
	t.Helper()
	nonce, _ := approval.NewNonce()
	signed, err := s.Sign(approval.Payload{
		ProposalID: "depprop.1", GhostID: ghostID, ProposedBy: "edgectl://operator/ops",
		Steps: specs, ExpiresAtMS: uint64(expires.UnixMilli()), Nonce: nonce,
	})
	if err != nil {
		t.Fatal(err)
	}
	return InstallPlanRequest{ProposalID: "depprop.1", ApprovedBy: s.Principal.String(), Specs: specs, Approval: &signed}
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
	future := time.Now().Add(10 * time.Minute)

	foreign := operatorSigner(t, tlstest.NewPKI(t), "lead")
	if _, err := h.svc.ExecuteInstallPlan(h.planSignedBy(t, foreign, h.svc.cfg.GhostID, future, spec)); !errors.Is(err, approval.ErrInvalidApproval) {
		t.Errorf("foreign root: expected ErrInvalidApproval, got %v", err)
	}
	if _, err := h.svc.ExecuteInstallPlan(h.planSignedBy(t, h.signer, "ghost.other", future, spec)); !errors.Is(err, ErrDependencyPlanUnapproved) {
		t.Errorf("other ghost: expected ErrDependencyPlanUnapproved, got %v", err)
	}
	changed := h.plan(t, spec)
	changed.Specs = []seeds.InstallSpec{toolSpec(server.URL, sum, "seed.tool/other")}
	if _, err := h.svc.ExecuteInstallPlan(changed); !errors.Is(err, ErrDependencyPlanUnapproved) {
		t.Errorf("changed steps: expected ErrDependencyPlanUnapproved, got %v", err)
	}
	h.svc.approvalNow = func() time.Time { return future.Add(time.Minute) }
	if _, err := h.svc.ExecuteInstallPlan(h.planSignedBy(t, h.signer, h.svc.cfg.GhostID, future, spec)); !errors.Is(err, approval.ErrApprovalExpired) {
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
}

// With the real catalog, a signed step that is not an exact catalog entry is refused and nothing runs.
func TestInstallPlanRejectsUncatalogedStepWithRealCatalog(t *testing.T) {
	testlog.Start(t)
	server, sum := toolServer(t, []byte("payload"))
	h := newInstallGhost(t, server)
	h.svc.catalogContains = nil

	resp := h.svc.handleControlRequest(controlRequest{Action: "install_dependencies", Install: h.plan(t, toolSpec(server.URL, sum, "seed.tool/tool"))}, "")
	if resp.OK || resp.ErrorCode != schema.ErrCodeSemanticValidation {
		t.Fatalf("uncataloged step: ok=%v code=%d err=%s", resp.OK, resp.ErrorCode, resp.Error)
	}
	if h.installed("tool") {
		t.Fatalf("uncataloged step must not install")
	}

	// A brew step with the catalog package but an extra tap is no longer an exact entry.
	tampered := seeds.InstallSpec{SeedID: "seed.docker", Method: seeds.InstallMethodBrew, Package: "docker", Binary: "docker", InstallToBin: true, Tap: "evil/tap"}
	if _, err := h.svc.ExecuteInstallPlan(h.plan(t, tampered)); !errors.Is(err, ErrDependencyNotCataloged) {
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
