package fs

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/danmuck/edgectl/internal/testutil/testlog"
)

func TestSeedWriteReadListDelete(t *testing.T) {
	testlog.Start(t)
	root := filepath.Join(t.TempDir(), "local", "dir")
	seed := NewSeedWithRoot(root)

	if _, err := seed.Execute("write", map[string]string{
		"path":    "buildlog/a.log",
		"content": "hello",
	}); err != nil {
		t.Fatalf("write: %v", err)
	}

	readRes, err := seed.Execute("read", map[string]string{"path": "buildlog/a.log"})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(readRes.Stdout) != "hello" {
		t.Fatalf("unexpected read content: %q", string(readRes.Stdout))
	}

	listRes, err := seed.Execute("list", map[string]string{"prefix": "buildlog/"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(string(listRes.Stdout), "buildlog/a.log") {
		t.Fatalf("expected buildlog/a.log in list output: %q", string(listRes.Stdout))
	}

	if _, err := seed.Execute("delete", map[string]string{"path": "buildlog/a.log"}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := seed.Execute("read", map[string]string{"path": "buildlog/a.log"}); err == nil {
		t.Fatalf("expected read error after delete")
	}
}

