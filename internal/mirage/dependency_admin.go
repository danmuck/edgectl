package mirage

import (
	"context"
	"crypto/x509"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/danmuck/edgectl/internal/approval"
	"github.com/danmuck/edgectl/internal/identity"
)

// AdminDependencyRequest carries dependency proposal admin parameters.
type AdminDependencyRequest struct {
	ProposalID string   `json:"proposal_id,omitempty"`
	GhostID    string   `json:"ghost_id,omitempty"`
	SeedIDs    []string `json:"seed_ids,omitempty"`
	Actor      string   `json:"actor,omitempty"`
	// Proposer-signed intent (propose_dependencies); its ghost_id and seed_ids define the proposal.
	Intent *approval.SignedIntent `json:"intent,omitempty"`
	// Operator-signed approval (approve_dependencies).
	Approval *approval.Signed `json:"approval,omitempty"`
}

// dependencyInstaller dispatches an approved plan to one Ghost; replaceable in tests.
type dependencyInstaller func(ctx context.Context, adminAddr string, p DependencyProposal) (DependencyExecution, error)

// ExecuteDependencyProposal runs an approved proposal on its Ghost. Execution is initiated and
// tracked by Mirage only: the ledger moves approved -> executing before dispatch, and the Ghost
// result (or dispatch failure) closes it as succeeded/failed with audit records.
func (s *Service) ExecuteDependencyProposal(ctx context.Context, id string, actor string) (DependencyProposal, error) {
	ledger := s.server.Dependencies()
	p, err := ledger.BeginExecution(id, actor)
	if err != nil {
		return DependencyProposal{}, err
	}

	s.adminGhostMu.RLock()
	addr := s.adminGhostAddrs[p.GhostID]
	s.adminGhostMu.RUnlock()

	var exec DependencyExecution
	if strings.TrimSpace(addr) == "" {
		exec = DependencyExecution{Error: fmt.Sprintf("no admin route for ghost_id=%q", p.GhostID)}
	} else {
		install := s.dependencyInstaller
		if install == nil {
			install = func(ctx context.Context, adminAddr string, p DependencyProposal) (DependencyExecution, error) {
				return s.newGhostControlClient(p.GhostID, adminAddr).InstallDependencies(ctx, p)
			}
		}
		result, err := install(ctx, addr, p)
		if err != nil {
			exec = DependencyExecution{Error: err.Error()}
		} else {
			exec = result
		}
	}
	return ledger.CompleteExecution(p.ID, exec)
}

// ProposeDependencyProposal verifies a proposer-signed intent and records a pending proposal whose
// steps come only from the catalog (admin_security.toml [install_approval] mirage_verification). The
// intent signer must be the calling operator; actor is already bound to the caller principal.
func (s *Service) ProposeDependencyProposal(actor string, intent *approval.SignedIntent) (DependencyProposal, error) {
	if intent == nil {
		return DependencyProposal{}, fmt.Errorf("%w: signed proposer intent required", approval.ErrInvalidApproval)
	}
	tr, err := s.approvalTrust()
	if err != nil {
		return DependencyProposal{}, err
	}
	proposer, err := approval.VerifyIntent(*intent, tr)
	if err != nil {
		return DependencyProposal{}, err
	}
	if a := strings.TrimSpace(actor); a != "" && a != proposer.String() {
		return DependencyProposal{}, fmt.Errorf("%w: intent signed by %s but submitted by %s", approval.ErrInvalidApproval, proposer, a)
	}
	return s.server.Dependencies().Propose(*intent, proposer.String())
}

// ApproveDependencyProposal verifies an operator-signed approval (both signatures, separation,
// expiry, catalog steps) and records it. The approver must be the calling operator; the ledger then
// requires the payload to match the stored proposal and intent exactly.
func (s *Service) ApproveDependencyProposal(id string, actor string, signed *approval.Signed) (DependencyProposal, error) {
	if signed == nil {
		return DependencyProposal{}, fmt.Errorf("%w: signed approval required", approval.ErrInvalidApproval)
	}
	tr, err := s.approvalTrust()
	if err != nil {
		return DependencyProposal{}, err
	}
	res, err := approval.Verify(*signed, tr)
	if err != nil {
		return DependencyProposal{}, err
	}
	if a := strings.TrimSpace(actor); a != "" && a != res.Approver.String() {
		return DependencyProposal{}, fmt.Errorf("%w: approval signed by %s but submitted by %s", approval.ErrInvalidApproval, res.Approver, a)
	}
	return s.server.Dependencies().Approve(id, res.Approver.String(), *signed)
}

// Trust for signature verification: the admin CA plus admin denied serials. Fails closed when admin
// TLS is not configured or the denied list does not parse.
func (s *Service) approvalTrust() (approval.Trust, error) {
	caFile := strings.TrimSpace(s.cfg.AdminTLS.CAFile)
	if caFile == "" {
		return approval.Trust{}, fmt.Errorf("%w: admin_tls_ca_file required to verify approvals", approval.ErrInvalidApproval)
	}
	pemBytes, err := os.ReadFile(caFile)
	if err != nil {
		return approval.Trust{}, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pemBytes) {
		return approval.Trust{}, fmt.Errorf("%w: no certificates in %s", approval.ErrInvalidApproval, caFile)
	}
	denied, err := identity.ParseDeniedSerials(s.cfg.AdminTLS.DeniedSerials)
	if err != nil {
		return approval.Trust{}, err
	}
	return approval.Trust{Roots: pool, Denied: denied, Now: time.Now()}, nil
}

// Routes dependency admin actions; ok=false when action is not a dependency action.
func (s *Service) handleDependencyAdminAction(action string, req AdminDependencyRequest) (adminControlResponse, bool) {
	ledger := s.server.Dependencies()
	switch action {
	case "propose_dependencies":
		p, err := s.ProposeDependencyProposal(req.Actor, req.Intent)
		if err != nil {
			return failAdmin(err), true
		}
		return adminControlResponse{OK: true, Data: p}, true
	case "approve_dependencies":
		p, err := s.ApproveDependencyProposal(req.ProposalID, req.Actor, req.Approval)
		if err != nil {
			return failAdmin(err), true
		}
		return adminControlResponse{OK: true, Data: p}, true
	case "execute_dependencies":
		ctx, cancel := context.WithTimeout(context.Background(), dependencyInstallTimeout)
		defer cancel()
		p, err := s.ExecuteDependencyProposal(ctx, req.ProposalID, req.Actor)
		if err != nil {
			return failAdmin(err), true
		}
		return adminControlResponse{OK: true, Data: p}, true
	case "list_dependency_proposals":
		if id := strings.TrimSpace(req.ProposalID); id != "" {
			p, ok := ledger.Get(id)
			if !ok {
				return failAdmin(fmt.Errorf("%w: %s", ErrDependencyProposalNotFound, id)), true
			}
			return adminControlResponse{OK: true, Data: []DependencyProposal{p}}, true
		}
		return adminControlResponse{OK: true, Data: ledger.List()}, true
	}
	return adminControlResponse{}, false
}
