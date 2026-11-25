package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/danmuck/edgectl/internal/approval"
	"github.com/danmuck/edgectl/internal/mirage"
)

// Root plus operator certificates ops and lead, made with edgecert itself.
type signFixture struct {
	ca, certs string
}

func newSignFixture(t *testing.T) signFixture {
	t.Helper()
	dir := t.TempDir()
	root := filepath.Join(dir, "root")
	if err := run([]string{"root", "-out", root}); err != nil {
		t.Fatalf("root: %v", err)
	}
	f := signFixture{ca: filepath.Join(root, "ca.crt"), certs: filepath.Join(dir, "certs")}
	for _, id := range []string{"ops", "lead"} {
		if err := run([]string{"issue", "-ca-cert", f.ca, "-ca-key", filepath.Join(root, "ca.key"), "-role", "operator", "-id", id, "-out", f.certs}); err != nil {
			t.Fatalf("issue %s: %v", id, err)
		}
	}
	return f
}

func (f signFixture) creds(id string) []string {
	return []string{"-cert", filepath.Join(f.certs, id+".crt"), "-key", filepath.Join(f.certs, id+".key")}
}

// Runs one edgecert command with in on stdin and decodes its stdout into out.
func runSign(t *testing.T, in any, out any, args ...string) error {
	t.Helper()
	var stdin bytes.Buffer
	switch v := in.(type) {
	case nil:
	case string:
		stdin.WriteString(v)
	default:
		if err := json.NewEncoder(&stdin).Encode(v); err != nil {
			t.Fatal(err)
		}
	}
	var stdout bytes.Buffer
	prevIn, prevOut := signIn, signOut
	signIn, signOut = &stdin, &stdout
	defer func() { signIn, signOut = prevIn, prevOut }()
	if err := run(args); err != nil {
		if stdout.Len() != 0 {
			t.Errorf("%v: wrote output on failure: %s", args, stdout.String())
		}
		return err
	}
	if out != nil {
		if err := json.Unmarshal(stdout.Bytes(), out); err != nil {
			t.Fatalf("%v: decode %q: %v", args, stdout.String(), err)
		}
	}
	return nil
}

