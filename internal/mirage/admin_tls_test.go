package mirage

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/danmuck/edgectl/internal/adminauth"
	"github.com/danmuck/edgectl/internal/ghost"
	"github.com/danmuck/edgectl/internal/identity"
	"github.com/danmuck/edgectl/internal/protocol/session"
	"github.com/danmuck/edgectl/internal/testutil/testlog"
	"github.com/danmuck/edgectl/internal/testutil/tlstest"
)

// Stub Ghost admin endpoint over mTLS that answers status with reportedID.
func stubGhostTLS(t *testing.T, p *tlstest.PKI, certID string, reportedID string) string {
	t.Helper()
	cert, key, _ := p.Issue(t, identity.RoleGhost, certID)
	ln, err := adminauth.Listen("127.0.0.1:0", adminauth.Credentials{CertFile: cert, KeyFile: key, CAFile: p.CAFile()},
		identity.Principal{Role: identity.RoleGhost, ID: certID}, true)
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
			go func() {
				defer conn.Close()
				if _, err := adminauth.PeerFromConn(conn, 2*time.Second); err != nil {
					return
				}
				r := bufio.NewReader(conn)
				for {
					if _, err := r.ReadBytes('\n'); err != nil {
						return
					}
					data, _ := json.Marshal(map[string]any{"ghost_id": reportedID, "phase": "radiating"})
					resp, _ := json.Marshal(ghostControlResponse{OK: true, Data: data})
					if _, err := conn.Write(append(resp, '\n')); err != nil {
						return
					}
				}
			}()
		}
	}()
	return ln.Addr().String()
}

func tlsMirageService(t *testing.T, p *tlstest.PKI) *Service {
	t.Helper()
	cert, key, _ := p.Issue(t, identity.RoleMirage, "mirage.main")
	cfg := DefaultServiceConfig()
	cfg.MirageID = "mirage.main"
	cfg.LocalGhostAdminAddr = ""
	cfg.AdminTLS = adminauth.Credentials{CertFile: cert, KeyFile: key, CAFile: p.CAFile()}
	return NewServiceWithConfig(cfg)
}

func TestMirageGhostClientPinsGhostPrincipal(t *testing.T) {
	testlog.Start(t)
	p := tlstest.NewPKI(t)
	svc := tlsMirageService(t, p)
	addr := stubGhostTLS(t, p, "ghost.alpha", "ghost.alpha")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	client := svc.newGhostControlClient("ghost.alpha", addr)
	if _, err := client.Status(ctx); err != nil {
		t.Fatalf("pinned status: %v", err)
	}
	if got := client.PeerPrincipal().String(); got != "edgectl://ghost/ghost.alpha" {
		t.Fatalf("peer principal = %q", got)
	}
	if _, err := svc.newGhostControlClient("ghost.beta", addr).Status(ctx); !errors.Is(err, adminauth.ErrUnexpectedPeer) {
		t.Fatalf("wrong pin: expected ErrUnexpectedPeer, got %v", err)
	}
}

// attach without a known id learns it from the certificate and refuses a contradicting status.
func TestAttachGhostAdminTrustsCertificateOverStatus(t *testing.T) {
	testlog.Start(t)
	p := tlstest.NewPKI(t)
	svc := tlsMirageService(t, p)

	liar := stubGhostTLS(t, p, "ghost.liar", "ghost.victim")
	if _, err := svc.attachGhostAdmin("", liar); err == nil {
		t.Fatalf("attach must reject a ghost whose status contradicts its certificate")
	}
}

// Ghost session dialer: server must be a Mirage principal, the pinned one, and production requires a pin.
func TestGhostSessionPinsMiragePrincipal(t *testing.T) {
	testlog.Start(t)
	dir := t.TempDir()
	ca := tlstest.NewAuthority(t, dir, "edgectl-test-ca")
	ghCert, ghKey := ca.IssuePrincipalCert(t, dir, "edgectl://ghost/ghost.alpha", nil, nil)

	tryPin := func(addr string, pin string) error {
		sc := session.DefaultConfig()
		sc.SecurityMode = session.SecurityModeProduction
		sc.TLS = session.TLSConfig{Enabled: true, Mutual: true, CertFile: ghCert, KeyFile: ghKey, CAFile: ca.CAFile(), ServerName: "mirage.local"}
		client, err := ghost.NewMirageClient(ghost.MirageClientConfig{
			Address: addr, GhostID: "ghost.alpha", SeedList: []session.SeedInfo{}, Session: sc,
			MaxConnectAttempts: 1, MiragePrincipal: pin,
		})
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		gs, err := client.ConnectAndRegister(ctx)
		if err == nil {
			gs.Close()
		}
		return err
	}

	addr := startTLSSessionServer(t, dir, ca, nil)
	if err := tryPin(addr, "edgectl://mirage/mirage.local"); err != nil {
		t.Fatalf("correct pin: %v", err)
	}
	if err := tryPin(addr, "edgectl://mirage/mirage.other"); err == nil {
		t.Fatalf("wrong pin must fail")
	}
	if err := tryPin(addr, ""); !errors.Is(err, ghost.ErrMiragePrincipalRequired) {
		t.Fatalf("production without pin: expected ErrMiragePrincipalRequired, got %v", err)
	}
	if err := tryPin(addr, "edgectl://ghost/ghost.alpha"); !errors.Is(err, identity.ErrWrongRole) {
		t.Fatalf("non-mirage pin: expected ErrWrongRole, got %v", err)
	}
}
