package mirage

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/danmuck/edgectl/internal/adminauth"
	"github.com/danmuck/edgectl/internal/identity"
	"github.com/danmuck/edgectl/internal/protocol/schema"
	"github.com/danmuck/edgectl/internal/testutil/testlog"
	"github.com/danmuck/edgectl/internal/testutil/tlstest"
)

// Raw client presenting cert/key (if any) without client-side identity checks, so the test observes
// only what the server enforces. Test-only: production clients verify and pin the server.
func rawAdminClient(t *testing.T, certFile, keyFile string) *tls.Config {
	t.Helper()
	cfg := &tls.Config{MinVersion: tls.VersionTLS13, InsecureSkipVerify: true}
	if certFile != "" {
		pair, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			t.Fatal(err)
		}
		cfg.Certificates = []tls.Certificate{pair}
	}
	return cfg
}

// Like mirageAdminCall but returns transport errors instead of failing the test.
func tryMirageAdminCall(addr string, cfg *tls.Config, req adminControlRequest) (adminControlResponse, error) {
	conn, err := adminauth.Dial(context.Background(), addr, cfg, 2*time.Second)
	if err != nil {
		return adminControlResponse{}, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	line, _ := json.Marshal(req)
	if _, err := conn.Write(append(line, '\n')); err != nil {
		return adminControlResponse{}, err
	}
	respLine, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil {
		return adminControlResponse{}, err
	}
	var resp adminControlResponse
	err = json.Unmarshal(respLine, &resp)
	return resp, err
}

func issueFor(intentID string) AdminIssueRequest {
	return AdminIssueRequest{
		IntentID: intentID, TargetScope: "ghost:ghost.alpha", Objective: "status",
		Stages: []AdminIssueStage{{ID: "s1", Commands: []AdminIssueCommand{{GhostID: "ghost.alpha", SeedSelector: "seed.flow", Operation: "status"}}}},
	}
}

// Slice F negative matrix for the Mirage admin endpoint (admin_security.toml [transport],
// [authorization.mirage_admin], [trust], [identity]): every case submits an issue and must be
// refused without recording it.
func TestMirageAdminEndpointNegativeMatrix(t *testing.T) {
	testlog.Start(t)
	p := tlstest.NewPKI(t)
	mCert, mKey, _ := p.Issue(t, identity.RoleMirage, "mirage.main")
	opCert, opKey, _ := p.Issue(t, identity.RoleOperator, "dan")
	ghCert, ghKey, _ := p.Issue(t, identity.RoleGhost, "ghost.alpha")
	peerMirage, peerMirageKey, _ := p.Issue(t, identity.RoleMirage, "mirage.other")
	revoked, revokedKey, revokedSerial := p.Issue(t, identity.RoleOperator, "revoked")
	expired, expiredKey := p.IssueExpired(t, identity.RoleOperator, "expired")
	multi, multiKey := p.IssueMultiURI(t, "multi",
		identity.Principal{Role: identity.RoleOperator, ID: "dan"}, identity.Principal{Role: identity.RoleGhost, ID: "ghost.alpha"})
	foreign := tlstest.NewPKI(t)
	fCert, fKey, _ := foreign.Issue(t, identity.RoleOperator, "dan")

	cfg := DefaultServiceConfig()
	cfg.MirageID = "mirage.main"
	cfg.LocalGhostAdminAddr = ""
	cfg.AdminTLS = adminauth.Credentials{CertFile: mCert, KeyFile: mKey, CAFile: p.CAFile(), DeniedSerials: []string{revokedSerial}}
	svc := NewServiceWithConfig(cfg)
	addr := serveMirageAdmin(t, svc)

	cases := []struct {
		name string
		cfg  *tls.Config
	}{
		{"no client certificate", rawAdminClient(t, "", "")},
		{"wrong CA", rawAdminClient(t, fCert, fKey)},
		{"expired", rawAdminClient(t, expired, expiredKey)},
		{"revoked serial", rawAdminClient(t, revoked, revokedKey)},
		{"wrong role (ghost)", rawAdminClient(t, ghCert, ghKey)},
		{"wrong role (mirage)", rawAdminClient(t, peerMirage, peerMirageKey)},
		{"multiple URI SANs", rawAdminClient(t, multi, multiKey)},
		{"plaintext client", nil},
	}
	for i, c := range cases {
		id := "intent.neg." + string(rune('a'+i))
		resp, err := tryMirageAdminCall(addr, c.cfg, adminControlRequest{Action: "submit_issue", Issue: issueFor(id)})
		if err == nil && (resp.OK || resp.ErrorCode != schema.ErrCodeTransportFailure) {
			t.Errorf("%s: not refused: ok=%v code=%d %s", c.name, resp.OK, resp.ErrorCode, resp.Error)
		}
		if _, ok := svc.server.SnapshotIntent(id); ok {
			t.Errorf("%s: refused issue was recorded", c.name)
		}
		// Privileged actions are refused the same way.
		resp, err = tryMirageAdminCall(addr, c.cfg, adminControlRequest{Action: "deploy_ghost", Deploy: DeployGhostRequest{GhostID: "ghost.alpha"}})
		if err == nil && (resp.OK || resp.ErrorCode != schema.ErrCodeTransportFailure) {
			t.Errorf("%s deploy_ghost: not refused: ok=%v code=%d %s", c.name, resp.OK, resp.ErrorCode, resp.Error)
		}
	}

	// Control: an operator submits over the same listener.
	resp, err := tryMirageAdminCall(addr, rawAdminClient(t, opCert, opKey), adminControlRequest{Action: "submit_issue", Issue: issueFor("intent.neg.ok")})
	if err != nil || !resp.OK {
		t.Fatalf("operator: err=%v %s", err, resp.Error)
	}

	// Plaintext on a non-loopback address never listens, in any mode.
	plainCfg := DefaultServiceConfig()
	plainCfg.LocalGhostAdminAddr = ""
	plain := NewServiceWithConfig(plainCfg)
	if err := plain.serveAdminControl(context.Background(), "0.0.0.0:0"); !errors.Is(err, adminauth.ErrPlaintextNotAllowed) {
		t.Fatalf("plaintext non-loopback: expected ErrPlaintextNotAllowed, got %v", err)
	}
}
