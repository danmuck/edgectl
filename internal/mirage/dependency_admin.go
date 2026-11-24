package mirage

import (
	"context"
	"fmt"
	"strings"
)

// AdminDependencyRequest carries dependency proposal admin parameters.
type AdminDependencyRequest struct {
	ProposalID string   `json:"proposal_id,omitempty"`
	GhostID    string   `json:"ghost_id,omitempty"`
	SeedIDs    []string `json:"seed_ids,omitempty"`
	Actor      string   `json:"actor,omitempty"`
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

// Routes dependency admin actions; ok=false when action is not a dependency action.
func (s *Service) handleDependencyAdminAction(action string, req AdminDependencyRequest) (adminControlResponse, bool) {
	ledger := s.server.Dependencies()
	switch action {
	case "propose_dependencies":
		p, err := ledger.Propose(req.GhostID, req.SeedIDs, req.Actor)
		if err != nil {
			return failAdmin(err), true
		}
		return adminControlResponse{OK: true, Data: p}, true
	case "approve_dependencies":
		p, err := ledger.Approve(req.ProposalID, req.Actor)
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
