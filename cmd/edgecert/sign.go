package main

// sign.go: non-interactive signing for dependency installs, for operator tooling that is not the
// terminal client (docs/architecture/definitions/api_v1.toml [parity.signing];
// admin_security.toml [install_approval]). Each command reads JSON on stdin where it needs input
// and writes one JSON object on stdout. Building and signing are separate commands so the caller
// can show the digest and get the operator's confirmation in between
// (install_approval.operator_confirmation).

import (
	"crypto/x509"
	"encoding/hex"
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
	"github.com/danmuck/edgectl/internal/opsign"
	"github.com/danmuck/edgectl/internal/seeds"
)

// Cap on stdin for the signing commands; proposals and payloads are small.
const signInputLimit = 1 << 20

// Streams the signing commands use; replaced in tests.
var (
	signIn  io.Reader = os.Stdin
	signOut io.Writer = os.Stdout
)

// intentOutput is what `edgecert intent` prints: the unsigned intent, its digest, and the catalog
// steps it would install, for the operator to review before signing.
type intentOutput struct {
	Intent approval.Intent     `json:"intent"`
	Digest string              `json:"digest"`
	Steps  []seeds.InstallSpec `json:"steps"`
}

// signedIntentOutput is what `edgecert sign-intent` prints.
type signedIntentOutput struct {
	SignedIntent approval.SignedIntent `json:"signed_intent"`
	Digest       string                `json:"digest"`
	Principal    string                `json:"principal"`
}

// approvalOutput is what `edgecert approval` prints: the unsigned payload and its digest.
type approvalOutput struct {
	Payload     approval.Payload `json:"payload"`
	Digest      string           `json:"digest"`
	Approver    string           `json:"approver"`
	Proposer    string           `json:"proposer"`
	ExpiresAtMS uint64           `json:"expires_at_ms"`
}

// signedApprovalOutput is what `edgecert sign-approval` prints.
type signedApprovalOutput struct {
	Approval  approval.Signed `json:"approval"`
	Digest    string          `json:"digest"`
	Principal string          `json:"principal"`
}

// Decodes exactly one JSON value from stdin; unknown fields and trailing data are errors so a
// caller cannot sign something other than what it showed.
func readSignInput(out any) error {
	raw, err := io.ReadAll(io.LimitReader(signIn, signInputLimit+1))
	if err != nil {
		return err
	}
	if len(raw) > signInputLimit {
		return fmt.Errorf("input exceeds %d bytes", signInputLimit)
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return fmt.Errorf("stdin: %w", err)
	}
	if dec.More() {
		return errors.New("stdin: trailing data after JSON value")
	}
	return nil
}

func writeSignOutput(v any) error {
	enc := json.NewEncoder(signOut)
	return enc.Encode(v)
}

