package docker

import "github.com/danmuck/edgectl/internal/seeds"

// Deps returns external dependencies required by seed.docker on target hosts.
func Deps() []seeds.DepSpec {
	return []seeds.DepSpec{
		{
			Name:          "docker",
			Binary:        "docker",
			InstallMethod: seeds.InstallMethodApt,
			AptPackage:    "docker.io",
			Required:      true,
		},
	}
}
