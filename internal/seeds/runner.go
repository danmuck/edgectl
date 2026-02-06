package seeds

import (
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func joinCommand(cmd string, args []string) string {
	if len(args) == 0 {
		return shellEscape(cmd)
	}

	var builder strings.Builder
	builder.WriteString(shellEscape(cmd))
	for _, arg := range args {
		builder.WriteByte(' ')
		builder.WriteString(shellEscape(arg))
	}

	return builder.String()
}

func shellEscape(value string) string {
	if value == "" {
		return "''"
	}

	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

type Runner interface {
	Run(cmd string, args ...string) (string, error)
	RunStreaming(cmd string, args []string, stdout, stderr io.Writer) error
}

type LocalRunner struct{}

func (LocalRunner) Run(cmd string, args ...string) (string, error) {
	out, err := exec.Command(cmd, args...).CombinedOutput()
	return string(out), err
}

func (LocalRunner) RunStreaming(cmd string, args []string, stdout, stderr io.Writer) error {
	command := exec.Command(cmd, args...)
	if stdout != nil {
		command.Stdout = stdout
	}
	if stderr != nil {
		command.Stderr = stderr
	}
	return command.Run()
}

type SSHRunner struct {
	Host                        string
	Port                        string
	User                        string
	KeyPath                     string
	Passphrase                  []byte
	KnownHostsPath              string
	InsecureSkipHostKeyChecking bool
	Timeout                     time.Duration
}

func (r SSHRunner) Run(cmd string, args ...string) (string, error) {
	client, err := r.dial()
	if err != nil {
		return "", err
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()

	out, err := session.CombinedOutput(joinCommand(cmd, args))
	return string(out), err
}

func (r SSHRunner) RunStreaming(cmd string, args []string, stdout, stderr io.Writer) error {
	client, err := r.dial()
	if err != nil {
		return err
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	if stdout != nil {
		session.Stdout = stdout
	}
	if stderr != nil {
		session.Stderr = stderr
	}

	return session.Run(joinCommand(cmd, args))
}

func (r SSHRunner) dial() (*ssh.Client, error) {
	address, err := r.address()
	if err != nil {
		return nil, err
	}

	config, err := r.clientConfig()
	if err != nil {
		return nil, err
	}

	if r.Timeout <= 0 {
		return ssh.Dial("tcp", address, config)
	}

	conn, err := net.DialTimeout("tcp", address, r.Timeout)
	if err != nil {
		return nil, err
	}

	clientConn, chans, reqs, err := ssh.NewClientConn(conn, address, config)
	if err != nil {
		conn.Close()
		return nil, err
	}

	return ssh.NewClient(clientConn, chans, reqs), nil
}

func (r SSHRunner) address() (string, error) {
	host := strings.TrimSpace(r.Host)
	if host == "" {
		return "", fmt.Errorf("ssh host is required")
	}

	if r.Port != "" {
		return net.JoinHostPort(host, r.Port), nil
	}

	if _, _, err := net.SplitHostPort(host); err == nil {
		return host, nil
	}

	return net.JoinHostPort(host, "22"), nil
}

func (r SSHRunner) clientConfig() (*ssh.ClientConfig, error) {
	if r.User == "" {
		return nil, fmt.Errorf("ssh user is required")
	}

	signer, err := r.signer()
	if err != nil {
		return nil, err
	}

	var hostKeyCallback ssh.HostKeyCallback
	if r.InsecureSkipHostKeyChecking {
		hostKeyCallback = ssh.InsecureIgnoreHostKey()
	} else {
		callback, err := r.knownHostsCallback()
		if err != nil {
			return nil, err
		}
		hostKeyCallback = callback
	}

	return &ssh.ClientConfig{
		User:            r.User,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: hostKeyCallback,
		Timeout:         r.Timeout,
	}, nil
}

func (r SSHRunner) signer() (ssh.Signer, error) {
	if r.KeyPath == "" {
		return nil, fmt.Errorf("ssh key path is required")
	}

	privateKey, err := os.ReadFile(r.KeyPath)
	if err != nil {
		return nil, err
	}

	if len(r.Passphrase) > 0 {
		return ssh.ParsePrivateKeyWithPassphrase(privateKey, r.Passphrase)
	}

	return ssh.ParsePrivateKey(privateKey)
}

func (r SSHRunner) knownHostsCallback() (ssh.HostKeyCallback, error) {
	path := strings.TrimSpace(r.KnownHostsPath)
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("known hosts path not set and home dir unavailable")
		}
		path = filepath.Join(home, ".ssh", "known_hosts")
	}

	return knownhosts.New(path)
}
