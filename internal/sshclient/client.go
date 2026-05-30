package sshclient

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"time"

	"golang.org/x/crypto/ssh"
)

type Auth struct {
	Password string
	KeyPath  string
}

type Config struct {
	Host    string
	Port    int
	User    string
	Auth    Auth
	Timeout time.Duration
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

	client, err := ssh.Dial("tcp", net.JoinHostPort(cfg.Host, fmt.Sprintf("%d", cfg.Port)), &ssh.ClientConfig{
		User:            cfg.User,
		Auth:            auth,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
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
	session, err := c.inner.NewSession()
	if err != nil {
		return Result{}, fmt.Errorf("create ssh session: %w", err)
	}
	defer session.Close()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

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
