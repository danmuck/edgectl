package fs

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/danmuck/edgectl/internal/seeds"
)

const (
	// SeedID is the canonical identifier for filesystem-backed persistence.
	SeedID = "seed.fs"
)

// Seed is a filesystem persistence adapter scoped to local/dir.
type Seed struct {
	root string
}

// NewSeed constructs a filesystem seed rooted at local/dir under cwd.
func NewSeed() Seed {
	return NewSeedWithRoot(filepath.Join("local", "dir"))
}

// NewSeedWithRoot constructs a filesystem seed with explicit root.
func NewSeedWithRoot(root string) Seed {
	resolved := strings.TrimSpace(root)
	if resolved == "" {
		resolved = filepath.Join("local", "dir")
	}
	return Seed{root: resolved}
}

// Metadata returns stable seed identity and capability details.
func (s Seed) Metadata() seeds.SeedMetadata {
	return seeds.SeedMetadata{
		ID:          SeedID,
		Name:        "Filesystem (local/dir)",
		Description: "Temporary file persistence seed scoped to local/dir",
	}
}

// Operations returns supported filesystem persistence operations.
func (s Seed) Operations() []seeds.OperationSpec {
	return []seeds.OperationSpec{
		{Name: "write", Description: "write content to relative path under local/dir", Idempotent: true},
		{Name: "read", Description: "read content from relative path under local/dir", Idempotent: true},
		{Name: "delete", Description: "delete file path under local/dir", Idempotent: true},
		{Name: "list", Description: "list file paths under local/dir (optional prefix)", Idempotent: true},
	}
}

// Execute applies one filesystem operation scoped to local/dir.
func (s Seed) Execute(action string, args map[string]string) (seeds.SeedResult, error) {
	switch strings.TrimSpace(action) {
	case "write":
		p, err := s.resolvePath(args["path"])
		if err != nil {
			return errorResult(err), err
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return errorResult(err), err
		}
		content := []byte(args["content"])
		if err := os.WriteFile(p, content, 0o644); err != nil {
			return errorResult(err), err
		}
		return okResult("ok\n"), nil
	case "read":
		p, err := s.resolvePath(args["path"])
		if err != nil {
			return errorResult(err), err
		}
		out, err := os.ReadFile(p)
		if err != nil {
			return errorResult(err), err
		}
		return seeds.SeedResult{Status: "ok", Stdout: out, ExitCode: 0}, nil
	case "delete":
		p, err := s.resolvePath(args["path"])
		if err != nil {
			return errorResult(err), err
		}
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return errorResult(err), err
		}
		return okResult("ok\n"), nil
	case "list":
		root, err := filepath.Abs(s.root)
		if err != nil {
			return errorResult(err), err
		}
		prefix := strings.TrimSpace(args["prefix"])
		keys := make([]string, 0)
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil || d.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return nil
			}
			rel = filepath.ToSlash(rel)
			if prefix == "" || strings.HasPrefix(rel, prefix) {
				keys = append(keys, rel)
			}
			return nil
		})
		sort.Strings(keys)
		return okResult(strings.Join(keys, "\n") + "\n"), nil
	default:
		err := fmt.Errorf("seed.fs: unknown action=%q", action)
		return errorResult(err), err
	}
}

func (s Seed) resolvePath(pathArg string) (string, error) {
	rel := strings.TrimSpace(pathArg)
	if rel == "" {
		return "", fmt.Errorf("seed.fs: missing path")
	}
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("seed.fs: absolute path not allowed")
	}
	root, err := filepath.Abs(s.root)
	if err != nil {
		return "", err
	}
	p := filepath.Clean(filepath.Join(root, rel))
	if !isWithin(p, root) {
		return "", fmt.Errorf("seed.fs: path escapes root")
	}
	return p, nil
}

func isWithin(path string, root string) bool {
	p := filepath.Clean(path)
	r := filepath.Clean(root)
	if p == r {
		return true
	}
	return strings.HasPrefix(p, r+string(os.PathSeparator))
}

func okResult(stdout string) seeds.SeedResult {
	return seeds.SeedResult{
		Status:   "ok",
		Stdout:   []byte(stdout),
		ExitCode: 0,
	}
}

func errorResult(err error) seeds.SeedResult {
	msg := "error"
	if err != nil {
		msg = err.Error()
	}
	return seeds.SeedResult{
		Status:   "error",
		Stderr:   []byte(msg + "\n"),
		ExitCode: 1,
	}
}

