// sign is lab tooling that produces admin requests carrying operator signatures
// (admin_security.toml [install_approval]) so the boundary can be exercised without client-tm.
// Unlike client-tm it performs no safety checks: it signs whatever it is given, so testers can craft
// stale, self-approved, re-wrapped, or wrong-step payloads and confirm the runtimes refuse them.
//
// Usage (run from the repo root):
//
//	go run ./testutil/adminlab/sign propose -as ops -ghost ghost.lab -seeds seed.docker
//	go run ./testutil/adminlab/sign approve -as lead -proposal resp.json [-expires 10m] [-nonce n]
//	go run ./testutil/adminlab/sign intent  -as ops -ghost ghost.lab -seeds seed.docker [-proposed-at-ms N] [-nonce n]
//	go run ./testutil/adminlab/sign payload -as lead -payload payload.json
//
// -as <name> loads certs/<name>.crt and certs/<name>.key (operator certificates only).
// propose/approve print one admin request line for testutil/adminlab/admin.sh; intent/payload print
// the bare signed object.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/danmuck/edgectl/internal/approval"
	"github.com/danmuck/edgectl/internal/mirage"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "sign:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New("subcommand required: propose | approve | intent | payload")
	}
	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	as := fs.String("as", "", "operator name: certs/<name>.crt and certs/<name>.key")
	ghostID := fs.String("ghost", "", "target ghost_id (propose, intent)")
	seedList := fs.String("seeds", "", "comma-separated seed ids (propose, intent)")
	proposedAt := fs.Int64("proposed-at-ms", 0, "intent proposed_at_ms; 0 means now (intent, propose)")
	nonce := fs.String("nonce", "", "nonce; empty means random")
	proposalFile := fs.String("proposal", "", "proposal JSON, or an admin response containing it; - for stdin (approve)")
	expires := fs.Duration("expires", 10*time.Minute, "approval lifetime from now (approve)")
	payloadFile := fs.String("payload", "", "approval payload JSON to sign as-is; - for stdin (payload)")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *as == "" {
		return errors.New("-as required")
	}
	signer, err := approval.LoadSigner("certs/"+*as+".crt", "certs/"+*as+".key")
	if err != nil {
		return err
	}
	n := *nonce
	if n == "" {
		if n, err = approval.NewNonce(); err != nil {
			return err
		}
	}

	switch args[0] {
	case "intent", "propose":
		at := *proposedAt
		if at == 0 {
			at = time.Now().UnixMilli()
		}
		si, err := signer.SignIntent(approval.Intent{GhostID: *ghostID, SeedIDs: splitList(*seedList), ProposedAtMS: uint64(at), Nonce: n})
		if err != nil {
			return err
		}
		if args[0] == "intent" {
			return emit(si)
		}
		return emit(map[string]any{"action": "propose_dependencies", "dependency": map[string]any{"intent": si}})
	case "approve":
		p, err := readProposal(*proposalFile)
		if err != nil {
			return err
		}
		signed, err := signer.Sign(approval.Payload{
			ProposalID: p.ID, GhostID: p.GhostID, ProposerIntent: p.Intent, Steps: p.Steps,
			ExpiresAtMS: uint64(time.Now().Add(*expires).UnixMilli()), Nonce: n,
		})
		if err != nil {
			return err
		}
		return emit(map[string]any{"action": "approve_dependencies", "dependency": map[string]any{"proposal_id": p.ID, "approval": signed}})
	case "payload":
		raw, err := readInput(*payloadFile)
		if err != nil {
			return err
		}
		var pl approval.Payload
		if err := json.Unmarshal(raw, &pl); err != nil {
			return err
		}
		signed, err := signer.Sign(pl)
		if err != nil {
			return err
		}
		return emit(signed)
	}
	return fmt.Errorf("unknown subcommand %q", args[0])
}

// Accepts a bare proposal, an admin response {"ok":..,"data":proposal}, or a list response (first item).
func readProposal(path string) (mirage.DependencyProposal, error) {
	raw, err := readInput(path)
	if err != nil {
		return mirage.DependencyProposal{}, err
	}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if json.Unmarshal(raw, &envelope) == nil && len(envelope.Data) > 0 {
		raw = envelope.Data
	}
	var list []mirage.DependencyProposal
	if json.Unmarshal(raw, &list) == nil && len(list) > 0 {
		return list[0], nil
	}
	var p mirage.DependencyProposal
	if err := json.Unmarshal(raw, &p); err != nil {
		return p, err
	}
	if p.ID == "" {
		return p, errors.New("no proposal id in input")
	}
	return p, nil
}

func readInput(path string) ([]byte, error) {
	switch path {
	case "":
		return nil, errors.New("input file required")
	case "-":
		return io.ReadAll(os.Stdin)
	}
	return os.ReadFile(path)
}

// One compact JSON line, ready to pass to admin.sh.
func emit(v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	fmt.Println(string(raw))
	return nil
}

func splitList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if v := strings.TrimSpace(part); v != "" {
			out = append(out, v)
		}
	}
	return out
}
