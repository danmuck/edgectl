package ghost

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
	"github.com/danmuck/edgectl/internal/protocol/session"
	"github.com/danmuck/edgectl/internal/testutil/testlog"
	"github.com/danmuck/edgectl/internal/testutil/tlstest"
)

// Serves svc's admin handler on a listener built by adminauth.Listen; returns the address.
func serveAdmin(t *testing.T, svc *Service, production bool) string {
	t.Helper()
	ln, err := adminauth.Listen("127.0.0.1:0", svc.cfg.AdminTLS, svc.selfPrincipal(), production)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go svc.handleAdminConn(conn)
		}
	}()
	return ln.Addr().String()
}

// One JSON request/response over the admin protocol.
func adminCall(t *testing.T, addr string, cfg *tls.Config, req controlRequest) (controlResponse, error) {
	t.Helper()
	conn, err := adminauth.Dial(context.Background(), addr, cfg, 2*time.Second)
	if err != nil {
		return controlResponse{}, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	line, _ := json.Marshal(req)
	if _, err := conn.Write(append(line, '\n')); err != nil {
		return controlResponse{}, err
	}
	respLine, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil {
		return controlResponse{}, err
	}
	var resp controlResponse
	err = json.Unmarshal(respLine, &resp)
	return resp, err
}

func flowCommandFrame(t *testing.T, commandID string) []byte {
	t.Helper()
	raw, err := session.EncodeCommandFrame(1, session.Command{
		CommandID: commandID, IntentID: "intent.tls", GhostID: "ghost.alpha", SeedSelector: "seed.flow", Operation: "status",
	})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// Plaintext loopback serves read actions but refuses privileged ones with code 1000.
func TestGhostAdminPlaintextRefusesPrivilegedActions(t *testing.T) {
	testlog.Start(t)
	svc := NewServiceWithConfig(DefaultServiceConfig())
	svc.server = newRadiatingServer(t, "ghost.alpha")
	addr := serveAdmin(t, svc, false)

	resp, err := adminCall(t, addr, nil, controlRequest{Action: "status"})
	if err != nil || !resp.OK {
		t.Fatalf("status over loopback plaintext: ok=%v err=%v %s", resp.OK, err, resp.Error)
	}
	resp, err = adminCall(t, addr, nil, controlRequest{Action: "execute_envelope", CommandFrame: flowCommandFrame(t, "cmd.plain")})
	if err != nil {
		t.Fatal(err)
	}
	if resp.OK || resp.ErrorCode != schema.ErrCodeTransportFailure {
		t.Fatalf("privileged over plaintext: ok=%v code=%d err=%s", resp.OK, resp.ErrorCode, resp.Error)
	}
	if _, ok := svc.server.ExecutionByCommandID("cmd.plain"); ok {
		t.Fatalf("refused command must not execute")
	}
}

// Over mTLS a Mirage principal may run privileged actions; the Ghost serves as its own principal.
func TestGhostAdminMutualTLSAllowsPrivilegedActions(t *testing.T) {
	testlog.Start(t)
	p := tlstest.NewPKI(t)
	gCert, gKey, _ := p.Issue(t, identity.RoleGhost, "ghost.alpha")
	mCert, mKey, _ := p.Issue(t, identity.RoleMirage, "mirage.main")

	cfg := DefaultServiceConfig()
	cfg.GhostID = "ghost.alpha"
	cfg.AdminTLS = adminauth.Credentials{CertFile: gCert, KeyFile: gKey, CAFile: p.CAFile()}
	svc := NewServiceWithConfig(cfg)
	svc.server = newRadiatingServer(t, "ghost.alpha")
	addr := serveAdmin(t, svc, true)

	clientCfg, err := adminauth.ClientTLSConfig(adminauth.Credentials{CertFile: mCert, KeyFile: mKey, CAFile: p.CAFile()},
		identity.Principal{Role: identity.RoleMirage, ID: "mirage.main"},
		adminauth.ExpectPrincipal(identity.Principal{Role: identity.RoleGhost, ID: "ghost.alpha"}))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := adminCall(t, addr, clientCfg, controlRequest{Action: "execute_envelope", CommandFrame: flowCommandFrame(t, "cmd.tls")})
	if err != nil || !resp.OK {
		t.Fatalf("execute over mTLS: ok=%v err=%v %s", resp.OK, err, resp.Error)
	}
	if _, ok := svc.server.ExecutionByCommandID("cmd.tls"); !ok {
		t.Fatalf("command did not execute")
	}

	// Plaintext clients cannot talk to a TLS listener at all.
	if _, err := adminCall(t, addr, nil, controlRequest{Action: "status"}); err == nil {
		t.Fatalf("plaintext client must not get a response from the TLS listener")
	}
}

// Children of a TLS parent need operator-issued certificates; plaintext parents only spawn on loopback.
func TestChildAdminTLSCredentials(t *testing.T) {
	testlog.Start(t)
	p := tlstest.NewPKI(t)
	gCert, gKey, _ := p.Issue(t, identity.RoleGhost, "ghost.alpha")
	cfg := DefaultServiceConfig()
	cfg.GhostID = "ghost.alpha"
	cfg.AdminTLS = adminauth.Credentials{CertFile: gCert, KeyFile: gKey, CAFile: p.CAFile()}
	svc := NewServiceWithConfig(cfg)

	if _, err := svc.childAdminTLS("ghost.alpha.edge.1", "127.0.0.1:0"); err == nil {
		t.Fatalf("child without an issued certificate must be refused")
	}
	p.Issue(t, identity.RoleGhost, "ghost.alpha.edge.1")
	child, err := svc.childAdminTLS("ghost.alpha.edge.1", "127.0.0.1:0")
	if err != nil || !child.Enabled() || child.CAFile != p.CAFile() {
		t.Fatalf("child credentials: %+v err=%v", child, err)
	}

	plain := NewServiceWithConfig(DefaultServiceConfig())
	if _, err := plain.childAdminTLS("ghost.local.x", "0.0.0.0:7999"); !errors.Is(err, adminauth.ErrPlaintextNotAllowed) {
		t.Fatalf("plaintext child on all interfaces: expected ErrPlaintextNotAllowed, got %v", err)
	}
	if _, err := plain.childAdminTLS("ghost.local.x", "127.0.0.1:7999"); err != nil {
		t.Fatalf("plaintext loopback child: %v", err)
	}
}
