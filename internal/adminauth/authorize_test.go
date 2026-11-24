package adminauth

import (
	"errors"
	"testing"

	"github.com/danmuck/edgectl/internal/identity"
)

func authed(role identity.Role, id string) Peer {
	return Peer{Principal: identity.Principal{Role: role, ID: id}, Authenticated: true}
}

func TestAuthorizeMirageEndpointOperatorsOnly(t *testing.T) {
	p := Policy{Endpoint: MirageAdmin}
	if err := p.Authorize(authed(identity.RoleOperator, "dan"), "deploy_ghost"); err != nil {
		t.Fatalf("operator deploy: %v", err)
	}
	for _, peer := range []Peer{authed(identity.RoleGhost, "ghost.alpha"), authed(identity.RoleMirage, "mirage.other")} {
		for _, action := range []string{"deploy_ghost", "approve_dependencies", "submit_issue", "status"} {
			if err := p.Authorize(peer, action); !errors.Is(err, ErrAuthorizationDenied) {
				t.Errorf("%s %s: expected ErrAuthorizationDenied, got %v", peer.Principal, action, err)
			}
		}
	}
	if err := p.Authorize(authed(identity.RoleOperator, "dan"), "drop_tables"); !errors.Is(err, ErrAuthorizationDenied) {
		t.Fatalf("unknown action: expected default deny, got %v", err)
	}
}

func TestAuthorizeGhostEndpointPinnedMirageAndOperatorFlag(t *testing.T) {
	p := Policy{Endpoint: GhostAdmin, PinnedMirage: "edgectl://mirage/mirage.main"}
	if err := p.Authorize(authed(identity.RoleMirage, "mirage.main"), "execute_envelope"); err != nil {
		t.Fatalf("pinned mirage execute: %v", err)
	}
	cases := []struct {
		peer   Peer
		action string
	}{
		{authed(identity.RoleMirage, "mirage.other"), "execute_envelope"},
		{authed(identity.RoleGhost, "ghost.beta"), "execute_envelope"},
		{authed(identity.RoleGhost, "ghost.beta"), "install_dependencies"},
		{authed(identity.RoleGhost, "ghost.beta"), "status"},
		{authed(identity.RoleOperator, "dan"), "status"},
	}
	for _, c := range cases {
		if err := p.Authorize(c.peer, c.action); !errors.Is(err, ErrAuthorizationDenied) {
			t.Errorf("%s %s: expected ErrAuthorizationDenied, got %v", c.peer.Principal, c.action, err)
		}
	}

	withOps := Policy{Endpoint: GhostAdmin, PinnedMirage: p.PinnedMirage, OperatorAccess: true}
	if err := withOps.Authorize(authed(identity.RoleOperator, "dan"), "status"); err != nil {
		t.Fatalf("operator status with access: %v", err)
	}
	if err := withOps.Authorize(authed(identity.RoleOperator, "dan"), "execute_envelope"); !errors.Is(err, ErrAuthorizationDenied) {
		t.Fatalf("operator execute is mirage-only: expected deny, got %v", err)
	}

	noPin := Policy{Endpoint: GhostAdmin}
	if err := noPin.Authorize(authed(identity.RoleMirage, "mirage.main"), "status"); !errors.Is(err, ErrAuthorizationDenied) {
		t.Fatalf("no pin means no mirage may act: got %v", err)
	}
}

func TestAuthorizeUnauthenticatedPeers(t *testing.T) {
	p := Policy{Endpoint: GhostAdmin}
	if err := p.Authorize(Peer{}, "status"); err != nil {
		t.Fatalf("loopback status: %v", err)
	}
	if err := p.Authorize(Peer{}, "execute_envelope"); !errors.Is(err, ErrAuthRequired) {
		t.Fatalf("loopback privileged: expected ErrAuthRequired, got %v", err)
	}
	if err := p.Authorize(Peer{}, "nope"); !errors.Is(err, ErrAuthorizationDenied) {
		t.Fatalf("loopback unknown action: expected deny, got %v", err)
	}
}

func TestBindActor(t *testing.T) {
	op := authed(identity.RoleOperator, "dan")
	if got, err := BindActor(op, ""); err != nil || got != "edgectl://operator/dan" {
		t.Fatalf("empty actor: %q %v", got, err)
	}
	if got, err := BindActor(op, "edgectl://operator/dan"); err != nil || got != "edgectl://operator/dan" {
		t.Fatalf("matching actor: %q %v", got, err)
	}
	if _, err := BindActor(op, "user:someone-else"); !errors.Is(err, ErrAuthorizationDenied) {
		t.Fatalf("spoofed actor: expected deny, got %v", err)
	}
	if got, err := BindActor(Peer{}, "user:dev"); err != nil || got != "user:dev" {
		t.Fatalf("loopback actor kept: %q %v", got, err)
	}
}
