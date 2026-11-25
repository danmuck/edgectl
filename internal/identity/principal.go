package identity

import (
	"crypto/x509"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var (
	// Certificate carries no URI SAN principal.
	ErrNoPrincipal = errors.New("identity: certificate has no principal URI SAN")
	// Certificate carries more than one URI SAN; the principal would be ambiguous.
	ErrMultiplePrincipals = errors.New("identity: certificate has multiple URI SANs")
	// URI is not a well-formed edgectl://<role>/<id> principal.
	ErrInvalidPrincipal = errors.New("identity: invalid principal")
	// Principal role differs from the role the boundary requires.
	ErrWrongRole = errors.New("identity: principal has wrong role")
)

// Role is the authorization role encoded in a principal (admin_security.toml [identity]).
type Role string

const (
	RoleOperator Role = "operator"
	RoleMirage   Role = "mirage"
	RoleGhost    Role = "ghost"
)

// PrincipalScheme is the URI scheme of every edgectl principal.
const PrincipalScheme = "edgectl"

var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,62}$`)

// Principal is one authenticated identity: edgectl://<role>/<id>.
type Principal struct {
	Role Role
	ID   string
}

// String renders the canonical principal URI.
func (p Principal) String() string {
	return PrincipalScheme + "://" + string(p.Role) + "/" + p.ID
}

// Validate checks role membership and id charset.
func (p Principal) Validate() error {
	switch p.Role {
	case RoleOperator, RoleMirage, RoleGhost:
	default:
		return fmt.Errorf("%w: unknown role %q", ErrInvalidPrincipal, p.Role)
	}
	if !idPattern.MatchString(p.ID) {
		return fmt.Errorf("%w: id %q must match %s", ErrInvalidPrincipal, p.ID, idPattern.String())
	}
	return nil
}

// ParsePrincipal parses and validates an edgectl://<role>/<id> URI. Userinfo, port, query,
// fragment, and nested paths are rejected so one principal has exactly one spelling.
func ParsePrincipal(raw string) (Principal, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return Principal{}, fmt.Errorf("%w: %v", ErrInvalidPrincipal, err)
	}
	return principalFromURL(u)
}

func principalFromURL(u *url.URL) (Principal, error) {
	if u == nil || u.Scheme != PrincipalScheme {
		return Principal{}, fmt.Errorf("%w: scheme must be %q", ErrInvalidPrincipal, PrincipalScheme)
	}
	if u.User != nil || u.Port() != "" || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return Principal{}, fmt.Errorf("%w: %q has extra components", ErrInvalidPrincipal, u.String())
	}
	id := strings.TrimPrefix(u.Path, "/")
	if strings.Contains(id, "/") {
		return Principal{}, fmt.Errorf("%w: %q has a nested path", ErrInvalidPrincipal, u.String())
	}
	p := Principal{Role: Role(u.Hostname()), ID: id}
	if err := p.Validate(); err != nil {
		return Principal{}, err
	}
	return p, nil
}

// PrincipalFromCert reads the principal from exactly one URI SAN. Common name is ignored.
func PrincipalFromCert(cert *x509.Certificate) (Principal, error) {
	if cert == nil || len(cert.URIs) == 0 {
		return Principal{}, ErrNoPrincipal
	}
	if len(cert.URIs) > 1 {
		return Principal{}, ErrMultiplePrincipals
	}
	return principalFromURL(cert.URIs[0])
}

// RequireRole returns the certificate principal only if it has the given role.
func RequireRole(cert *x509.Certificate, role Role) (Principal, error) {
	p, err := PrincipalFromCert(cert)
	if err != nil {
		return Principal{}, err
	}
	if p.Role != role {
		return Principal{}, fmt.Errorf("%w: %s, want %s", ErrWrongRole, p.Role, role)
	}
	return p, nil
}
