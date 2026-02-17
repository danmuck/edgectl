package host

import "github.com/danmuck/edgectl/internal/seeds"

// Deps returns external dependencies required by seed.host on target hosts.
// seed.host has no external dependencies — it uses only OS-native interfaces.
func Deps() []seeds.DepSpec {
	return nil
}
