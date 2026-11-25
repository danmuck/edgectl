package mirage

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/danmuck/edgectl/internal/adminauth"
	"github.com/danmuck/edgectl/internal/approval"
	"github.com/danmuck/edgectl/internal/depcatalog"
	"github.com/danmuck/edgectl/internal/identity"
	"github.com/danmuck/edgectl/internal/seeds"
)

var (
	// Proposal request is malformed or resolves to nothing installable.
	ErrInvalidDependencyProposal = errors.New("mirage: invalid dependency proposal")
	// Proposal id is unknown.
	ErrDependencyProposalNotFound = errors.New("mirage: dependency proposal not found")
	// Requested transition is not allowed from the proposal's current state.
	ErrDependencyProposalState = errors.New("mirage: invalid dependency proposal state")
)

// DependencyProposalState is the approval/execution lifecycle of one install proposal
// (docs/progress/mvp_p9_dependency_approval_flow.mmd).
type DependencyProposalState string

const (
	ProposalPending   DependencyProposalState = "pending"
	ProposalApproved  DependencyProposalState = "approved"
	ProposalExecuting DependencyProposalState = "executing"
	ProposalSucceeded DependencyProposalState = "succeeded"
	ProposalFailed    DependencyProposalState = "failed"
)

// DependencyAuditKind names the audited lifecycle steps required by Slice F.
type DependencyAuditKind string

const (
	AuditProposal        DependencyAuditKind = "proposal"
	AuditApproval        DependencyAuditKind = "approval"
	AuditExecutionStart  DependencyAuditKind = "execution_start"
	AuditExecutionResult DependencyAuditKind = "execution_result"
	AuditRollback        DependencyAuditKind = "rollback"
)

// DependencyAuditRecord is one immutable audit entry on a proposal.
type DependencyAuditRecord struct {
	Kind   DependencyAuditKind     `json:"kind"`
	Actor  string                  `json:"actor"`
	State  DependencyProposalState `json:"state"`
	Detail string                  `json:"detail,omitempty"`
	AtMS   uint64                  `json:"at_ms"`
}