func loadRoots(caFile string) (*x509.CertPool, error) {
	raw, err := os.ReadFile(caFile)
	if err != nil {
		return nil, err
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(raw) {
		return nil, fmt.Errorf("no certificates in %s", caFile)
	}
	return roots, nil
}

// edgecert intent -ghost <id> -seeds a,b: builds an unsigned proposer intent. Needs no key.
func runIntent(args []string) error {
	fs := flag.NewFlagSet("intent", flag.ContinueOnError)
	ghostID := fs.String("ghost", "", "ghost id the install is for")
	seedList := fs.String("seeds", "", "comma-separated seed ids")
	if err := fs.Parse(args); err != nil {
		return err
	}
	intent, err := opsign.NewIntent(*ghostID, splitList(*seedList), time.Now())
	if err != nil {
		return err
	}
	digest, err := intent.Digest()
	if err != nil {
		return err
	}
	steps := approval.CatalogSteps(intent.SeedIDs)
	if steps == nil {
		steps = []seeds.InstallSpec{}
	}
	return writeSignOutput(intentOutput{Intent: intent, Digest: hex.EncodeToString(digest[:]), Steps: steps})
}

// edgecert sign-intent -cert -key: signs the intent JSON on stdin as the proposer.
func runSignIntent(args []string) error {
	fs := flag.NewFlagSet("sign-intent", flag.ContinueOnError)
	cert := fs.String("cert", "", "proposer operator certificate")
	key := fs.String("key", "", "proposer operator private key (0600)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	signer, err := approval.LoadSigner(*cert, *key)
	if err != nil {
		return err
	}
	var intent approval.Intent
	if err := readSignInput(&intent); err != nil {
		return err
	}
	if strings.TrimSpace(intent.GhostID) == "" || len(intent.SeedIDs) == 0 || intent.Nonce == "" || intent.ProposedAtMS == 0 {
		return errors.New("intent is incomplete; build it with `edgecert intent`")
	}
	digest, err := intent.Digest()
	if err != nil {
		return err
	}
	signed, err := signer.SignIntent(intent)
	if err != nil {
		return err
	}
	return writeSignOutput(signedIntentOutput{SignedIntent: signed, Digest: hex.EncodeToString(digest[:]), Principal: signer.Principal.String()})
}

// edgecert approval -cert -key -ca: reads a proposal (as Mirage returns it) on stdin, checks it the way
// the terminal client does, and prints the unsigned payload for the approver named by -cert.
func runApproval(args []string) error {
	fs := flag.NewFlagSet("approval", flag.ContinueOnError)
	cert := fs.String("cert", "", "approver operator certificate")
	key := fs.String("key", "", "approver operator private key (0600)")
	ca := fs.String("ca", "", "root CA certificate used to verify the proposer")
	if err := fs.Parse(args); err != nil {
		return err
	}
	signer, err := approval.LoadSigner(*cert, *key)
	if err != nil {
		return err
	}
	roots, err := loadRoots(*ca)
	if err != nil {
		return err
	}
	var proposal mirage.DependencyProposal
	if err := readSignInput(&proposal); err != nil {
		return err
	}
	payload, err := opsign.ApprovalPayload(proposal, signer.Principal, roots, time.Now())
	if err != nil {
		return err
	}
	digest, err := payload.Digest()
	if err != nil {
		return err
	}
	return writeSignOutput(approvalOutput{
		Payload:     payload,
		Digest:      hex.EncodeToString(digest[:]),
		Approver:    signer.Principal.String(),
		Proposer:    proposal.ProposedBy,
		ExpiresAtMS: payload.ExpiresAtMS,
	})
}

// edgecert sign-approval -cert -key: signs the payload JSON on stdin as the approver. The payload
// must be one `edgecert approval` produced; the digest is printed again so the caller can check it
// is the one the operator confirmed.
func runSignApproval(args []string) error {
	fs := flag.NewFlagSet("sign-approval", flag.ContinueOnError)
	cert := fs.String("cert", "", "approver operator certificate")
	key := fs.String("key", "", "approver operator private key (0600)")
	expect := fs.String("digest", "", "payload digest the operator confirmed; signing is refused if it differs")
	if err := fs.Parse(args); err != nil {
		return err
	}
	signer, err := approval.LoadSigner(*cert, *key)
	if err != nil {
		return err
	}
	var payload approval.Payload
	if err := readSignInput(&payload); err != nil {
		return err
	}
	if payload.ProposalID == "" || payload.Nonce == "" || payload.ExpiresAtMS == 0 {
		return errors.New("payload is incomplete; build it with `edgecert approval`")
	}
	digest, err := payload.Digest()
	if err != nil {
		return err
	}
	got := hex.EncodeToString(digest[:])
	if strings.TrimSpace(*expect) == "" {
		return errors.New("-digest is required: pass the digest the operator confirmed")
	}
	if !strings.EqualFold(strings.TrimSpace(*expect), got) {
		return fmt.Errorf("payload digest %s is not the confirmed digest %s; not signed", got, strings.TrimSpace(*expect))
	}
	signed, err := signer.Sign(payload)
	if err != nil {
		return err
	}
	return writeSignOutput(signedApprovalOutput{Approval: signed, Digest: got, Principal: signer.Principal.String()})
}
