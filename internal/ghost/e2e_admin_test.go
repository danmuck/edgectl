package ghost_test

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/danmuck/edgectl/internal/adminauth"
	"github.com/danmuck/edgectl/internal/approval"
	"github.com/danmuck/edgectl/internal/ghost"
	"github.com/danmuck/edgectl/internal/identity"
	"github.com/danmuck/edgectl/internal/mirage"
	"github.com/danmuck/edgectl/internal/protocol/schema"
	"github.com/danmuck/edgectl/internal/testutil/testlog"
	"github.com/danmuck/edgectl/internal/testutil/tlstest"
)

// Records installer commands; every command succeeds.
type recordingRunner struct {
	mu   sync.Mutex
	cmds []string
}

func (r *recordingRunner) Run(name string, args ...string) ([]byte, []byte, uint32, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cmds = append(r.cmds, strings.Join(append([]string{name}, args...), " "))
	return nil, nil, 0, nil
}

func (r *recordingRunner) ran(cmd string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.cmds {
		if c == cmd {
			return true
		}
	}
	return false
}

type adminResp struct {
	OK        bool            `json:"ok"`
	Error     string          `json:"error"`
	ErrorCode uint32          `json:"error_code"`
	Data      json.RawMessage `json:"data"`
}

// One JSON request/response over an admin connection.
func call(t *testing.T, addr string, cfg *tls.Config, req map[string]any) adminResp {
	t.Helper()
	conn, err := adminauth.Dial(context.Background(), addr, cfg, 2*time.Second)
	if err != nil {
		t.Fatalf("dial %s: %v", addr, err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	line, _ := json.Marshal(req)
	if _, err := conn.Write(append(line, '\n')); err != nil {
		t.Fatal(err)
	}
	raw, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var resp adminResp
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatal(err)
	}
	return resp
}

func clientFor(t *testing.T, p *tlstest.PKI, cert, key string, self, server identity.Principal) *tls.Config {
	t.Helper()
	cfg, err := adminauth.ClientTLSConfig(adminauth.Credentials{CertFile: cert, KeyFile: key, CAFile: p.CAFile()}, self, adminauth.ExpectPrincipal(server))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

// Slice F end-to-end over real mTLS sockets for both admin endpoints (admin_security.toml): two
// operators propose and approve a catalog install on Mirage; Mirage dials the pinned Ghost over mTLS;
// the Ghost verifies both signatures and installs. Separation of duties, operator access to the Ghost,
// and replays are refused across the real boundary.
func TestAdminSecurityEndToEnd(t *testing.T) {
	testlog.Start(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	p := tlstest.NewPKI(t)
	ghostP := identity.Principal{Role: identity.RoleGhost, ID: "ghost.alpha"}
	mirageP := identity.Principal{Role: identity.RoleMirage, ID: "mirage.main"}
	gCert, gKey, _ := p.Issue(t, ghostP.Role, ghostP.ID)
	mCert, mKey, _ := p.Issue(t, mirageP.Role, mirageP.ID)
	opsCert, opsKey, _ := p.Issue(t, identity.RoleOperator, "ops")
	leadCert, leadKey, _ := p.Issue(t, identity.RoleOperator, "lead")
	ops, err := approval.LoadSigner(opsCert, opsKey)
	if err != nil {
		t.Fatal(err)
	}
	lead, err := approval.LoadSigner(leadCert, leadKey)
	if err != nil {
		t.Fatal(err)
	}

	// Ghost: mTLS admin endpoint, pinned Mirage, installs enabled for seed.docker, installer recorded.
	gCfg := ghost.DefaultServiceConfig()
	gCfg.GhostID = ghostP.ID
	gCfg.AdminTLS = adminauth.Credentials{CertFile: gCert, KeyFile: gKey, CAFile: p.CAFile()}
	gCfg.MiragePrincipal = mirageP.String()
	workspace := t.TempDir()
	gCfg.SeedInstall = ghost.SeedInstallConfig{
		Enabled: true, WorkspaceRoot: workspace, InstallRoot: filepath.Join("local", "seeds"),
		BinRoot: filepath.Join("local", "bin"), Whitelist: []string{"seed.docker"},
	}
	gSvc := ghost.NewServiceWithConfig(gCfg)
	runner := &recordingRunner{}
	ghost.SetInstallRunnerForTest(gSvc, runner)
	if err := ghost.BootstrapForTest(gSvc); err != nil {
		t.Fatalf("ghost bootstrap: %v", err)
	}
	gLn, err := adminauth.Listen("127.0.0.1:0", gCfg.AdminTLS, ghostP, true)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = gSvc.ServeAdmin(ctx, gLn) }()
	ghostAddr := gLn.Addr().String()

	// Mirage: mTLS admin endpoint with the same root.
	mCfg := mirage.DefaultServiceConfig()
	mCfg.MirageID = mirageP.ID
	mCfg.LocalGhostAdminAddr = ""
	mCfg.AdminTLS = adminauth.Credentials{CertFile: mCert, KeyFile: mKey, CAFile: p.CAFile()}
	mSvc := mirage.NewServiceWithConfig(mCfg)
	mLn, err := adminauth.Listen("127.0.0.1:0", mCfg.AdminTLS, mirageP, true)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = mSvc.ServeAdmin(ctx, mLn) }()
	mirageAddr := mLn.Addr().String()

	opsMirage := clientFor(t, p, opsCert, opsKey, ops.Principal, mirageP)
	leadMirage := clientFor(t, p, leadCert, leadKey, lead.Principal, mirageP)

	// Attach: Mirage dials the Ghost over mTLS and learns its id from the certificate.
	if resp := call(t, mirageAddr, opsMirage, map[string]any{"action": "attach_ghost_admin", "ghost_admin_addr": ghostAddr}); !resp.OK {
		t.Fatalf("attach: %s", resp.Error)
	}

	// Propose: ops signs an intent; Mirage derives steps from its catalog.
	intent, err := ops.SignIntent(approval.Intent{GhostID: ghostP.ID, SeedIDs: []string{"seed.docker"}, ProposedAtMS: uint64(time.Now().UnixMilli()), Nonce: "e2e-intent"})
	if err != nil {
		t.Fatal(err)
	}
	resp := call(t, mirageAddr, opsMirage, map[string]any{"action": "propose_dependencies", "dependency": map[string]any{"intent": intent}})
	if !resp.OK {
		t.Fatalf("propose: %s", resp.Error)
	}
	var proposal mirage.DependencyProposal
	if err := json.Unmarshal(resp.Data, &proposal); err != nil {
		t.Fatal(err)
	}
	if proposal.ProposedBy != "edgectl://operator/ops" || len(proposal.Steps) == 0 {
		t.Fatalf("proposal: %+v", proposal)
	}

	sign := func(s *approval.Signer, nonce string) approval.Signed {
		signed, err := s.Sign(approval.Payload{
			ProposalID: proposal.ID, GhostID: proposal.GhostID, ProposerIntent: proposal.Intent, Steps: proposal.Steps,
			ExpiresAtMS: uint64(time.Now().Add(10 * time.Minute).UnixMilli()), Nonce: nonce,
		})
		if err != nil {
			t.Fatal(err)
		}
		return signed
	}
	approve := func(cfg *tls.Config, signed approval.Signed) adminResp {
		return call(t, mirageAddr, cfg, map[string]any{"action": "approve_dependencies", "dependency": map[string]any{"proposal_id": proposal.ID, "approval": signed}})
	}

	// The proposer cannot approve their own proposal.
	if resp := approve(opsMirage, sign(ops, "self")); resp.OK || resp.ErrorCode != schema.ErrCodeSemanticValidation {
		t.Fatalf("self approval: ok=%v code=%d %s", resp.OK, resp.ErrorCode, resp.Error)
	}
	// An approval signed by lead but submitted by ops is not attributed to ops.
	if resp := approve(opsMirage, sign(lead, "relay")); resp.OK || resp.ErrorCode != schema.ErrCodeSemanticValidation {
		t.Fatalf("relayed approval: ok=%v code=%d %s", resp.OK, resp.ErrorCode, resp.Error)
	}
	if resp := approve(leadMirage, sign(lead, "e2e-approval")); !resp.OK {
		t.Fatalf("approve: %s", resp.Error)
	}

	// Execute: Mirage dispatches to the pinned Ghost over mTLS; the Ghost verifies and installs.
	resp = call(t, mirageAddr, leadMirage, map[string]any{"action": "execute_dependencies", "dependency": map[string]any{"proposal_id": proposal.ID}})
	if !resp.OK {
		t.Fatalf("execute: %s", resp.Error)
	}
	var done mirage.DependencyProposal
	if err := json.Unmarshal(resp.Data, &done); err != nil {
		t.Fatal(err)
	}
	if done.State != mirage.ProposalSucceeded {
		t.Fatalf("state = %s execution=%+v", done.State, done.Execution)
	}
	if !runner.ran("brew install docker") {
		t.Fatalf("ghost did not run the catalog install; ran %v", runner.cmds)
	}

	// Re-executing the closed proposal is refused by Mirage.
	if resp := call(t, mirageAddr, leadMirage, map[string]any{"action": "execute_dependencies", "dependency": map[string]any{"proposal_id": proposal.ID}}); resp.OK {
		t.Fatalf("re-execute must be refused")
	}

	// Operators cannot reach the Ghost's install action directly (operator access off; mirage-only).
	opsGhost := clientFor(t, p, opsCert, opsKey, ops.Principal, ghostP)
	replay := done.Approval
	if resp := call(t, ghostAddr, opsGhost, map[string]any{"action": "install_dependencies", "install": map[string]any{"proposal_id": proposal.ID, "approval": replay}}); resp.OK || resp.ErrorCode != schema.ErrCodeTransportFailure {
		t.Fatalf("operator install on ghost: ok=%v code=%d %s", resp.OK, resp.ErrorCode, resp.Error)
	}

	// Even the pinned Mirage cannot replay the consumed approval.
	mirageGhost := clientFor(t, p, mCert, mKey, mirageP, ghostP)
	if resp := call(t, ghostAddr, mirageGhost, map[string]any{"action": "install_dependencies", "install": map[string]any{"proposal_id": proposal.ID, "approval": replay}}); resp.OK || resp.ErrorCode != schema.ErrCodeSemanticValidation {
		t.Fatalf("replayed approval: ok=%v code=%d %s", resp.OK, resp.ErrorCode, resp.Error)
	}
}
