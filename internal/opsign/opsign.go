// Package opsign builds the two things an operator signs for a dependency install: the proposer
// intent and the approval payload (docs/architecture/definitions/admin_security.toml
// [install_approval]).
//
// Ownership boundary:
// - one implementation of these checks for every operator tool (client-tm, edgecert), so a
// terminal and a web front end cannot disagree on what is safe to sign
//
// - builds and checks only; signing is approval.Signer, verification is Mirage and every Ghost
//
// Canonical references (consult before changes):
// - docs/index.md
//
// - docs/architecture/definitions/admin_security.toml
//
// - docs/architecture/definitions/api_v1.toml
//
// - docs/progress/mvp_p9_dependency_approval_flow.mmd
package opsign

import (
	"crypto/x509"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/danmuck/edgectl/internal/approval"
	"github.com/danmuck/edgectl/internal/identity"
	"github.com/danmuck/edgectl/internal/mirage"
)

// ApprovalLifetime keeps approvals well under approval.MaxLifetime.
const ApprovalLifetime = 10 * time.Minute

// NewIntent builds a proposer intent with a fresh nonce; ghost id and at least one seed id are
// required.
func NewIntent(ghostID string, seedIDs []string, now time.Time) (approval.Intent, error) {
	ghostID = strings.TrimSpace(ghostID)
	if ghostID == "" || len(seedIDs) == 0 {
		return approval.Intent{}, errors.New("ghost id and at least one seed id required")
	}
	nonce, err := approval.NewNonce()
	if err != nil {
		return approval.Intent{}, err
	}
	return approval.Intent{GhostID: ghostID, SeedIDs: seedIDs, ProposedAtMS: uint64(now.UnixMilli()), Nonce: nonce}, nil
}

// ApprovalPayload builds the payload approver would sign for p without trusting Mirage's view of
// it: p must be pending, its intent must verify under roots from a different operator, name p's
// ghost, and p's steps must equal this binary's catalog steps for the intent seeds.
func ApprovalPayload(p mirage.DependencyProposal, approver identity.Principal, roots *x509.CertPool, now time.Time) (approval.Payload, error) {
	if p.State != mirage.ProposalPending {
		return approval.Payload{}, fmt.Errorf("proposal %s is %s; only pending proposals can be approved", p.ID, p.State)
	}
	proposer, err := approval.VerifyIntent(p.Intent, approval.Trust{Roots: roots, Now: now})
	if err != nil {
		return approval.Payload{}, fmt.Errorf("proposer intent: %w", err)
	}
	if proposer == approver {
		return approval.Payload{}, approval.ErrSeparationOfDuties
	}
	if proposer.String() != p.ProposedBy || p.Intent.Intent.GhostID != p.GhostID {
		return approval.Payload{}, fmt.Errorf("proposal %s does not match its signed intent", p.ID)
	}
	if !reflect.DeepEqual(p.Steps, approval.CatalogSteps(p.Intent.Intent.SeedIDs)) {
		return approval.Payload{}, fmt.Errorf("proposal %s: %w", p.ID, approval.ErrStepsMismatch)
	}
	nonce, err := approval.NewNonce()
	if err != nil {
		return approval.Payload{}, err
	}
	return approval.Payload{
		ProposalID:     p.ID,
		GhostID:        p.GhostID,
		ProposerIntent: p.Intent,
		Steps:          p.Steps,
		ExpiresAtMS:    uint64(now.Add(ApprovalLifetime).UnixMilli()),
		Nonce:          nonce,
	}, nil
}
