package ghost

import (
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/danmuck/edgectl/internal/adminauth"
	"github.com/danmuck/edgectl/internal/approval"
	"github.com/danmuck/edgectl/internal/identity"
	"github.com/danmuck/edgectl/internal/seeds"
	logs "github.com/danmuck/smplog"
)

var (
	// This Ghost is not configured to accept dependency install plans (seed_install_enabled=false).
	ErrDependencyInstallDisabled = errors.New("ghost: dependency installs disabled")
	// Plan lacks the Mirage proposal and approval it must carry.
	ErrDependencyPlanUnapproved = errors.New("ghost: dependency plan lacks mirage approval")
	// Signed steps are not exactly this Ghost's compiled-in catalog steps for the proposer's seeds.
	ErrDependencyNotCataloged = errors.New("ghost: dependency plan step not in catalog")
	// Approval nonce or proposer intent already used, or intent predates this Ghost's start
	// (admin_security.toml install_approval.replay).
	ErrApprovalReplayed = errors.New("ghost: approval nonce already used")
)

// Trust for approval verification: this Ghost's admin CA, admin denied serials, the approval clock,
// and catalog step derivation. Fails closed when the CA is missing or the denied list does not parse.
func (s *Service) approvalTrust() (approval.Trust, error) {
	caFile := strings.TrimSpace(s.cfg.AdminTLS.CAFile)
	if caFile == "" {
		return approval.Trust{}, fmt.Errorf("admin_tls_ca_file required to verify approvals")
	}
	pemBytes, err := os.ReadFile(caFile)
	if err != nil {
		return approval.Trust{}, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pemBytes) {
		return approval.Trust{}, fmt.Errorf("no certificates in %s", caFile)
	}
	denied, err := identity.ParseDeniedSerials(s.cfg.AdminTLS.DeniedSerials)
	if err != nil {
		return approval.Trust{}, err
	}
	return approval.Trust{Roots: pool, Denied: denied, Now: s.approvalClock(), StepsFor: s.approvalStepsFor}, nil
}

func (s *Service) approvalClock() time.Time {
	if s.approvalNow != nil {
		return s.approvalNow()
	}
	return time.Now()
}

// Single-use enforcement (install_approval.replay). The approval nonce is held until the approval
// expires and the proposer intent until it ages out, so neither a replayed approval nor a fresh
// approval re-wrapping a consumed intent installs again. Intents proposed before this Ghost started
// are refused, so a restart (which clears the in-memory record) cannot reopen consumed ones.
// Nothing is recorded unless both checks pass. Caller holds installMu.
func (s *Service) consumeApproval(pl approval.Payload, intentKey string) error {
	now := s.approvalClock()
	if s.usedNonces == nil {
		s.usedNonces = make(map[string]time.Time)
	}
	if s.usedIntents == nil {
		s.usedIntents = make(map[string]time.Time)
	}
	for k, exp := range s.usedNonces {
		if now.After(exp) {
			delete(s.usedNonces, k)
		}
	}
	for k, exp := range s.usedIntents {
		if now.After(exp) {
			delete(s.usedIntents, k)
		}
	}
	proposedAt := time.UnixMilli(int64(pl.ProposerIntent.Intent.ProposedAtMS))
	if !s.startedAt.IsZero() && proposedAt.Before(s.startedAt.Add(-approval.ClockSkew)) {
		return fmt.Errorf("%w: intent proposed before this ghost started; propose again", ErrApprovalReplayed)
	}
	if _, used := s.usedNonces[pl.Nonce]; used {
		return fmt.Errorf("%w: approval nonce %s", ErrApprovalReplayed, pl.Nonce)
	}
	if _, used := s.usedIntents[intentKey]; used {
		return fmt.Errorf("%w: proposer intent already installed", ErrApprovalReplayed)
	}
	s.usedNonces[pl.Nonce] = time.UnixMilli(int64(pl.ExpiresAtMS))
	s.usedIntents[intentKey] = proposedAt.Add(approval.MaxIntentAge + approval.ClockSkew)
	return nil
}

