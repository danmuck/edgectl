// edgecert is operator tooling that creates the edgectl root CA and issues principal certificates
// (admin_security.toml [trust], [identity]). Runtimes never mint certificates.
//
// Usage:
//
//	edgecert root  -out certs/root
//	edgecert issue -ca-cert certs/root/ca.crt -ca-key certs/root/ca.key -role ghost -id ghost.pi -out certs [-dns a,b] [-ip 1.2.3.4] [-days 90]
package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/danmuck/edgectl/internal/identity"
	"github.com/danmuck/edgectl/internal/pki"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "edgecert:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: edgecert root|issue [flags]")
	}
	switch args[0] {
	case "root":
		return runRoot(args[1:])
	case "issue":
		return runIssue(args[1:])
	}
	return fmt.Errorf("unknown command %q (want root or issue)", args[0])
}

// Creates <out>/ca.crt and <out>/ca.key (0600). Keep <out> offline; runtimes need only ca.crt.
func runRoot(args []string) error {
	fs := flag.NewFlagSet("root", flag.ContinueOnError)
	out := fs.String("out", "certs/root", "output directory for ca.crt and ca.key")
	days := fs.Int("days", 3650, "root validity in days")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := os.MkdirAll(*out, 0o700); err != nil {
		return err
	}
	ca, der, err := pki.NewRoot("edgectl-root", *days, time.Now())
	if err != nil {
		return err
	}
	if err := pki.WriteCertPEM(filepath.Join(*out, "ca.crt"), der); err != nil {
		return err
	}
	if err := pki.WriteKeyPEM(filepath.Join(*out, "ca.key"), ca.Key); err != nil {
		return err
	}
	fmt.Printf("root: %s serial=%s\n", filepath.Join(*out, "ca.crt"), ca.Cert.SerialNumber.Text(16))
	return nil
}

// Issues <out>/<id>.crt and <out>/<id>.key (0600) for edgectl://<role>/<id>.
func runIssue(args []string) error {
	fs := flag.NewFlagSet("issue", flag.ContinueOnError)
	caCert := fs.String("ca-cert", "certs/root/ca.crt", "root CA certificate")
	caKey := fs.String("ca-key", "certs/root/ca.key", "root CA private key (0600)")
	role := fs.String("role", "", "operator | mirage | ghost")
	id := fs.String("id", "", "principal id ([a-z0-9][a-z0-9._-]{0,62})")
	out := fs.String("out", "certs", "output directory")
	dns := fs.String("dns", "", "comma-separated DNS SANs for hostname verification")
	ips := fs.String("ip", "", "comma-separated IP SANs for hostname verification")
	days := fs.Int("days", pki.MaxLeafDays, "leaf validity in days (max 90)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	p := identity.Principal{Role: identity.Role(strings.TrimSpace(*role)), ID: strings.TrimSpace(*id)}
	if err := p.Validate(); err != nil {
		return err
	}
	ca, err := pki.LoadAuthority(*caCert, *caKey)
	if err != nil {
		return err
	}
	var ipList []net.IP
	for _, raw := range splitList(*ips) {
		ip := net.ParseIP(raw)
		if ip == nil {
			return fmt.Errorf("invalid ip %q", raw)
		}
		ipList = append(ipList, ip)
	}
	leaf, err := ca.IssueLeaf(p, splitList(*dns), ipList, *days, time.Now())
	if err != nil {
		return err
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		return err
	}
	certPath := filepath.Join(*out, p.ID+".crt")
	keyPath := filepath.Join(*out, p.ID+".key")
	if err := pki.WriteCertPEM(certPath, leaf.DER); err != nil {
		return err
	}
	if err := pki.WriteKeyPEM(keyPath, leaf.Key); err != nil {
		return err
	}
	fmt.Printf("issued: %s principal=%s serial=%s expires=%s\n",
		certPath, p.String(), leaf.Cert.SerialNumber.Text(16), leaf.Cert.NotAfter.Format(time.RFC3339))
	return nil
}

func splitList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if v := strings.TrimSpace(part); v != "" {
			out = append(out, v)
		}
	}
	return out
}
