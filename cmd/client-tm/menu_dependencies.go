package main

// menu_dependencies.go drives dependency install proposals: propose, review, sign-and-approve,
// execute (admin_security.toml [install_approval]; docs/progress/mvp_p9_dependency_approval_flow.mmd).

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/danmuck/edgectl/internal/approval"
	"github.com/danmuck/edgectl/internal/mirage"
	logs "github.com/danmuck/smplog"
)

// approvalLifetime keeps approvals well under approval.MaxLifetime.
const approvalLifetime = 10 * time.Minute

func (a *App) runMirageDependencyConsole(target MirageTarget) error {
	for {
		fmt.Println()
		fmt.Printf("Dependency Console (%s)\n", target.Name)
		fmt.Println("  1) List proposals")
		fmt.Println("  2) Propose install")
		fmt.Println("  3) Approve proposal (signs with your operator key)")
		fmt.Println("  4) Execute approved proposal")
		fmt.Println("  5) Back")
		choice, err := a.promptInt("Choose", 1, 5, true, true)
		if err != nil {
			return err
		}
		a.clearIfEnabled()
		switch choice {
		case 1:
			list, err := target.Admin.ListDependencyProposals("")
			if err != nil {
				logs.Errf("list proposals failed: %v", err)
				continue
			}
			for _, p := range list {
				printProposal(p)
			}
		case 2:
			ghostID, err := a.promptLine("Ghost id")
			if err != nil {
				return err
			}
			seedLine, err := a.promptLine("Seed ids (comma-separated)")
			if err != nil {
				return err
			}
			p, err := target.Admin.ProposeDependencies(ghostID, splitComma(seedLine))
			if err != nil {
				logs.Errf("propose failed: %v", err)
				continue
			}
			printProposal(p)
		case 3:
			id, err := a.promptLine("Proposal id")
			if err != nil {
				return err
			}
			if err := a.approveProposal(target, id); err != nil {
				logs.Errf("approve failed: %v", err)
			}
		case 4:
			id, err := a.promptLine("Proposal id")
			if err != nil {
				return err
			}
			p, err := target.Admin.ExecuteDependencies(id)
			if err != nil {
				logs.Errf("execute failed: %v", err)
				continue
			}
			printProposal(p)
		case 5:
			return nil
		}
	}
}

// Fetches the stored proposal, signs exactly its content, and submits the approval.
func (a *App) approveProposal(target MirageTarget, id string) error {
	list, err := target.Admin.ListDependencyProposals(id)
	if err != nil {
		return err
	}
	if len(list) != 1 {
		return fmt.Errorf("proposal %q not found", id)
	}
	p := list[0]
	printProposal(p)
	signed, err := a.signProposal(p, time.Now())
	if err != nil {
		return err
	}
	out, err := target.Admin.ApproveDependencies(p.ID, signed)
	if err != nil {
		return err
	}
	printProposal(out)
	return nil
}

// signProposal signs the proposal content with the configured operator key.
func (a *App) signProposal(p mirage.DependencyProposal, now time.Time) (approval.Signed, error) {
	creds, ok := a.operatorCredentials(true)
	if !ok {
		return approval.Signed{}, errors.New("approving requires operator_cert_file and operator_key_file")
	}
	signer, err := approval.LoadSigner(creds.CertFile, creds.KeyFile)
	if err != nil {
		return approval.Signed{}, err
	}
	nonce, err := approval.NewNonce()
	if err != nil {
		return approval.Signed{}, err
	}
	return signer.Sign(approval.Payload{
		ProposalID:  p.ID,
		GhostID:     p.GhostID,
		ProposedBy:  p.ProposedBy,
		Steps:       p.Steps,
		ExpiresAtMS: uint64(now.Add(approvalLifetime).UnixMilli()),
		Nonce:       nonce,
	})
}

func printProposal(p mirage.DependencyProposal) {
	fmt.Printf("\n%s  ghost=%s  state=%s  proposed_by=%s", p.ID, p.GhostID, p.State, p.ProposedBy)
	if p.ApprovedBy != "" {
		fmt.Printf("  approved_by=%s", p.ApprovedBy)
	}
	fmt.Println()
	for _, s := range p.Steps {
		fmt.Printf("  step: %s %s %s%s\n", s.SeedID, s.Method, s.Package, s.URL)
	}
	if len(p.Unresolved) > 0 {
		fmt.Printf("  unresolved: %s\n", strings.Join(p.Unresolved, ", "))
	}
	for _, rec := range p.Audit {
		fmt.Printf("  audit: %-16s state=%-9s actor=%s %s\n", rec.Kind, rec.State, rec.Actor, rec.Detail)
	}
}

func splitComma(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if v := strings.TrimSpace(part); v != "" {
			out = append(out, v)
		}
	}
	return out
}
