package host

import (
	"errors"
	"fmt"
	"net"
	"os"
	"runtime"
	"strings"

	"github.com/danmuck/edgectl/internal/seeds"
	"github.com/danmuck/edgectl/internal/tools"
	logs "github.com/danmuck/smplog"
)

var (
	ErrUnknownAction = errors.New("unknown seed action")
	ErrCommandFailed = errors.New("seed command failed")
)

// HostResolver abstracts Go stdlib host introspection for testability.
type HostResolver interface {
	Hostname() (string, error)
	Interfaces() ([]net.Interface, error)
	InterfaceAddrs(iface net.Interface) ([]net.Addr, error)
}

// defaultResolver delegates to os and net stdlib.
type defaultResolver struct{}

func (defaultResolver) Hostname() (string, error) { return os.Hostname() }
func (defaultResolver) Interfaces() ([]net.Interface, error) { return net.Interfaces() }
func (defaultResolver) InterfaceAddrs(iface net.Interface) ([]net.Addr, error) {
	return iface.Addrs()
}

// Seed is a predefined service seed for host introspection.
type Seed struct {
	runner   tools.CommandRunner
	resolver HostResolver
}

// NewSeed constructs a host seed with default resolver and runner.
func NewSeed() Seed {
	logs.Debug("seeds.host.NewSeed")
	return NewSeedWithDeps(tools.ExecRunner{}, defaultResolver{})
}

// NewSeedWithDeps constructs a host seed with explicit dependencies.
func NewSeedWithDeps(runner tools.CommandRunner, resolver HostResolver) Seed {
	if runner == nil {
		runner = tools.ExecRunner{}
	}
	if resolver == nil {
		resolver = defaultResolver{}
	}
	return Seed{runner: runner, resolver: resolver}
}

// Metadata returns stable identity and capability description.
func (s Seed) Metadata() seeds.SeedMetadata {
	return seeds.SeedMetadata{
		ID:          "seed.host",
		Name:        "Host Introspection",
		Description: "Host identity and network introspection adapter",
	}
}

// Operations returns host introspection behavior catalog.
func (s Seed) Operations() []seeds.OperationSpec {
	return []seeds.OperationSpec{
		{Name: "status", Description: "hostname, primary ip, os", Idempotent: true},
		{Name: "ports", Description: "listening tcp/udp ports", Idempotent: true},
		{Name: "interfaces", Description: "network interfaces with ips", Idempotent: true},
	}
}

// Execute dispatches host introspection operations.
func (s Seed) Execute(action string, args map[string]string) (seeds.SeedResult, error) {
	act := strings.TrimSpace(action)
	switch act {
	case "status":
		return s.statusOp()
	case "ports":
		return s.portsOp()
	case "interfaces":
		return s.interfacesOp()
	default:
		errMsg := fmt.Sprintf("unknown action: %s", act)
		return seeds.SeedResult{Status: "error", Stderr: []byte(errMsg + "\n"), ExitCode: 64}, ErrUnknownAction
	}
}

// statusOp uses Go stdlib for hostname/os and derives primary IP from interfaces.
func (s Seed) statusOp() (seeds.SeedResult, error) {
	hostname, err := s.resolver.Hostname()
	if err != nil {
		return seeds.SeedResult{
			Status: "error", Stderr: []byte(err.Error() + "\n"), ExitCode: 1,
		}, fmt.Errorf("%w: hostname: %v", ErrCommandFailed, err)
	}

	primaryIP := primaryIPFromInterfaces(s.resolver)
	goos := runtime.GOOS
	goarch := runtime.GOARCH

	out := fmt.Sprintf("hostname=%s\nos=%s\narch=%s\nip=%s\n", hostname, goos, goarch, primaryIP)
	return seeds.SeedResult{Status: "ok", Stdout: []byte(out), ExitCode: 0}, nil
}

// portsOp shells out to platform-appropriate command for listening ports.
func (s Seed) portsOp() (seeds.SeedResult, error) {
	// Cross-platform: try lsof first (macOS/Linux), fall back to ss (Linux).
	stdout, stderr, exitCode, err := s.runner.Run("lsof", "-i", "-P", "-n", "-sTCP:LISTEN")
	if err != nil {
		// Fallback to ss on Linux.
		stdout, stderr, exitCode, err = s.runner.Run("ss", "-tulnp")
		if err != nil {
			if len(stderr) == 0 {
				stderr = []byte(err.Error() + "\n")
			}
			if exitCode == 0 {
				exitCode = 1
			}
			return seeds.SeedResult{
				Status: "error", Stdout: stdout, Stderr: stderr, ExitCode: exitCode,
			}, fmt.Errorf("%w: %v", ErrCommandFailed, err)
		}
	}
	return seeds.SeedResult{Status: "ok", Stdout: stdout, Stderr: stderr, ExitCode: 0}, nil
}

// interfacesOp uses Go stdlib to enumerate network interfaces and their addresses.
func (s Seed) interfacesOp() (seeds.SeedResult, error) {
	ifaces, err := s.resolver.Interfaces()
	if err != nil {
		return seeds.SeedResult{
			Status: "error", Stderr: []byte(err.Error() + "\n"), ExitCode: 1,
		}, fmt.Errorf("%w: interfaces: %v", ErrCommandFailed, err)
	}

	var b strings.Builder
	for _, iface := range ifaces {
		addrs, err := s.resolver.InterfaceAddrs(iface)
		if err != nil {
			continue
		}
		addrStrs := make([]string, 0, len(addrs))
		for _, a := range addrs {
			addrStrs = append(addrStrs, a.String())
		}
		fmt.Fprintf(&b, "%s flags=%s addrs=%s\n", iface.Name, iface.Flags.String(), strings.Join(addrStrs, ","))
	}
	return seeds.SeedResult{Status: "ok", Stdout: []byte(b.String()), ExitCode: 0}, nil
}

// primaryIPFromInterfaces returns the first non-loopback IPv4 address found.
func primaryIPFromInterfaces(resolver HostResolver) string {
	ifaces, err := resolver.Interfaces()
	if err != nil {
		return "unknown"
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := resolver.InterfaceAddrs(iface)
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip != nil && ip.To4() != nil && !ip.IsLoopback() {
				return ip.String()
			}
		}
	}
	return "unknown"
}
