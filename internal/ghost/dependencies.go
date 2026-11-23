package ghost

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/danmuck/edgectl/internal/depcatalog"
	"github.com/danmuck/edgectl/internal/seeds"
	logs "github.com/danmuck/smplog"
)

var (
	// This Ghost is not configured to accept dependency install plans (seed_install_enabled=false).
	ErrDependencyInstallDisabled = errors.New("ghost: dependency installs disabled")
	// Plan lacks the Mirage proposal and approval it must carry.
	ErrDependencyPlanUnapproved = errors.New("ghost: dependency plan lacks mirage approval")
	// Plan step is not an exact entry of the compiled-in dependency catalog.
	ErrDependencyNotCataloged = errors.New("ghost: dependency plan step not in catalog")
)

// InstallPlanRequest is a Mirage-approved dependency install plan delivered over the admin boundary.
// Ghost never originates installs; it executes only plans that present the admin auth token, carry a
// proposal id and approver, and consist solely of exact dependency-catalog entries.
type InstallPlanRequest struct {
	ProposalID string              `json:"proposal_id"`
	ApprovedBy string              `json:"approved_by"`
	Specs      []seeds.InstallSpec `json:"specs"`
	// Admin frame auth token; required (Ghost refuses plans when none is configured).
	Auth string `json:"auth,omitempty"`
}

