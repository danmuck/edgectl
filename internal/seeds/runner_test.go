package seeds

import (
	"testing"

	logs "github.com/danmuck/smplog"
)

func TestJoinCommandEscaping(t *testing.T) {
	got := joinCommand("echo", []string{"a b", "quote'v"})
	want := "'echo' 'a b' 'quote'\"'\"'v'"
	if got != want {
		t.Fatalf("unexpected joined command\nwant: %s\ngot:  %s", want, got)
	}
	logs.Logf("runner/join-command: %s", got)
}

func TestSSHRunnerAddressValidation(t *testing.T) {
	r := SSHRunner{}
	if _, err := r.address(); err == nil {
		t.Fatalf("expected host validation error")
	}

	r.Host = "node-a"
	addr, err := r.address()
	if err != nil {
		t.Fatalf("unexpected address error: %v", err)
	}
	if addr != "node-a:22" {
		t.Fatalf("expected default ssh port, got %q", addr)
	}
	logs.Logf("runner/address: host=%s resolved=%s", r.Host, addr)
}

func TestSSHRunnerClientConfigValidation(t *testing.T) {
	r := SSHRunner{Host: "node-a"}
	if _, err := r.clientConfig(); err == nil {
		t.Fatalf("expected missing user validation error")
	}
	logs.Logf("runner/client-config: missing user path validated")
}
