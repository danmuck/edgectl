package main

// tls.go builds operator mTLS connections to admin endpoints (admin_security.toml [transport]).

import (
	"crypto/tls"
	"strings"

	"github.com/danmuck/edgectl/internal/adminauth"
	"github.com/danmuck/edgectl/internal/identity"
)

// Operator credentials from either config file; the first with a cert file wins.
func (a *App) operatorCredentials(preferMirage bool) (adminauth.Credentials, bool) {
	order := []operatorTLSConfig{a.ghostCfg.operatorTLSConfig, a.mirageCfg.operatorTLSConfig}
	if preferMirage {
		order[0], order[1] = order[1], order[0]
	}
	for _, op := range order {
		if strings.TrimSpace(op.OperatorCertFile) != "" {
			return adminauth.Credentials{
				CertFile: strings.TrimSpace(op.OperatorCertFile),
				KeyFile:  strings.TrimSpace(op.OperatorKeyFile),
				CAFile:   strings.TrimSpace(op.CAFile),
			}, true
		}
	}
	return adminauth.Credentials{}, false
}

// Operator dial config that pins the server to role/id (any id of role when id is empty);
// nil config means plaintext (no operator certificate configured).
func (a *App) operatorDialConfig(role identity.Role, id string, preferMirage bool) (*tls.Config, error) {
	creds, ok := a.operatorCredentials(preferMirage)
	if !ok {
		return nil, nil
	}
	self, err := adminauth.SelfFromCert(creds, identity.RoleOperator)
	if err != nil {
		return nil, err
	}
	check := adminauth.ExpectRole(role)
	if id = strings.TrimSpace(id); id != "" {
		check = adminauth.ExpectPrincipal(identity.Principal{Role: role, ID: id})
	}
	return adminauth.ClientTLSConfig(creds, self, check)
}

// Ghost admin client pinned to edgectl://ghost/<ghostID>.
func (a *App) newGhostAdmin(addr string, ghostID string) *RemoteGhostAdmin {
	admin := NewRemoteGhostAdmin(addr)
	admin.tlsConfig, admin.tlsErr = a.operatorDialConfig(identity.RoleGhost, ghostID, false)
	return admin
}

// Mirage admin client pinned to edgectl://mirage/<mirageID>.
func (a *App) newMirageAdmin(addr string, mirageID string) *RemoteMirageAdmin {
	admin := NewRemoteMirageAdmin(addr)
	admin.tlsConfig, admin.tlsErr = a.operatorDialConfig(identity.RoleMirage, mirageID, true)
	return admin
}
