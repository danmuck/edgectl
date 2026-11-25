package tools

import "strings"

// NormalizeNonEmptyList trims entries, drops empty values, and preserves order.
func NormalizeNonEmptyList(in []string) []string {
	if len(in) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(in))
	for _, raw := range in {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	return out
}
