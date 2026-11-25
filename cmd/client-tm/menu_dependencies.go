package main

// menu_dependencies.go drives dependency install proposals: sign-and-propose, review,
// sign-and-approve, execute (admin_security.toml [install_approval];
// docs/progress/mvp_p9_dependency_approval_flow.mmd). Nothing is signed without explicit confirmation
// (install_approval.operator_confirmation).

import (
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/danmuck/edgectl/internal/approval"
	"github.com/danmuck/edgectl/internal/mirage"
	"github.com/danmuck/edgectl/internal/opsign"
	"github.com/danmuck/edgectl/internal/seeds"
	logs "github.com/danmuck/smplog"
)

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
			if err := a.proposeInstall(target, ghostID, splitComma(seedLine)); err != nil {
				logs.Errf("propose failed: %v", err)
			}
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

// Signs a proposer intent after confirmation and submits it; Mirage derives the steps from its catalog.
func (a *App) proposeInstall(target MirageTarget, ghostID string, seedIDs []string) error {
	signer, _, err := a.operatorSigner()
	if err != nil {
		return err
	}
	intent, err := newIntent(ghostID, seedIDs, time.Now())
	if err != nil {
		return err
	}
	digest, err := intent.Digest()
	if err != nil {
		return err
	}
	fmt.Printf("\nPropose as %s\n  ghost: %s\n  seeds: %s\n", signer.Principal, intent.GhostID, strings.Join(intent.SeedIDs, ", "))
	for _, st := range approval.CatalogSteps(intent.SeedIDs) {
		fmt.Printf("  catalog step: %s\n", describeStep(st))
	}
	fmt.Printf("  intent digest: %s\n", hex.EncodeToString(digest[:]))
	ok, err := a.promptConfirm("Sign this proposal intent")
	if err != nil || !ok {
		if err == nil {
			fmt.Println("Not signed.")
		}
		return err
	}
	signed, err := signer.SignIntent(intent)
	if err != nil {
		return err
	}
	p, err := target.Admin.ProposeDependencies(signed)
	if err != nil {
		return err
	}
	printProposal(p)
	return nil
}

// Builds an intent; shared with other operator tooling (internal/opsign).
func newIntent(ghostID string, seedIDs []string, now time.Time) (approval.Intent, error) {
	return opsign.NewIntent(ghostID, seedIDs, now)
}

// Fetches the stored proposal, checks it independently, shows exactly what will be signed, and signs
// only after explicit confirmation.
func (a *App) approveProposal(target MirageTarget, id string) error {
	list, err := target.Admin.ListDependencyProposals(id)
	if err != nil {
		return err
	}
	if len(list) != 1 {
		return fmt.Errorf("proposal %q not found", id)
	}
	p := list[0]
	signer, roots, err := a.operatorSigner()
	if err != nil {
		return err
	}
	payload, err := approvalPayload(p, signer, roots, time.Now())
	if err != nil {
		return err
	}
	digest, err := payload.Digest()
	if err != nil {
		return err
	}
	printProposal(p)
	fmt.Printf("\nApprove as %s\n  proposer (verified intent signature): %s\n  expires: %s\n  payload digest: %s\n",
		signer.Principal, p.ProposedBy, time.UnixMilli(int64(payload.ExpiresAtMS)).Format(time.RFC3339), hex.EncodeToString(digest[:]))
	ok, err := a.promptConfirm("Sign this approval")
	if err != nil || !ok {
		if err == nil {
			fmt.Println("Not signed.")
		}
		return err
	}
	signed, err := signer.Sign(payload)
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

// Builds the approval payload for p without trusting Mirage's view of it; shared with other
// operator tooling (internal/opsign).
func approvalPayload(p mirage.DependencyProposal, signer *approval.Signer, roots *x509.CertPool, now time.Time) (approval.Payload, error) {
	return opsign.ApprovalPayload(p, signer.Principal, roots, now)
}

// Loads the operator signing key and the root used to check other operators' signatures.
func (a *App) operatorSigner() (*approval.Signer, *x509.CertPool, error) {
	creds, ok := a.operatorCredentials(true)
	if !ok {
		return nil, nil, errors.New("signing requires operator_cert_file and operator_key_file")
	}
	signer, err := approval.LoadSigner(creds.CertFile, creds.KeyFile)
	if err != nil {
		return nil, nil, err
	}
	if creds.CAFile == "" {
		return nil, nil, errors.New("signing requires admin_tls_ca_file to verify proposals")
	}
	raw, err := os.ReadFile(creds.CAFile)
	if err != nil {
		return nil, nil, err
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(raw) {
		return nil, nil, fmt.Errorf("no certificates in %s", creds.CAFile)
	}
	return signer, roots, nil
}

// Full JSON of a step: every signed field is shown, so confirmation covers exactly what the digest covers.
func describeStep(s seeds.InstallSpec) string {
	raw, err := json.Marshal(s)
	if err != nil {
		return fmt.Sprintf("%+v", s)
	}
	return string(raw)
}

func printProposal(p mirage.DependencyProposal) {
	fmt.Printf("\n%s  ghost=%s  state=%s  proposed_by=%s", p.ID, p.GhostID, p.State, p.ProposedBy)
	if p.ApprovedBy != "" {
		fmt.Printf("  approved_by=%s", p.ApprovedBy)
	}
	fmt.Println()
	for _, st := range p.Steps {
		fmt.Printf("  step: %s\n", describeStep(st))
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
