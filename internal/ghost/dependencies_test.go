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

	"github.com/danmuck/edgectl/internal/protocol/schema"
	"github.com/danmuck/edgectl/internal/seeds"
	"github.com/danmuck/edgectl/internal/testutil/testlog"
)

// Ghost with installs enabled, a temp workspace, and installer HTTP pointed at a TLS test server.
func newInstallGhost(t *testing.T, server *httptest.Server) (*Service, string) {
	t.Helper()
	workspace := t.TempDir()
	cfg := DefaultServiceConfig()
	cfg.AdminFrameAuthToken = testInstallToken
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
	return svc, workspace
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

const testInstallToken = "install-secret"

func approvedPlan(specs ...seeds.InstallSpec) InstallPlanRequest {
	return InstallPlanRequest{ProposalID: "depprop.ghost.alpha.1", ApprovedBy: "user:approver", Specs: specs, Auth: testInstallToken}
}

func TestInstallPlanRequiresEnabledAndApproval(t *testing.T) {
	testlog.Start(t)
	disabled := NewServiceWithConfig(DefaultServiceConfig())
	if _, err := disabled.ExecuteInstallPlan(approvedPlan()); !errors.Is(err, ErrDependencyInstallDisabled) {
		t.Fatalf("expected ErrDependencyInstallDisabled, got %v", err)
	}

	svc, _ := newInstallGhost(t, nil)
	resp := svc.handleControlRequest(controlRequest{Action: "install_dependencies", Install: InstallPlanRequest{
		Specs: []seeds.InstallSpec{{SeedID: "seed.tool", Method: seeds.InstallMethodBrew, Package: "x"}},
		Auth:  testInstallToken,
	}}, "")
	if resp.OK || resp.ErrorCode != schema.ErrCodeSemanticValidation {
		t.Fatalf("unapproved plan: ok=%v code=%d err=%s", resp.OK, resp.ErrorCode, resp.Error)
	}
}

// A plan with any non-compliant step installs nothing.
func TestInstallPlanPolicyViolationInstallsNothing(t *testing.T) {
	testlog.Start(t)
	server, sum := toolServer(t, []byte("tool"))
	svc, workspace := newInstallGhost(t, server)

	resp := svc.handleControlRequest(controlRequest{Action: "install_dependencies", Install: approvedPlan(
		toolSpec(server.URL, sum, "seed.tool/tool"),
		seeds.InstallSpec{SeedID: "seed.tool", Method: "apt", Package: "docker.io"},
	)}, "")
	if resp.OK || resp.ErrorCode != schema.ErrCodeSemanticValidation {
		t.Fatalf("expected semantic rejection, got ok=%v code=%d err=%s", resp.OK, resp.ErrorCode, resp.Error)
	}
	if _, err := os.Stat(filepath.Join(workspace, "local", "seeds", "seed.tool", "tool")); !os.IsNotExist(err) {
		t.Fatalf("no step may run when the plan violates policy, stat err=%v", err)
	}
}

func TestInstallPlanSuccessViaAdminAction(t *testing.T) {
	testlog.Start(t)
	server, sum := toolServer(t, []byte("tool-binary"))
	svc, workspace := newInstallGhost(t, server)

	resp := svc.handleControlRequest(controlRequest{Action: "install_dependencies", Install: approvedPlan(toolSpec(server.URL, sum, "seed.tool/tool"))}, "")
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

	status := svc.DependencyStatus([]string{"tool", "definitely-not-installed-xyz"})
	if len(status) != 2 || !status[0].Present || status[0].Path != filepath.Join(workspace, "local", "bin", "tool") || status[1].Present {
		t.Fatalf("unexpected dependency status: %+v", status)
	}
}

// A failed step stops the plan and rolls back steps this plan already installed.
func TestInstallPlanFailureRollsBackPriorSteps(t *testing.T) {
	testlog.Start(t)
	server, sum := toolServer(t, []byte("tool-binary"))
	svc, workspace := newInstallGhost(t, server)

	result, err := svc.ExecuteInstallPlan(approvedPlan(
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
	if _, err := os.Stat(filepath.Join(workspace, "local", "seeds", "seed.tool", "first")); !os.IsNotExist(err) {
		t.Fatalf("rolled-back artifact still present, stat err=%v", err)
	}
}

// Fail closed: with installs enabled but no admin token configured, no plan is accepted.
func TestInstallPlanRefusedWithoutConfiguredToken(t *testing.T) {
	testlog.Start(t)
	svc, _ := newInstallGhost(t, nil)
	svc.cfg.AdminFrameAuthToken = ""
	_, err := svc.ExecuteInstallPlan(approvedPlan(seeds.InstallSpec{SeedID: "seed.tool", Method: seeds.InstallMethodBrew, Package: "x"}))
	if !errors.Is(err, ErrDependencyInstallDisabled) {
		t.Fatalf("expected ErrDependencyInstallDisabled without token, got %v", err)
	}
}

func TestInstallPlanRejectsWrongToken(t *testing.T) {
	testlog.Start(t)
	svc, _ := newInstallGhost(t, nil)
	plan := approvedPlan(seeds.InstallSpec{SeedID: "seed.tool", Method: seeds.InstallMethodBrew, Package: "x"})
	plan.Auth = "guess"
	resp := svc.handleControlRequest(controlRequest{Action: "install_dependencies", Install: plan}, "")
	if resp.OK || resp.ErrorCode != schema.ErrCodeTransportFailure {
		t.Fatalf("wrong token: ok=%v code=%d err=%s", resp.OK, resp.ErrorCode, resp.Error)
	}
}

// With the real catalog, a step that is not an exact catalog entry is refused and nothing runs.
func TestInstallPlanRejectsUncatalogedStepWithRealCatalog(t *testing.T) {
	testlog.Start(t)
	server, sum := toolServer(t, []byte("payload"))
	svc, workspace := newInstallGhost(t, server)
	svc.catalogContains = nil

	resp := svc.handleControlRequest(controlRequest{Action: "install_dependencies", Install: approvedPlan(toolSpec(server.URL, sum, "seed.tool/tool"))}, "")
	if resp.OK || resp.ErrorCode != schema.ErrCodeSemanticValidation {
		t.Fatalf("uncataloged step: ok=%v code=%d err=%s", resp.OK, resp.ErrorCode, resp.Error)
	}
	if _, err := os.Stat(filepath.Join(workspace, "local", "seeds", "seed.tool", "tool")); !os.IsNotExist(err) {
		t.Fatalf("uncataloged step must not install, stat err=%v", err)
	}

	// A brew step with the catalog package but an extra tap is no longer an exact entry.
	tampered := seeds.InstallSpec{SeedID: "seed.docker", Method: seeds.InstallMethodBrew, Package: "docker", Binary: "docker", InstallToBin: true, Tap: "evil/tap"}
	if _, err := svc.ExecuteInstallPlan(approvedPlan(tampered)); !errors.Is(err, ErrDependencyNotCataloged) {
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
