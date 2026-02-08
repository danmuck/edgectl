package mirage

import (
	"fmt"
	"net"
	"strings"
)

// GhostAdminTarget declares one ghost admin endpoint allowed for Mirage preload attach.
type GhostAdminTarget struct {
	GhostID   string
	AdminAddr string
}

// normalizeGhostAdminAddr resolves hostnames to stable IP endpoints for ghost admin control.
func normalizeGhostAdminAddr(rawAddr string) (string, error) {
	addr := strings.TrimSpace(rawAddr)
	if addr == "" {
		return "", fmt.Errorf("ghost admin addr required")
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", fmt.Errorf("invalid ghost admin addr %q", addr)
	}
	host = strings.TrimSpace(host)
	port = strings.TrimSpace(port)
	if port == "" {
		return "", fmt.Errorf("invalid ghost admin addr %q", addr)
	}
	if host == "" || strings.EqualFold(host, "localhost") {
		host = "127.0.0.1"
		return net.JoinHostPort(host, port), nil
	}
	if ip := net.ParseIP(host); ip != nil {
		return net.JoinHostPort(ip.String(), port), nil
	}
	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		return "", fmt.Errorf("resolve ghost admin host %q: %w", host, err)
	}
	for i := range ips {
		if v4 := ips[i].To4(); v4 != nil {
			return net.JoinHostPort(v4.String(), port), nil
		}
	}
	return net.JoinHostPort(ips[0].String(), port), nil
}

