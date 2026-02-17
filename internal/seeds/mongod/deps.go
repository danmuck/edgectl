package mongod

import "github.com/danmuck/edgectl/internal/seeds"

// Deps returns external dependencies required by seed.mongod on target hosts.
func Deps() []seeds.DepSpec {
	return []seeds.DepSpec{
		{
			Name:          "mongod",
			Binary:        "mongod",
			InstallMethod: seeds.InstallMethodApt,
			AptPackage:    "mongodb-org",
			Required:      true,
		},
		{
			Name:          "mongosh",
			Binary:        "mongosh",
			InstallMethod: seeds.InstallMethodApt,
			AptPackage:    "mongodb-mongosh",
			Required:      true,
		},
	}
}
