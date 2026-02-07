package kv

import (
	"strings"
	"testing"

	"github.com/danmuck/edgectl/internal/testutil/testlog"
)

func TestSeedPutGetListDelete(t *testing.T) {
	testlog.Start(t)

	seed := NewSeed()
	if _, err := seed.Execute("put", map[string]string{"key": "a", "value": "1"}); err != nil {
		t.Fatalf("put a: %v", err)
	}
	if _, err := seed.Execute("put", map[string]string{"key": "b", "value": "2"}); err != nil {
		t.Fatalf("put b: %v", err)
	}

	getRes, err := seed.Execute("get", map[string]string{"key": "a"})
	if err != nil {
		t.Fatalf("get a: %v", err)
	}
	if strings.TrimSpace(string(getRes.Stdout)) != "1" {
		t.Fatalf("unexpected get result: %q", string(getRes.Stdout))
	}

	listRes, err := seed.Execute("list", map[string]string{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	out := strings.TrimSpace(string(listRes.Stdout))
	if out != "a\nb" && out != "b\na" {
		t.Fatalf("unexpected list output: %q", out)
	}

	if _, err := seed.Execute("delete", map[string]string{"key": "a"}); err != nil {
		t.Fatalf("delete a: %v", err)
	}
	if _, err := seed.Execute("get", map[string]string{"key": "a"}); err == nil {
		t.Fatalf("expected missing key error after delete")
	}
}
