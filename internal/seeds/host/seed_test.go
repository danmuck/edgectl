package host

import (
	"errors"
	"net"
	"strings"
	"testing"

	"github.com/danmuck/edgectl/internal/seeds"
	"github.com/danmuck/edgectl/internal/testutil/testlog"
)

type fakeRunner struct {
	stdout   []byte
	stderr   []byte
	exitCode uint32
	err      error
	name     string
	args     []string
}

func (r *fakeRunner) Run(name string, args ...string) ([]byte, []byte, uint32, error) {
	r.name = name
	r.args = append([]string{}, args...)
	return r.stdout, r.stderr, r.exitCode, r.err
}

type fakeResolver struct {
	hostname    string
	hostnameErr error
	ifaces      []net.Interface
	ifacesErr   error
	addrs       map[string][]net.Addr
}

func (r *fakeResolver) Hostname() (string, error)            { return r.hostname, r.hostnameErr }
func (r *fakeResolver) Interfaces() ([]net.Interface, error) { return r.ifaces, r.ifacesErr }
func (r *fakeResolver) InterfaceAddrs(iface net.Interface) ([]net.Addr, error) {
	addrs, ok := r.addrs[iface.Name]
	if !ok {
		return nil, nil
	}
	return addrs, nil
}

func TestSeedMetadata(t *testing.T) {
	testlog.Start(t)
	seed := NewSeedWithDeps(&fakeRunner{}, &fakeResolver{hostname: "test"})
	meta := seed.Metadata()
	if meta.ID != "seed.host" {
		t.Fatalf("unexpected id: %q", meta.ID)
	}
	if err := seeds.ValidateMetadata(meta); err != nil {
		t.Fatalf("metadata should be valid: %v", err)
	}
}

func TestSeedStatusReturnsHostInfo(t *testing.T) {
	testlog.Start(t)
	resolver := &fakeResolver{
		hostname: "edgenode-1",
		ifaces: []net.Interface{
			{Index: 1, Name: "eth0", Flags: net.FlagUp},
		},
		addrs: map[string][]net.Addr{
			"eth0": {&net.IPNet{IP: net.ParseIP("192.168.1.10"), Mask: net.CIDRMask(24, 32)}},
		},
	}
	seed := NewSeedWithDeps(&fakeRunner{}, resolver)
	res, err := seed.Execute("status", nil)
	if err != nil {
		t.Fatalf("status execute failed: %v", err)
	}
	if res.Status != "ok" || res.ExitCode != 0 {
		t.Fatalf("unexpected status result: %+v", res)
	}
	out := string(res.Stdout)
	if !strings.Contains(out, "hostname=edgenode-1") {
		t.Fatalf("expected hostname in output: %q", out)
	}
	if !strings.Contains(out, "ip=192.168.1.10") {
		t.Fatalf("expected ip in output: %q", out)
	}
}

func TestSeedStatusHostnameError(t *testing.T) {
	testlog.Start(t)
	resolver := &fakeResolver{hostnameErr: errors.New("no hostname")}
	seed := NewSeedWithDeps(&fakeRunner{}, resolver)
	res, err := seed.Execute("status", nil)
	if !errors.Is(err, ErrCommandFailed) {
		t.Fatalf("expected ErrCommandFailed, got %v", err)
	}
	if res.Status != "error" {
		t.Fatalf("expected error status: %+v", res)
	}
}

func TestSeedPortsUsesLsof(t *testing.T) {
	testlog.Start(t)
	r := &fakeRunner{stdout: []byte("COMMAND PID\n")}
	seed := NewSeedWithDeps(r, &fakeResolver{hostname: "test"})
	res, err := seed.Execute("ports", nil)
	if err != nil {
		t.Fatalf("ports execute failed: %v", err)
	}
	if res.Status != "ok" {
		t.Fatalf("unexpected ports result: %+v", res)
	}
	if r.name != "lsof" {
		t.Fatalf("expected lsof command, got %q", r.name)
	}
}

func TestSeedInterfacesReturnsIfaceList(t *testing.T) {
	testlog.Start(t)
	resolver := &fakeResolver{
		hostname: "test",
		ifaces: []net.Interface{
			{Index: 1, Name: "lo0", Flags: net.FlagUp | net.FlagLoopback},
			{Index: 2, Name: "eth0", Flags: net.FlagUp},
		},
		addrs: map[string][]net.Addr{
			"lo0":  {&net.IPNet{IP: net.ParseIP("127.0.0.1"), Mask: net.CIDRMask(8, 32)}},
			"eth0": {&net.IPNet{IP: net.ParseIP("10.0.0.5"), Mask: net.CIDRMask(24, 32)}},
		},
	}
	seed := NewSeedWithDeps(&fakeRunner{}, resolver)
	res, err := seed.Execute("interfaces", nil)
	if err != nil {
		t.Fatalf("interfaces execute failed: %v", err)
	}
	if res.Status != "ok" {
		t.Fatalf("unexpected interfaces result: %+v", res)
	}
	out := string(res.Stdout)
	if !strings.Contains(out, "eth0") {
		t.Fatalf("expected eth0 in output: %q", out)
	}
}

func TestSeedUnknownAction(t *testing.T) {
	testlog.Start(t)
	seed := NewSeedWithDeps(&fakeRunner{}, &fakeResolver{hostname: "test"})
	res, err := seed.Execute("bogus", nil)
	if !errors.Is(err, ErrUnknownAction) {
		t.Fatalf("expected ErrUnknownAction, got %v", err)
	}
	if res.Status != "error" {
		t.Fatalf("expected error status: %+v", res)
	}
}

func TestSeedBrewVersion(t *testing.T) {
	testlog.Start(t)
	r := &fakeRunner{stdout: []byte("Homebrew 4.4.0\n")}
	seed := NewSeedWithDeps(r, &fakeResolver{hostname: "test"})
	res, err := seed.Execute("brew_version", nil)
	if err != nil {
		t.Fatalf("brew_version execute failed: %v", err)
	}
	if res.Status != "ok" || res.ExitCode != 0 {
		t.Fatalf("unexpected brew_version result: %+v", res)
	}
	if r.name != "brew" || len(r.args) != 1 || r.args[0] != "--version" {
		t.Fatalf("unexpected brew command invocation: name=%q args=%v", r.name, r.args)
	}
	if !strings.Contains(string(res.Stdout), "Homebrew") {
		t.Fatalf("expected brew version output, got %q", string(res.Stdout))
	}
}

func TestSeedBrewVersionError(t *testing.T) {
	testlog.Start(t)
	r := &fakeRunner{exitCode: 127, err: errors.New("not found")}
	seed := NewSeedWithDeps(r, &fakeResolver{hostname: "test"})
	res, err := seed.Execute("brew_version", nil)
	if !errors.Is(err, ErrCommandFailed) {
		t.Fatalf("expected ErrCommandFailed, got %v", err)
	}
	if res.Status != "error" || res.ExitCode != 127 {
		t.Fatalf("unexpected brew_version failure result: %+v", res)
	}
}

func TestSeedCommandCatalog(t *testing.T) {
	testlog.Start(t)
	seed := NewSeedWithDeps(&fakeRunner{}, &fakeResolver{hostname: "test"})
	catalog := seed.CommandCatalog()
	if len(catalog) != 4 {
		t.Fatalf("unexpected catalog size: %d", len(catalog))
	}
	if catalog[0].SeedSelector != "seed.host" || catalog[0].Operation != "status" {
		t.Fatalf("unexpected first catalog entry: %+v", catalog[0])
	}
}
