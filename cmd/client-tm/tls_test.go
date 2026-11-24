package main

import (
	"errors"
	"testing"

	"github.com/danmuck/edgectl/internal/identity"
	"github.com/danmuck/edgectl/internal/testutil/tlstest"
)

// Operator credentials must carry an operator principal; dial configs pin the target role/id.
func TestOperatorDialConfig(t *testing.T) {
	p := tlstest.NewPKI(t)
	opCert, opKey, _ := p.Issue(t, identity.RoleOperator, "dan")
	ghCert, ghKey, _ := p.Issue(t, identity.RoleGhost, "ghost.alpha")

	a := &App{}
	if cfg, err := a.operatorDialConfig(identity.RoleGhost, "ghost.alpha", false); cfg != nil || err != nil {
		t.Fatalf("no operator cert: want plaintext (nil, nil), got %v %v", cfg, err)
	}

	a.ghostCfg.operatorTLSConfig = operatorTLSConfig{OperatorCertFile: opCert, OperatorKeyFile: opKey, CAFile: p.CAFile()}
	if cfg, err := a.operatorDialConfig(identity.RoleGhost, "ghost.alpha", false); cfg == nil || err != nil {
		t.Fatalf("operator cert: %v %v", cfg, err)
	}

	a.ghostCfg.operatorTLSConfig = operatorTLSConfig{OperatorCertFile: ghCert, OperatorKeyFile: ghKey, CAFile: p.CAFile()}
	if _, err := a.operatorDialConfig(identity.RoleGhost, "ghost.alpha", false); !errors.Is(err, identity.ErrWrongRole) {
		t.Fatalf("ghost cert as operator: expected ErrWrongRole, got %v", err)
	}
}
