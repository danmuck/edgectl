package ghost

// export_test.go exposes test-only hooks to the external ghost_test package (compiled only into this
// package's tests; never part of the production API).

import (
	"github.com/danmuck/edgectl/internal/seeds"
	"github.com/danmuck/edgectl/internal/tools"
)

// SetInstallRunnerForTest routes installer commands (brew, git) to r instead of the host.
func SetInstallRunnerForTest(s *Service, r tools.CommandRunner) {
	s.installerConfigHook = func(c *seeds.InstallerConfig) { c.Runner = r }
}

// BootstrapForTest runs appear -> seed -> radiate without starting the signal-driven Run loop.
func BootstrapForTest(s *Service) error { return s.bootstrap() }
