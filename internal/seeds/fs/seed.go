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

// Seed is a filesystem persistence adapter scoped to one configured root.
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
	root := strings.TrimSpace(filepath.ToSlash(s.root))
	if root == "" {
		root = "local/dir"
	}
	return seeds.SeedMetadata{
		ID:          SeedID,
		Name:        "Filesystem",
		Description: "Temporary file persistence seed scoped to " + root,
	}
}

// Operations returns supported filesystem persistence operations.
func (s Seed) Operations() []seeds.OperationSpec {
	return []seeds.OperationSpec{
		{Name: "write", Description: "write content to relative path under seed root", Idempotent: true},
		{Name: "read", Description: "read content from relative path under seed root", Idempotent: true},
		{Name: "delete", Description: "delete file path under seed root", Idempotent: true},
		{Name: "list", Description: "list file paths under seed root (optional prefix)", Idempotent: true},
	}
}

// CommandCatalog returns guided operator-facing command templates for this seed.
func (s Seed) CommandCatalog() []seeds.CommandTemplate {
	seedID := s.Metadata().ID
	return []seeds.CommandTemplate{
		{
			ID:           seedID + ".write",
			Label:        "Filesystem Write",
			Description:  "Write file content under ghost-scoped seed.fs root.",
			SeedSelector: seedID,
			Operation:    "write",
			Args: []seeds.CommandArgSpec{
				{Key: "path", Prompt: "filename (relative path)", Required: true},
				{Key: "content", Prompt: "file content", Required: true, Multiline: true, Terminator: ".done"},
			},
			DefaultBlocking: true,
		},
		{
			ID:           seedID + ".read",
			Label:        "Filesystem Read",
			Description:  "Read file content from ghost-scoped seed.fs root.",
			SeedSelector: seedID,
			Operation:    "read",
			Args: []seeds.CommandArgSpec{
				{Key: "path", Prompt: "relative file path", Required: true},
			},
			DefaultBlocking: true,
		},
		{
			ID:           seedID + ".list",
			Label:        "Filesystem List",
			Description:  "List file paths from ghost-scoped seed.fs root.",
			SeedSelector: seedID,
			Operation:    "list",
			Args: []seeds.CommandArgSpec{
				{Key: "prefix", Prompt: "path prefix (optional)", Required: false},
			},
			DefaultBlocking: true,
		},
		{
			ID:           seedID + ".delete",
			Label:        "Filesystem Delete",
			Description:  "Delete file path from ghost-scoped seed.fs root.",
			SeedSelector: seedID,
			Operation:    "delete",
			Args: []seeds.CommandArgSpec{
				{Key: "path", Prompt: "relative file path", Required: true},
			},
			DefaultBlocking: true,
		},
	}
}

// Execute applies one filesystem operation scoped to the configured seed root.
func (s Seed) Execute(action string, args map[string]string) (seeds.SeedResult, error) {
	switch strings.TrimSpace(action) {
	case "write":
		p, err := s.resolvePath(args["path"])
		if err != nil {
			return seeds.ErrorResultErr(err), err
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return seeds.ErrorResultErr(err), err
		}
		content := []byte(args["content"])
		if err := os.WriteFile(p, content, 0o644); err != nil {
			return seeds.ErrorResultErr(err), err
		}
		return seeds.OKResult("ok\n"), nil
	case "read":
		p, err := s.resolvePath(args["path"])
		if err != nil {
			return seeds.ErrorResultErr(err), err
		}
		out, err := os.ReadFile(p)
		if err != nil {
			return seeds.ErrorResultErr(err), err
		}
		return seeds.SeedResult{Status: "ok", Stdout: out, ExitCode: 0}, nil
	case "delete":
		p, err := s.resolvePath(args["path"])
		if err != nil {
			return seeds.ErrorResultErr(err), err
		}
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return seeds.ErrorResultErr(err), err
		}
		return seeds.OKResult("ok\n"), nil
	case "list":
		root, err := filepath.Abs(s.root)
		if err != nil {
			return seeds.ErrorResultErr(err), err
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
		return seeds.OKResult(strings.Join(keys, "\n") + "\n"), nil
	default:
		err := fmt.Errorf("seed.fs: unknown action=%q", action)
		return seeds.ErrorResultErr(err), err
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