// InstallStepResult is the outcome of one plan step.
type InstallStepResult struct {
	SeedID string `json:"seed_id"`
	Method string `json:"method"`
	// installed | failed | not_attempted
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

// RollbackStepResult reports removal of one previously installed step after a plan failure.
type RollbackStepResult struct {
	SeedID     string `json:"seed_id"`
	Method     string `json:"method"`
	RolledBack bool   `json:"rolled_back"`
	Error      string `json:"error,omitempty"`
}

// InstallPlanResult is the execution report Ghost returns to Mirage.
type InstallPlanResult struct {
	ProposalID string               `json:"proposal_id"`
	Succeeded  bool                 `json:"succeeded"`
	Steps      []InstallStepResult  `json:"steps"`
	Rollback   []RollbackStepResult `json:"rollback,omitempty"`
}

// DependencyStatus reports where a required binary resolves, or that it is missing.
type DependencyStatus struct {
	Binary  string `json:"binary"`
	Present bool   `json:"present"`
	Path    string `json:"path,omitempty"`
}

// Installer constructor seam; tests substitute runners and HTTP clients.
func (s *Service) newInstaller() (*seeds.Installer, error) {
	cfg := seeds.InstallerConfig{
		WorkspaceRoot: s.cfg.SeedInstall.WorkspaceRoot,
		InstallRoot:   s.cfg.SeedInstall.InstallRoot,
		BinRoot:       s.cfg.SeedInstall.BinRoot,
		Whitelist:     s.cfg.SeedInstall.Whitelist,
	}
	if s.installerConfigHook != nil {
		s.installerConfigHook(&cfg)
	}
	return seeds.NewInstaller(cfg)
}

// ExecuteInstallPlan runs a Mirage-approved plan. The caller must present the configured admin token,
// and every spec is policy-checked and matched against the catalog before any step runs, so a
// non-compliant or uncataloged plan installs nothing. Steps run in order; on the first failure, steps this
// plan already installed under the managed roots are rolled back in reverse order.
func (s *Service) ExecuteInstallPlan(req InstallPlanRequest) (InstallPlanResult, error) {
	if !s.cfg.SeedInstall.Enabled {
		return InstallPlanResult{}, ErrDependencyInstallDisabled
	}
	// Fail closed: without an admin auth token any admin client could submit a plan.
	token := strings.TrimSpace(s.cfg.AdminFrameAuthToken)
	if token == "" {
		return InstallPlanResult{}, fmt.Errorf("%w: admin_frame_auth_token must be configured to accept install plans", ErrDependencyInstallDisabled)
	}
	if err := defaultCommandFrameAuthValidator(token)([]byte(strings.TrimSpace(req.Auth))); err != nil {
		return InstallPlanResult{}, err
	}
	if strings.TrimSpace(req.ProposalID) == "" || strings.TrimSpace(req.ApprovedBy) == "" {
		return InstallPlanResult{}, fmt.Errorf("%w: proposal_id and approved_by required", ErrDependencyPlanUnapproved)
	}
	if len(req.Specs) == 0 {
		return InstallPlanResult{}, fmt.Errorf("%w: empty plan", seeds.ErrInstallInvalidSpec)
	}
	inCatalog := s.catalogContains
	if inCatalog == nil {
		inCatalog = depcatalog.Contains
	}
	for i, spec := range req.Specs {
		if err := seeds.ValidateSpec(spec); err != nil {
			return InstallPlanResult{}, fmt.Errorf("specs[%d]: %w", i, err)
		}
		if !inCatalog(spec) {
			return InstallPlanResult{}, fmt.Errorf("specs[%d] seed_id=%q: %w", i, spec.SeedID, ErrDependencyNotCataloged)
		}
	}

	s.installMu.Lock()
	defer s.installMu.Unlock()

	installer, err := s.newInstaller()
	if err != nil {
		return InstallPlanResult{}, err
	}

	result := InstallPlanResult{ProposalID: req.ProposalID, Succeeded: true}
	var installed []seeds.InstallSpec
	for i, spec := range req.Specs {
		step := InstallStepResult{SeedID: spec.SeedID, Method: string(spec.Method)}
		if !result.Succeeded {
			step.Status = "not_attempted"
			result.Steps = append(result.Steps, step)
			continue
		}
		logs.Infof("ghost.dependency install start proposal_id=%q step=%d seed_id=%q method=%q", req.ProposalID, i, spec.SeedID, spec.Method)
		if err := installer.Install(spec); err != nil {
			step.Status = "failed"
			step.Error = err.Error()
			result.Succeeded = false
			logs.Warnf("ghost.dependency install failed proposal_id=%q step=%d seed_id=%q err=%q", req.ProposalID, i, spec.SeedID, err.Error())
		} else {
			step.Status = "installed"
			installed = append(installed, spec)
		}
		result.Steps = append(result.Steps, step)
	}

	if !result.Succeeded {
		for i := len(installed) - 1; i >= 0; i-- {
			spec := installed[i]
			rb := RollbackStepResult{SeedID: spec.SeedID, Method: string(spec.Method)}
			ok, err := installer.Uninstall(spec)
			rb.RolledBack = ok
			if err != nil {
				rb.Error = err.Error()
			} else if !ok {
				rb.Error = "method is not rolled back (outside managed roots)"
			}
			result.Rollback = append(result.Rollback, rb)
		}
	}
	return result, nil
}

// DependencyStatus checks each binary in the managed bin root first, then PATH.
func (s *Service) DependencyStatus(binaries []string) []DependencyStatus {
	binRoot := strings.TrimSpace(s.cfg.SeedInstall.BinRoot)
	if binRoot == "" {
		binRoot = filepath.Join("local", "bin")
	}
	if !filepath.IsAbs(binRoot) && strings.TrimSpace(s.cfg.SeedInstall.WorkspaceRoot) != "" {
		binRoot = filepath.Join(s.cfg.SeedInstall.WorkspaceRoot, binRoot)
	}
	out := make([]DependencyStatus, 0, len(binaries))
	for _, raw := range binaries {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		st := DependencyStatus{Binary: name}
		managed := filepath.Join(binRoot, filepath.Base(name))
		if info, err := os.Stat(managed); err == nil && !info.IsDir() {
			st.Present, st.Path = true, managed
		} else if path, err := exec.LookPath(name); err == nil {
			st.Present, st.Path = true, path
		}
		out = append(out, st)
	}
	return out
}
