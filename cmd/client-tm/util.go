package main

// util.go contains pure helper functions (string formatting, address normalization, seed operations).

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/danmuck/edgectl/internal/seeds"
	seedflow "github.com/danmuck/edgectl/internal/seeds/flow"
	seedfs "github.com/danmuck/edgectl/internal/seeds/fs"
	seedkv "github.com/danmuck/edgectl/internal/seeds/kv"
	seedmongod "github.com/danmuck/edgectl/internal/seeds/mongod"
)

func indentLines(in string, prefix string) string {
	lines := strings.Split(strings.TrimRight(in, "\n"), "\n")
	if len(lines) == 0 {
		return ""
	}
	var b strings.Builder
	for _, line := range lines {
		b.WriteString(prefix)
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String()
}

// Returns connected ghost ids that advertise one required seed.
func connectedGhostCandidatesForSeed(
	routes []MirageRoute,
	services []MirageAvailableService,
	seedID string,
) []string {
	connected := make(map[string]struct{}, len(routes))
	for i := range routes {
		route := routes[i]
		if route.Connected {
			connected[strings.TrimSpace(route.GhostID)] = struct{}{}
		}
	}
	outSet := make(map[string]struct{})
	for i := range services {
		svc := services[i]
		if strings.TrimSpace(svc.SeedID) != strings.TrimSpace(seedID) {
			continue
		}
		for j := range svc.GhostIDs {
			ghostID := strings.TrimSpace(svc.GhostIDs[j])
			if ghostID == "" {
				continue
			}
			if _, ok := connected[ghostID]; !ok {
				continue
			}
			outSet[ghostID] = struct{}{}
		}
	}
	out := make([]string, 0, len(outSet))
	for ghostID := range outSet {
		out = append(out, ghostID)
	}
	sort.Strings(out)
	return out
}

func operationsForSeed(seedID string) []seeds.OperationSpec {
	switch strings.TrimSpace(seedID) {
	case "seed.flow":
		s := seedflow.NewSeed()
		return sortedOps(s.Operations())
	case "seed.fs":
		s := seedfs.NewSeed()
		return sortedOps(s.Operations())
	case "seed.kv":
		s := seedkv.NewSeed()
		return sortedOps(s.Operations())
	case "seed.mongod":
		s := seedmongod.NewSeed()
		return sortedOps(s.Operations())
	default:
		return nil
	}
}

func sortedOps(in []seeds.OperationSpec) []seeds.OperationSpec {
	out := make([]seeds.OperationSpec, len(in))
	copy(out, in)
	sort.Slice(out, func(i int, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

func truncateRight(in string, max int) string {
	if max <= 0 {
		return ""
	}
	if len(in) <= max {
		return in
	}
	if max <= 3 {
		return in[:max]
	}
	return in[:max-3] + "..."
}

func normalizeSuffix(in string) string {
	raw := strings.ToLower(strings.TrimSpace(in))
	if raw == "" {
		return "node"
	}
	var b strings.Builder
	lastDot := false
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			b.WriteByte(c)
			lastDot = false
			continue
		}
		if !lastDot {
			b.WriteByte('.')
			lastDot = true
		}
	}
	out := strings.Trim(b.String(), ".")
	if out == "" {
		return "node"
	}
	return out
}

func inferGhostIDFromTargetName(name string) string {
	n := strings.TrimSpace(name)
	if n == "" {
		return "ghost.node"
	}
	if n == "local-ghost" {
		return "ghost.local"
	}
	if strings.HasPrefix(n, "local-ghost.") {
		suffix := strings.TrimPrefix(n, "local-ghost.")
		return "ghost.local." + normalizeSuffix(suffix)
	}
	return "ghost." + normalizeSuffix(n)
}

func normalizeTargetAddr(rootAddr string, requested string) (string, error) {
	req := strings.TrimSpace(requested)
	if req == "" {
		return "", errors.New("address required")
	}
	rootHost, _, rootErr := net.SplitHostPort(strings.TrimSpace(rootAddr))
	if rootErr != nil {
		rootHost = "127.0.0.1"
	}
	rootHost, hostErr := resolveEndpointHost(rootHost)
	if hostErr != nil {
		rootHost = "127.0.0.1"
	}
	if strings.Contains(req, ":") {
		host, port, err := net.SplitHostPort(req)
		if err != nil {
			return "", fmt.Errorf("invalid address %q", req)
		}
		if strings.TrimSpace(host) == "" {
			host = rootHost
		}
		host, err = resolveEndpointHost(host)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(port) == "" {
			return "", fmt.Errorf("invalid address %q", req)
		}
		return net.JoinHostPort(host, port), nil
	}
	if _, err := strconv.Atoi(req); err != nil {
		return "", fmt.Errorf("invalid port %q", req)
	}
	return net.JoinHostPort(rootHost, req), nil
}

// Normalizes localhost and resolvable DNS names to stable IP addresses.
func resolveEndpointHost(rawHost string) (string, error) {
	host := strings.TrimSpace(rawHost)
	if host == "" || strings.EqualFold(host, "localhost") {
		return "127.0.0.1", nil
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.String(), nil
	}
	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		return "", fmt.Errorf("resolve host %q: %w", host, err)
	}
	for i := range ips {
		if v4 := ips[i].To4(); v4 != nil {
			return v4.String(), nil
		}
	}
	return ips[0].String(), nil
}

func normalizeClientMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "g", "ghost":
		return "ghost"
	case "m", "mirage":
		return "mirage"
	default:
		return strings.ToLower(strings.TrimSpace(mode))
	}
}

func (a *App) clearIfEnabled() {
	if !a.clearScreen {
		return
	}
	fmt.Print("\033[H\033[2J")
}

// Creates a missing file and parent directory for config bootstrapping.
func ensureFile(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	return f.Close()
}
