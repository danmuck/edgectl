package fs

import (
	"os"
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

func TestSeedCommandCatalog(t *testing.T) {
	testlog.Start(t)
	seed := NewSeedWithRoot(filepath.Join(t.TempDir(), "local", "dir"))
	catalog := seed.CommandCatalog()
	if len(catalog) != 4 {
		t.Fatalf("unexpected catalog size: %d", len(catalog))
	}
	if catalog[0].SeedSelector != SeedID || catalog[0].Operation != "write" {
		t.Fatalf("unexpected first catalog entry: %+v", catalog[0])
	}
	if !catalog[0].DefaultBlocking {
		t.Fatalf("expected write command default blocking")
	}
}

// protocol.toml [envelopes.execution_safety]: a symlink inside the seed root must not carry
// write, read, or delete outside it.
func TestSeedRefusesSymlinkEscape(t *testing.T) {
	testlog.Start(t)
	base := t.TempDir()
	root := filepath.Join(base, "root")
	outside := filepath.Join(base, "outside")
	for _, d := range []string{root, outside} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	secret := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secret, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.Symlink(secret, filepath.Join(root, "filelink")); err != nil {
		t.Fatal(err)
	}
	seed := NewSeedWithRoot(root)

	if _, err := seed.Execute("write", map[string]string{"path": "link/pwn.txt", "content": "x"}); err == nil {
		t.Fatal("write through directory symlink: expected refusal")
	}
	if _, err := os.Stat(filepath.Join(outside, "pwn.txt")); !os.IsNotExist(err) {
		t.Fatalf("file landed outside the root: %v", err)
	}
	if _, err := seed.Execute("write", map[string]string{"path": "filelink", "content": "x"}); err == nil {
		t.Fatal("write through file symlink: expected refusal")
	}
	if _, err := seed.Execute("read", map[string]string{"path": "link/secret.txt"}); err == nil {
		t.Fatal("read through directory symlink: expected refusal")
	}
	if _, err := seed.Execute("read", map[string]string{"path": "filelink"}); err == nil {
		t.Fatal("read through file symlink: expected refusal")
	}
	if _, err := seed.Execute("delete", map[string]string{"path": "link/secret.txt"}); err == nil {
		t.Fatal("delete through directory symlink: expected refusal")
	}
	if got, err := os.ReadFile(secret); err != nil || string(got) != "keep" {
		t.Fatalf("outside file changed: %q err=%v", got, err)
	}
}

// Lexical escapes stay rejected and delete on a never-created root stays idempotent.
func TestSeedRejectsLexicalEscapeAndDeleteIsIdempotent(t *testing.T) {
	testlog.Start(t)
	seed := NewSeedWithRoot(filepath.Join(t.TempDir(), "never", "created"))
	for _, p := range []string{"../x", "a/../../x", "/etc/passwd"} {
		if _, err := seed.Execute("write", map[string]string{"path": p, "content": "x"}); err == nil {
			t.Fatalf("path %q: expected rejection", p)
		}
	}
	if _, err := seed.Execute("delete", map[string]string{"path": "missing.txt"}); err != nil {
		t.Fatalf("delete on missing root: %v", err)
	}
}