// DependencyStepResult mirrors one Ghost plan step outcome.
type DependencyStepResult struct {
	SeedID string `json:"seed_id"`
	Method string `json:"method"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

// DependencyRollbackResult mirrors one Ghost rollback step.
type DependencyRollbackResult struct {
	SeedID     string `json:"seed_id"`
	Method     string `json:"method"`
	RolledBack bool   `json:"rolled_back"`
	Error      string `json:"error,omitempty"`
}

// DependencyExecution is the recorded result of executing an approved proposal on its Ghost.
type DependencyExecution struct {
	Succeeded bool                       `json:"succeeded"`
	Steps     []DependencyStepResult     `json:"steps,omitempty"`
	Rollback  []DependencyRollbackResult `json:"rollback,omitempty"`
	// Dispatch or Ghost-side rejection that prevented the plan from running.
	Error string `json:"error,omitempty"`
}

// DependencyProposal is one Mirage-owned install plan for a Ghost, built only from the catalog.
type DependencyProposal struct {
	ID         string                  `json:"id"`
	GhostID    string                  `json:"ghost_id"`
	SeedIDs    []string                `json:"seed_ids"`
	Steps      []seeds.InstallSpec     `json:"steps"`
	Unresolved []string                `json:"unresolved,omitempty"`
	State      DependencyProposalState `json:"state"`
	ProposedBy string                  `json:"proposed_by"`
	// Proposer-signed intent; approvals must embed it unchanged so Ghosts can verify the proposer.
	Intent     approval.SignedIntent `json:"intent"`
	ApprovedBy string                `json:"approved_by,omitempty"`
	// Operator-signed approval forwarded to the Ghost, which re-verifies it before installing.
	Approval   *approval.Signed        `json:"approval,omitempty"`
	ExecutedBy string                  `json:"executed_by,omitempty"`
	Execution  *DependencyExecution    `json:"execution,omitempty"`
	Audit      []DependencyAuditRecord `json:"audit"`
}

// DependencyLedger holds proposals and their audit trail in memory.
type DependencyLedger struct {
	mu        sync.Mutex
	seq       uint64
	proposals map[string]*DependencyProposal
	// Intents already proposed, keyed by proposer and intent content; one intent, one proposal.
	intents map[string]struct{}
	now     func() time.Time
}

// NewDependencyLedger returns an empty ledger.
func NewDependencyLedger() *DependencyLedger {
	return &DependencyLedger{proposals: make(map[string]*DependencyProposal), intents: make(map[string]struct{}), now: time.Now}
}

// Propose builds a pending proposal for a verified proposer intent (caller ran approval.VerifyIntent).
// Steps are approval.CatalogSteps for the intent seeds, so they match what every Ghost recomputes;
// each is re-validated against install policy. Dependencies without a catalog source are listed as
// unresolved; a proposal with no installable step, or a reused intent, is rejected.
func (l *DependencyLedger) Propose(intent approval.SignedIntent, proposer string) (DependencyProposal, error) {
	ghostID := intent.Intent.GhostID
	proposer = strings.TrimSpace(proposer)
	if strings.TrimSpace(ghostID) == "" || proposer == "" || len(intent.Intent.SeedIDs) == 0 {
		return DependencyProposal{}, fmt.Errorf("%w: ghost_id, seed_ids, and proposer required", ErrInvalidDependencyProposal)
	}
	steps := approval.CatalogSteps(intent.Intent.SeedIDs)
	for _, spec := range steps {
		if err := seeds.ValidateSpec(spec); err != nil {
			return DependencyProposal{}, fmt.Errorf("%w: catalog entry %s/%s: %v", ErrInvalidDependencyProposal, spec.SeedID, spec.Package, err)
		}
	}
	var unresolved []string
	for _, raw := range intent.Intent.SeedIDs {
		seedID := strings.TrimSpace(raw)
		for _, name := range depcatalog.UncatalogedDependencies(seedID) {
			unresolved = append(unresolved, seedID+":"+name)
		}
	}
	if len(steps) == 0 {
		return DependencyProposal{}, fmt.Errorf("%w: no catalog install sources for seeds %v (unresolved: %v)", ErrInvalidDependencyProposal, intent.Intent.SeedIDs, unresolved)
	}

	pp, err := identity.ParsePrincipal(proposer)
	if err != nil {
		return DependencyProposal{}, fmt.Errorf("%w: proposer: %v", ErrInvalidDependencyProposal, err)
	}
	key, err := intent.ReplayKey(pp)
	if err != nil {
		return DependencyProposal{}, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, used := l.intents[key]; used {
		return DependencyProposal{}, fmt.Errorf("%w: intent already proposed", ErrInvalidDependencyProposal)
	}
	l.intents[key] = struct{}{}
	l.seq++
	p := &DependencyProposal{
		ID:         fmt.Sprintf("depprop.%s.%d", ghostID, l.seq),
		GhostID:    ghostID,
		SeedIDs:    append([]string{}, intent.Intent.SeedIDs...),
		Steps:      steps,
		Unresolved: unresolved,
		State:      ProposalPending,
		ProposedBy: proposer,
		Intent:     cloneIntent(intent),
	}
	l.auditLocked(p, AuditProposal, proposer, fmt.Sprintf("steps=%d unresolved=%v intent_signature=%s", len(steps), unresolved, intent.SignatureDigest()))
	l.proposals[p.ID] = p
	return cloneProposal(p), nil
}

// Approve records an operator-signed approval on a pending proposal. The caller has already verified
// both signatures (approval.Verify); here the signed payload must match the stored proposal exactly
// (id, ghost, proposer intent, steps) and the approver must differ from the proposer.
func (l *DependencyLedger) Approve(id string, approver string, signed approval.Signed) (DependencyProposal, error) {
	approver = strings.TrimSpace(approver)
	if approver == "" {
		return DependencyProposal{}, fmt.Errorf("%w: approver required", ErrInvalidDependencyProposal)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	p, ok := l.proposals[strings.TrimSpace(id)]
	if !ok {
		return DependencyProposal{}, fmt.Errorf("%w: %s", ErrDependencyProposalNotFound, id)
	}
	if p.State != ProposalPending {
		return DependencyProposal{}, fmt.Errorf("%w: %s is %s, want %s", ErrDependencyProposalState, p.ID, p.State, ProposalPending)
	}
	pl := signed.Payload
	if pl.ProposalID != p.ID || pl.GhostID != p.GhostID || !reflect.DeepEqual(pl.ProposerIntent, p.Intent) || !reflect.DeepEqual(pl.Steps, p.Steps) {
		return DependencyProposal{}, fmt.Errorf("%w: signed payload does not match proposal %s", ErrInvalidDependencyProposal, p.ID)
	}
	if approver == p.ProposedBy {
		return DependencyProposal{}, fmt.Errorf("%w: %v", ErrInvalidDependencyProposal, approval.ErrSeparationOfDuties)
	}
	p.State = ProposalApproved
	p.ApprovedBy = approver
	signedCopy := signed
	p.Approval = &signedCopy
	l.auditLocked(p, AuditApproval, approver, fmt.Sprintf("proposer=%s approver=%s intent_signature=%s signature=%s decision=approved",
		p.ProposedBy, approver, p.Intent.SignatureDigest(), signed.SignatureDigest()))
	return cloneProposal(p), nil
}

// BeginExecution moves an approved proposal to executing; Ghost dispatch happens only after this.
func (l *DependencyLedger) BeginExecution(id string, actor string) (DependencyProposal, error) {
	return l.transition(id, actor, ProposalApproved, ProposalExecuting, AuditExecutionStart, "", func(p *DependencyProposal) {
		p.ExecutedBy = strings.TrimSpace(actor)
	})
}

// CompleteExecution records the Ghost result, closing the proposal as succeeded or failed, and
// audits rollback state when the Ghost rolled back any step.
func (l *DependencyLedger) CompleteExecution(id string, exec DependencyExecution) (DependencyProposal, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	p, ok := l.proposals[strings.TrimSpace(id)]
	if !ok {
		return DependencyProposal{}, fmt.Errorf("%w: %s", ErrDependencyProposalNotFound, id)
	}
	if p.State != ProposalExecuting {
		return DependencyProposal{}, fmt.Errorf("%w: %s is %s, want %s", ErrDependencyProposalState, p.ID, p.State, ProposalExecuting)
	}
	execCopy := exec
	p.Execution = &execCopy
	p.State = ProposalFailed
	if exec.Succeeded {
		p.State = ProposalSucceeded
	}
	detail := fmt.Sprintf("succeeded=%v steps=%d", exec.Succeeded, len(exec.Steps))
	if exec.Error != "" {
		detail += " error=" + exec.Error
	}
	l.auditLocked(p, AuditExecutionResult, p.ExecutedBy, detail)
	if len(exec.Rollback) > 0 {
		rolled := 0
		for _, rb := range exec.Rollback {
			if rb.RolledBack {
				rolled++
			}
		}
		state := "rolled_back"
		if rolled < len(exec.Rollback) {
			state = "partial"
		}
		l.auditLocked(p, AuditRollback, p.GhostID, fmt.Sprintf("%s %d/%d", state, rolled, len(exec.Rollback)))
	}
	return cloneProposal(p), nil
}

// Get returns one proposal.
func (l *DependencyLedger) Get(id string) (DependencyProposal, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	p, ok := l.proposals[strings.TrimSpace(id)]
	if !ok {
		return DependencyProposal{}, false
	}
	return cloneProposal(p), true
}

// List returns every proposal ordered by id.
func (l *DependencyLedger) List() []DependencyProposal {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]DependencyProposal, 0, len(l.proposals))
	for _, p := range l.proposals {
		out = append(out, cloneProposal(p))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Shared state-machine step: from -> to with one audit record.
func (l *DependencyLedger) transition(
	id string,
	actor string,
	from DependencyProposalState,
	to DependencyProposalState,
	kind DependencyAuditKind,
	detail string,
	apply func(*DependencyProposal),
) (DependencyProposal, error) {
	actor = strings.TrimSpace(actor)
	if actor == "" {
		return DependencyProposal{}, fmt.Errorf("%w: actor required", ErrInvalidDependencyProposal)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	p, ok := l.proposals[strings.TrimSpace(id)]
	if !ok {
		return DependencyProposal{}, fmt.Errorf("%w: %s", ErrDependencyProposalNotFound, id)
	}
	if p.State != from {
		return DependencyProposal{}, fmt.Errorf("%w: %s is %s, want %s", ErrDependencyProposalState, p.ID, p.State, from)
	}
	p.State = to
	apply(p)
	l.auditLocked(p, kind, actor, detail)
	return cloneProposal(p), nil
}

// Appends an audit record and writes a matching structured log line. Caller holds l.mu.
func (l *DependencyLedger) auditLocked(p *DependencyProposal, kind DependencyAuditKind, actor string, detail string) {
	rec := DependencyAuditRecord{Kind: kind, Actor: actor, State: p.State, Detail: detail, AtMS: uint64(l.now().UnixMilli())}
	p.Audit = append(p.Audit, rec)
	// admin_security.toml [audit] approval_fields: written regardless of log level.
	_ = adminauth.Recordf(adminauth.AuditInfo,
		"mirage.dependency.audit component=mirage proposal_id=%q ghost_id=%q kind=%s state=%s actor=%q detail=%q timestamp_ms=%d",
		p.ID, p.GhostID, kind, p.State, actor, detail, rec.AtMS,
	)
}

func cloneProposal(p *DependencyProposal) DependencyProposal {
	out := *p
	out.SeedIDs = append([]string{}, p.SeedIDs...)
	out.Steps = append([]seeds.InstallSpec{}, p.Steps...)
	out.Unresolved = append([]string{}, p.Unresolved...)
	out.Audit = append([]DependencyAuditRecord{}, p.Audit...)
	out.Intent = cloneIntent(p.Intent)
	if p.Approval != nil {
		a := *p.Approval
		a.Signature = append([]byte{}, p.Approval.Signature...)
		a.ApproverCert = append([]byte{}, p.Approval.ApproverCert...)
		a.Payload.Steps = append([]seeds.InstallSpec{}, p.Approval.Payload.Steps...)
		a.Payload.ProposerIntent = cloneIntent(p.Approval.Payload.ProposerIntent)
		out.Approval = &a
	}
	if p.Execution != nil {
		exec := *p.Execution
		exec.Steps = append([]DependencyStepResult{}, p.Execution.Steps...)
		exec.Rollback = append([]DependencyRollbackResult{}, p.Execution.Rollback...)
		out.Execution = &exec
	}
	return out
}

// Deep copy so callers cannot mutate the ledger's stored intent.
func cloneIntent(si approval.SignedIntent) approval.SignedIntent {
	out := si
	out.Intent.SeedIDs = append([]string{}, si.Intent.SeedIDs...)
	out.Signature = append([]byte{}, si.Signature...)
	out.ProposerCert = append([]byte{}, si.ProposerCert...)
	return out
}