// InstallPlanRequest is a Mirage-approved dependency install plan delivered over the admin boundary.
// Ghost never originates installs; it executes only plans carrying an operator-signed approval for
// this Ghost, whose steps are exact dependency-catalog entries.
type InstallPlanRequest struct {
	ProposalID string              `json:"proposal_id"`
	ApprovedBy string              `json:"approved_by"`
	Specs      []seeds.InstallSpec `json:"specs"`
	// Operator-signed approval; Ghost verifies it independently of Mirage and installs only its steps.
	Approval *approval.Signed `json:"approval,omitempty"`
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

// ExecuteInstallPlan runs a plan carrying a two-operator approval (admin_security.toml
// [install_approval] ghost_verification). This Ghost verifies the proposer intent and the approval
// itself: both operator signatures under its root and not denied, two different principals, expiry
// and intent age, both naming this Ghost, and steps exactly equal to its own catalog steps for the
// intent seeds. Every step is policy-checked before any runs, so a rejected plan installs nothing.
// Steps run in order; on the first failure, steps this plan already installed under the managed roots
// are rolled back in reverse order.
func (s *Service) ExecuteInstallPlan(req InstallPlanRequest) (InstallPlanResult, error) {
	if !s.cfg.SeedInstall.Enabled {
		return InstallPlanResult{}, ErrDependencyInstallDisabled
	}
	// Fail closed: approvals can only be verified against a configured admin root.
	tr, err := s.approvalTrust()
	if err != nil {
		return InstallPlanResult{}, fmt.Errorf("%w: %v", ErrDependencyInstallDisabled, err)
	}
	if strings.TrimSpace(req.ProposalID) == "" || req.Approval == nil {
		return InstallPlanResult{}, fmt.Errorf("%w: proposal_id and signed approval required", ErrDependencyPlanUnapproved)
	}
	signed := *req.Approval
	pl := signed.Payload
	if pl.ProposalID != strings.TrimSpace(req.ProposalID) || pl.GhostID != strings.TrimSpace(s.cfg.GhostID) {
		return InstallPlanResult{}, fmt.Errorf("%w: approval is for %s on %s", ErrDependencyPlanUnapproved, pl.ProposalID, pl.GhostID)
	}
	res, err := approval.Verify(signed, tr)
	if errors.Is(err, approval.ErrStepsMismatch) {
		return InstallPlanResult{}, fmt.Errorf("%w: %w", ErrDependencyNotCataloged, err)
	}
	if err != nil {
		return InstallPlanResult{}, err
	}
	if a := strings.TrimSpace(req.ApprovedBy); a != "" && a != res.Approver.String() {
		return InstallPlanResult{}, fmt.Errorf("%w: approved_by %q is not the signer %s", ErrDependencyPlanUnapproved, a, res.Approver)
	}
	// Only the signed steps run; a plan that lists different steps is refused.
	specs := pl.Steps
	if len(req.Specs) > 0 && !reflect.DeepEqual(req.Specs, specs) {
		return InstallPlanResult{}, fmt.Errorf("%w: plan steps differ from the signed steps", ErrDependencyPlanUnapproved)
	}
	for i, spec := range specs {
		if err := seeds.ValidateSpec(spec); err != nil {
			return InstallPlanResult{}, fmt.Errorf("specs[%d]: %w", i, err)
		}
	}
	// admin_security.toml [audit] approval_fields: written regardless of log level.
	_ = adminauth.Recordf(adminauth.AuditInfo, "ghost.dependency.approval component=ghost ghost_id=%q proposal_id=%q proposer=%q approver=%q intent_signature=%s signature=%s decision=accepted",
		s.cfg.GhostID, pl.ProposalID, res.Proposer, res.Approver, pl.ProposerIntent.SignatureDigest(), signed.SignatureDigest())

	intentKey, err := pl.ProposerIntent.ReplayKey(res.Proposer)
	if err != nil {
		return InstallPlanResult{}, err
	}

	s.installMu.Lock()
	defer s.installMu.Unlock()

	if err := s.consumeApproval(pl, intentKey); err != nil {
		return InstallPlanResult{}, err
	}

	installer, err := s.newInstaller()
	if err != nil {
		return InstallPlanResult{}, err
	}

	result := InstallPlanResult{ProposalID: req.ProposalID, Succeeded: true}
	var installed []seeds.InstallSpec
	for i, spec := range specs {
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
