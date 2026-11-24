package adminauth

import (
	"context"
	"crypto/tls"
	"errors"
	"testing"
	"time"

	"github.com/danmuck/edgectl/internal/identity"
	"github.com/danmuck/edgectl/internal/testutil/tlstest"
)

func TestListenPlaintextRules(t *testing.T) {
	if _, err := Listen("0.0.0.0:0", Credentials{}, identity.Principal{}, false); !errors.Is(err, ErrPlaintextNotAllowed) {
		t.Fatalf("plaintext on all interfaces: expected ErrPlaintextNotAllowed, got %v", err)
	}
	if _, err := Listen(":0", Credentials{}, identity.Principal{}, false); !errors.Is(err, ErrPlaintextNotAllowed) {
		t.Fatalf("plaintext with empty host: expected ErrPlaintextNotAllowed, got %v", err)
	}
	if _, err := Listen("127.0.0.1:0", Credentials{}, identity.Principal{}, true); !errors.Is(err, ErrPlaintextNotAllowed) {
		t.Fatalf("plaintext in production: expected ErrPlaintextNotAllowed, got %v", err)
	}
	ln, err := Listen("127.0.0.1:0", Credentials{}, identity.Principal{}, false)
	if err != nil {
		t.Fatalf("loopback plaintext in development: %v", err)
	}
	ln.Close()
}

func TestPrivileged(t *testing.T) {
	for _, a := range []string{"execute_envelope", "install_dependencies", "spawn_ghost", "bind_mirage", "deploy_ghost", "approve_dependencies", "execute_dependencies"} {
		if !Privileged(a) {
			t.Errorf("%s should be privileged", a)
		}
	}
	if Privileged("status") || Privileged("list_intents") {
		t.Fatalf("read actions must not be privileged")
	}
}

// mTLS server for ghost.alpha; returns its address and a channel of accepted peers or errors.
func serveGhost(t *testing.T, p *tlstest.PKI, denied []string) (string, <-chan any) {
	t.Helper()
	cert, key, _ := p.Issue(t, identity.RoleGhost, "ghost.alpha")
	creds := Credentials{CertFile: cert, KeyFile: key, CAFile: p.CAFile(), DeniedSerials: denied}
	ln, err := Listen("127.0.0.1:0", creds, identity.Principal{Role: identity.RoleGhost, ID: "ghost.alpha"}, true)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	out := make(chan any, 4)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			peer, err := PeerFromConn(conn, 2*time.Second)
			if err != nil {
				out <- err
			} else {
				out <- peer
			}
			conn.Close()
		}
	}()
	return ln.Addr().String(), out
}

func dial(t *testing.T, addr string, cfg *tls.Config) error {
	t.Helper()
	conn, err := Dial(context.Background(), addr, cfg, 2*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.(*tls.Conn).Handshake()
}

func mirageClient(t *testing.T, p *tlstest.PKI, cert, key string, verify func(identity.Principal) error) *tls.Config {
	t.Helper()
	cfg, err := ClientTLSConfig(Credentials{CertFile: cert, KeyFile: key, CAFile: p.CAFile()},
		identity.Principal{Role: identity.RoleMirage, ID: "mirage.main"}, verify)
	if err != nil {
		t.Fatalf("client config: %v", err)
	}
	return cfg
}

func TestMutualTLSPinnedPrincipals(t *testing.T) {
	p := tlstest.NewPKI(t)
	addr, peers := serveGhost(t, p, nil)
	cert, key, _ := p.Issue(t, identity.RoleMirage, "mirage.main")

	ok := mirageClient(t, p, cert, key, ExpectPrincipal(identity.Principal{Role: identity.RoleGhost, ID: "ghost.alpha"}))
	if err := dial(t, addr, ok); err != nil {
		t.Fatalf("pinned handshake: %v", err)
	}
	got := <-peers
	peer, isPeer := got.(Peer)
	if !isPeer || !peer.Authenticated || peer.Principal.String() != "edgectl://mirage/mirage.main" {
		t.Fatalf("server saw %+v", got)
	}

	wrong := mirageClient(t, p, cert, key, ExpectPrincipal(identity.Principal{Role: identity.RoleGhost, ID: "ghost.beta"}))
	if err := dial(t, addr, wrong); !errors.Is(err, ErrUnexpectedPeer) {
		t.Fatalf("wrong pinned ghost: expected ErrUnexpectedPeer, got %v", err)
	}
	<-peers
}

func TestServerRejectsMissingForeignAndDeniedClients(t *testing.T) {
	p := tlstest.NewPKI(t)
	operatorCert, operatorKey, operatorSerial := p.Issue(t, identity.RoleOperator, "dan")
	addr, peers := serveGhost(t, p, []string{operatorSerial})

	// No client certificate.
	noCert := &tls.Config{MinVersion: tls.VersionTLS13, InsecureSkipVerify: true}
	_ = dial(t, addr, noCert)
	if _, isErr := (<-peers).(error); !isErr {
		t.Fatalf("server accepted a client without a certificate")
	}

	// Certificate from a different root.
	foreign := tlstest.NewPKI(t)
	fCert, fKey, _ := foreign.Issue(t, identity.RoleMirage, "mirage.evil")
	fCfg, err := ClientTLSConfig(Credentials{CertFile: fCert, KeyFile: fKey, CAFile: p.CAFile()},
		identity.Principal{Role: identity.RoleMirage, ID: "mirage.evil"}, ExpectRole(identity.RoleGhost))
	if err != nil {
		t.Fatal(err)
	}
	_ = dial(t, addr, fCfg)
	if _, isErr := (<-peers).(error); !isErr {
		t.Fatalf("server accepted a certificate from a foreign root")
	}

	// Valid chain but revoked serial.
	opCfg, err := ClientTLSConfig(Credentials{CertFile: operatorCert, KeyFile: operatorKey, CAFile: p.CAFile()},
		identity.Principal{Role: identity.RoleOperator, ID: "dan"}, ExpectRole(identity.RoleGhost))
	if err != nil {
		t.Fatal(err)
	}
	_ = dial(t, addr, opCfg)
	if _, isErr := (<-peers).(error); !isErr {
		t.Fatalf("server accepted a revoked certificate")
	}
}

func TestServerTLSConfigRequiresMatchingSelf(t *testing.T) {
	p := tlstest.NewPKI(t)
	cert, key, _ := p.Issue(t, identity.RoleGhost, "ghost.alpha")
	_, err := ServerTLSConfig(Credentials{CertFile: cert, KeyFile: key, CAFile: p.CAFile()},
		identity.Principal{Role: identity.RoleGhost, ID: "ghost.beta"})
	if !errors.Is(err, ErrSelfIdentity) {
		t.Fatalf("expected ErrSelfIdentity, got %v", err)
	}
}

func TestIsLoopbackAddr(t *testing.T) {
	for addr, want := range map[string]bool{
		"127.0.0.1:7010": true, "[::1]:7010": true, "localhost:7010": true,
		"0.0.0.0:7010": false, ":7010": false, "10.0.0.5:7010": false, "bad": false,
	} {
		if got := IsLoopbackAddr(addr); got != want {
			t.Errorf("%s: got %v want %v", addr, got, want)
		}
	}
}
