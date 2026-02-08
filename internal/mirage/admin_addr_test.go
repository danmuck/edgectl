package mirage

import "testing"

func TestNormalizeGhostAdminAddrLocalhost(t *testing.T) {
	addr, err := normalizeGhostAdminAddr("localhost:7011")
	if err != nil {
		t.Fatalf("normalize localhost addr: %v", err)
	}
	if addr != "127.0.0.1:7011" {
		t.Fatalf("unexpected normalized addr: %q", addr)
	}
}

func TestNormalizeGhostAdminAddrRejectsInvalid(t *testing.T) {
	if _, err := normalizeGhostAdminAddr("7011"); err == nil {
		t.Fatalf("expected invalid addr error")
	}
}
