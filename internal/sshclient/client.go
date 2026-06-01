package sshclient

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

type Auth struct {
	Password string
	KeyPath  string
}

type Config struct {
	Host                  string
	Port                  int
	User                  string
	Auth                  Auth
	Timeout               time.Duration
	KnownHostsPath        string
	InsecureIgnoreHostKey bool
}

type Client struct {
	inner *ssh.Client
}

type Result struct {
	Stdout string
	Stderr string
}

func Dial(cfg Config) (*Client, error) {
	if cfg.Host == "" {
		return nil, fmt.Errorf("ssh host is required")
	}
	if cfg.Port == 0 {
		cfg.Port = 22
	}
	if cfg.User == "" {
		return nil, fmt.Errorf("ssh user is required")
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 15 * time.Second
	}

	auth, err := authMethods(cfg.Auth)
	if err != nil {
		return nil, err
	}
	hostKeyCallback, err := hostKeyCallback(cfg)
	if err != nil {
		return nil, err
	}

	client, err := ssh.Dial("tcp", net.JoinHostPort(cfg.Host, fmt.Sprintf("%d", cfg.Port)), &ssh.ClientConfig{
		User:            cfg.User,
		Auth:            auth,
		HostKeyCallback: hostKeyCallback,
		Timeout:         cfg.Timeout,
	})
	if err != nil {
		return nil, fmt.Errorf("connect ssh %s:%d: %w", cfg.Host, cfg.Port, err)
	}

	return &Client{inner: client}, nil
}

func (c *Client) Close() error {
	return c.inner.Close()
}

func (c *Client) Run(command string) (Result, error) {
	return c.RunWithInput(command, "")
}

func (c *Client) RunWithInput(command string, input string) (Result, error) {
	session, err := c.inner.NewSession()
	if err != nil {
		return Result{}, fmt.Errorf("create ssh session: %w", err)
	}
	defer session.Close()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr
	if input != "" {
		session.Stdin = bytes.NewReader([]byte(input))
	}

	err = session.Run(command)
	result := Result{Stdout: stdout.String(), Stderr: stderr.String()}
	if err != nil {
		return result, fmt.Errorf("run remote command %q: %w", command, err)
	}
	return result, nil
}

func (c *Client) Upload(path string, data []byte, mode string) error {
	command := fmt.Sprintf("umask 077; cat > %s; chmod %s %s", shellQuote(path), shellQuote(mode), shellQuote(path))
	session, err := c.inner.NewSession()
	if err != nil {
		return fmt.Errorf("create ssh session: %w", err)
	}
	defer session.Close()

	session.Stdin = bytes.NewReader(data)
	var stderr bytes.Buffer
	session.Stderr = &stderr
	if err := session.Run(command); err != nil {
		return fmt.Errorf("upload %s: %w: %s", path, err, stderr.String())
	}
	return nil
}

func authMethods(auth Auth) ([]ssh.AuthMethod, error) {
	var methods []ssh.AuthMethod
	if auth.Password != "" {
		methods = append(methods, ssh.Password(auth.Password))
	}
	if auth.KeyPath != "" {
		data, err := os.ReadFile(auth.KeyPath)
		if err != nil {
			return nil, fmt.Errorf("read ssh key: %w", err)
		}
		signer, err := ssh.ParsePrivateKey(data)
		if err != nil {
			return nil, fmt.Errorf("parse ssh key: %w", err)
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}
	if len(methods) == 0 {
		return nil, fmt.Errorf("ssh password or key path is required")
	}
	return methods, nil
}

func hostKeyCallback(cfg Config) (ssh.HostKeyCallback, error) {
	if cfg.InsecureIgnoreHostKey {
		return ssh.InsecureIgnoreHostKey(), nil
	}
	path := cfg.KnownHostsPath
	if path == "" {
		path = DefaultKnownHostsPath()
	}
	if path == "" {
		return nil, fmt.Errorf("known_hosts path is required; pass --known-hosts or --insecure-skip-host-key-check")
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("known_hosts file not found: %s; add the host key first or pass --insecure-skip-host-key-check", path)
		}
		return nil, fmt.Errorf("read known_hosts file: %w", err)
	}
	callback, err := knownhosts.New(path)
	if err != nil {
		return nil, fmt.Errorf("parse known_hosts file %s: %w", path, err)
	}
	return callback, nil
}

func DefaultKnownHostsPath() string {
	if runtime.GOOS == "windows" {
		profile := os.Getenv("USERPROFILE")
		if profile == "" {
			return ""
		}
		return filepath.Join(profile, ".ssh", "known_hosts")
	}
	home := os.Getenv("HOME")
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".ssh", "known_hosts")
}

func shellQuote(value string) string {
	quoted := "'"
	for _, r := range value {
		if r == '\'' {
			quoted += "'\"'\"'"
			continue
		}
		quoted += string(r)
	}
	return quoted + "'"
}