// intent -> sign-intent -> approval -> sign-approval yields an approval that verifies as
// lead approving ops' proposal, the same result the terminal client produces.
func TestSigningCommandsProduceVerifiableApproval(t *testing.T) {
	f := newSignFixture(t)

	var built intentOutput
	if err := runSign(t, nil, &built, "intent", "-ghost", "ghost.alpha", "-seeds", "seed.docker"); err != nil {
		t.Fatalf("intent: %v", err)
	}
	if built.Intent.GhostID != "ghost.alpha" || built.Intent.Nonce == "" || len(built.Digest) != 64 || len(built.Steps) == 0 {
		t.Fatalf("intent output: %+v", built)
	}
	var signedIntent signedIntentOutput
	if err := runSign(t, built.Intent, &signedIntent, append([]string{"sign-intent"}, f.creds("ops")...)...); err != nil {
		t.Fatalf("sign-intent: %v", err)
	}
	if signedIntent.Principal != "edgectl://operator/ops" || signedIntent.Digest != built.Digest {
		t.Fatalf("sign-intent output: principal=%s digest=%s want digest=%s", signedIntent.Principal, signedIntent.Digest, built.Digest)
	}

	// The proposal as Mirage would return it for that intent.
	proposal := mirage.DependencyProposal{
		ID: "depprop.ghost.alpha.1", GhostID: "ghost.alpha", SeedIDs: []string{"seed.docker"},
		ProposedBy: "edgectl://operator/ops", State: mirage.ProposalPending,
		Intent: signedIntent.SignedIntent, Steps: approval.CatalogSteps([]string{"seed.docker"}),
		Audit: []mirage.DependencyAuditRecord{},
	}
	approvalArgs := func(id string) []string {
		return append(append([]string{"approval"}, f.creds(id)...), "-ca", f.ca)
	}
	var payload approvalOutput
	if err := runSign(t, proposal, &payload, approvalArgs("lead")...); err != nil {
		t.Fatalf("approval: %v", err)
	}
	if payload.Approver != "edgectl://operator/lead" || payload.Proposer != "edgectl://operator/ops" ||
		len(payload.Digest) != 64 || payload.ExpiresAtMS != payload.Payload.ExpiresAtMS {
		t.Fatalf("approval output: %+v", payload)
	}

	signArgs := func(id string, digest string) []string {
		return append(append([]string{"sign-approval"}, f.creds(id)...), "-digest", digest)
	}
	var signed signedApprovalOutput
	if err := runSign(t, payload.Payload, &signed, signArgs("lead", payload.Digest)...); err != nil {
		t.Fatalf("sign-approval: %v", err)
	}
	roots, err := loadRoots(f.ca)
	if err != nil {
		t.Fatal(err)
	}
	res, err := approval.Verify(signed.Approval, approval.Trust{Roots: roots, Now: time.Now()})
	if err != nil || res.Approver.String() != "edgectl://operator/lead" || res.Proposer.String() != "edgectl://operator/ops" {
		t.Fatalf("verify: %+v %v", res, err)
	}

	// Signing is refused unless the caller passes the digest the operator confirmed.
	if err := runSign(t, payload.Payload, nil, append([]string{"sign-approval"}, f.creds("lead")...)...); err == nil {
		t.Errorf("sign-approval without -digest must fail")
	}
	changed := payload.Payload
	changed.GhostID = "ghost.other"
	if err := runSign(t, changed, nil, signArgs("lead", payload.Digest)...); err == nil || !strings.Contains(err.Error(), "not signed") {
		t.Errorf("payload changed after confirmation: %v", err)
	}

	// The checks the terminal client makes before showing a payload hold here too.
	if err := runSign(t, proposal, nil, approvalArgs("ops")...); !errors.Is(err, approval.ErrSeparationOfDuties) {
		t.Errorf("proposer building its own approval: %v", err)
	}
	extra := proposal
	extra.Steps = append(append(extra.Steps[:0:0], proposal.Steps...), proposal.Steps[0])
	if err := runSign(t, extra, nil, approvalArgs("lead")...); !errors.Is(err, approval.ErrStepsMismatch) {
		t.Errorf("steps beyond the catalog: %v", err)
	}
	done := proposal
	done.State = mirage.ProposalApproved
	if err := runSign(t, done, nil, approvalArgs("lead")...); err == nil {
		t.Errorf("non-pending proposal must not get a payload")
	}
}

// Malformed input is refused rather than partly read.
func TestSigningCommandsRejectBadInput(t *testing.T) {
	f := newSignFixture(t)
	if err := runSign(t, nil, nil, "intent", "-ghost", "ghost.alpha"); err == nil {
		t.Errorf("intent without seeds must fail")
	}
	signIntent := append([]string{"sign-intent"}, f.creds("ops")...)
	for name, in := range map[string]string{
		"empty":         ``,
		"unknown field": `{"ghost_id":"ghost.alpha","seed_ids":["seed.docker"],"proposed_at_ms":1,"nonce":"n","extra":true}`,
		"trailing data": `{"ghost_id":"ghost.alpha","seed_ids":["seed.docker"],"proposed_at_ms":1,"nonce":"n"} {}`,
		"incomplete":    `{"ghost_id":"ghost.alpha","seed_ids":["seed.docker"]}`,
	} {
		if err := runSign(t, in, nil, signIntent...); err == nil {
			t.Errorf("sign-intent with %s input must fail", name)
		}
	}
	if err := runSign(t, `{}`, nil, "sign-intent", "-cert", filepath.Join(f.certs, "ops.crt"), "-key", filepath.Join(f.certs, "missing.key")); err == nil {
		t.Errorf("sign-intent with a missing key must fail")
	}
}
