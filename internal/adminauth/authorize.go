package adminauth

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/danmuck/edgectl/internal/identity"
)

// ErrAuthorizationDenied reports an action the caller's principal may not perform
// (admin_security.toml [errors]: 1000 with reason authorization_denied).
var ErrAuthorizationDenied = errors.New("adminauth: authorization_denied")

// Endpoint names one admin boundary with its own action table.
type Endpoint string

const (
	MirageAdmin Endpoint = "mirage_admin"
	GhostAdmin  Endpoint = "ghost_admin"
)

var (
	operatorOnly   = []identity.Role{identity.RoleOperator}
	mirageOnly     = []identity.Role{identity.RoleMirage}
	mirageOperator = []identity.Role{identity.RoleMirage, identity.RoleOperator}
)

// actionRoles is the default-deny action table (admin_security.toml [authorization.*]).
var actionRoles = map[Endpoint]map[string][]identity.Role{
	MirageAdmin: {
		"status":                    operatorOnly,
		"list_intents":              operatorOnly,
		"snapshot_intent":           operatorOnly,
		"recent_reports":            operatorOnly,
		"registered_ghosts":         operatorOnly,
		"routing_table":             operatorOnly,
		"available_services":        operatorOnly,
		"seed_catalog":              operatorOnly,
		"list_dependency_proposals": operatorOnly,
		"submit_issue":              operatorOnly,
		"reconcile_intent":          operatorOnly,
		"reconcile_all":             operatorOnly,
		"attach_ghost_admin":        operatorOnly,
		"spawn_local_ghost":         operatorOnly,
		"deploy_ghost":              operatorOnly,
		"propose_dependencies":      operatorOnly,
		"approve_dependencies":      operatorOnly,
		"execute_dependencies":      operatorOnly,
	},
	GhostAdmin: {
		"status":                  mirageOperator,
		"list_seeds":              mirageOperator,
		"list_seed_catalog":       mirageOperator,
		"recent_events":           mirageOperator,
		"verification":            mirageOperator,
		"execution_by_command_id": mirageOperator,
		"dependency_status":       mirageOperator,
		"execute_envelope":        mirageOnly,
		"spawn_ghost":             mirageOnly,
		"bind_mirage":             mirageOnly,
		"install_dependencies":    mirageOnly,
	},
}

// Policy authorizes admin requests for one endpoint.
type Policy struct {
	Endpoint Endpoint
	// Ghost endpoint: the only principal holding the mirage role; empty means no Mirage may act.
	PinnedMirage string
	// Ghost endpoint: whether operator principals are honored (off by default).
	OperatorAccess bool
}

// Authorize applies default deny: the action must be in the endpoint table; unauthenticated peers
// (development loopback only) may run listed non-privileged actions; authenticated peers need a
// listed role, with Ghost-endpoint mirage limited to the pinned principal and operator gated by
// OperatorAccess.
func (p Policy) Authorize(peer Peer, action string) error {
	action = strings.TrimSpace(action)
	allowed, ok := actionRoles[p.Endpoint][action]
	if !ok {
		return fmt.Errorf("%w: unknown action %q", ErrAuthorizationDenied, action)
	}
	if !peer.Authenticated {
		if Privileged(action) {
			return fmt.Errorf("%w: %s", ErrAuthRequired, action)
		}
		return nil
	}
	role := peer.Principal.Role
	if !roleListed(allowed, role) {
		return fmt.Errorf("%w: %s may not %s", ErrAuthorizationDenied, peer.Principal, action)
	}
	if p.Endpoint == GhostAdmin {
		switch role {
		case identity.RoleMirage:
			if pin := strings.TrimSpace(p.PinnedMirage); pin == "" || peer.Principal.String() != pin {
				return fmt.Errorf("%w: %s is not the pinned mirage", ErrAuthorizationDenied, peer.Principal)
			}
		case identity.RoleOperator:
			if !p.OperatorAccess {
				return fmt.Errorf("%w: operator access disabled on this ghost", ErrAuthorizationDenied)
			}
		}
	}
	return nil
}

func roleListed(roles []identity.Role, role identity.Role) bool {
	for _, r := range roles {
		if r == role {
			return true
		}
	}
	return false
}

// BindActor enforces actor_source: an empty actor becomes the caller principal; a non-empty actor
// must equal it. Unauthenticated (development loopback) peers keep the supplied actor.
func BindActor(peer Peer, actor string) (string, error) {
	actor = strings.TrimSpace(actor)
	if !peer.Authenticated {
		return actor, nil
	}
	self := peer.Principal.String()
	if actor == "" {
		return self, nil
	}
	if actor != self {
		return "", fmt.Errorf("%w: actor %q is not the caller %s", ErrAuthorizationDenied, actor, self)
	}
	return self, nil
}

// Decision values for admin audit lines.
const (
	DecisionAllow = "allow"
	DecisionDeny  = "deny"
)

// Audit writes one admin request decision with admin_security.toml [audit] admin_request_fields.
// The record is written regardless of the configured log level (see Record).
func Audit(component string, peer Peer, action string, decision string, errorCode uint32) {
	principal, role := "unauthenticated", "none"
	if peer.Authenticated {
		principal, role = peer.Principal.String(), string(peer.Principal.Role)
	}
	line := fmt.Sprintf(
		"admin.audit component=%s peer=%q principal=%q role=%s action=%q decision=%s error_code=%d timestamp_ms=%d",
		component, peer.Addr, principal, role, strings.TrimSpace(action), decision, errorCode, time.Now().UnixMilli(),
	)
	level := AuditInfo
	if decision == DecisionDeny {
		level = AuditWarn
	}
	_ = Record(level, line) // Record reports its own write failure on stderr.
}
