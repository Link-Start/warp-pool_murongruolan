package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
)

const (
	ExitModeDirect = "direct"
	ExitModeWarp   = "warp"

	ProxySocks5 = "socks5"
	ProxyHTTP   = "http"
	ProxyMixed  = "mixed"
)

type Config struct {
	Version  int           `json:"version"`
	Listen   ListenConfig  `json:"listen"`
	Defaults Defaults      `json:"defaults"`
	Nodes    []Node        `json:"nodes"`
	Tokens   []DeployToken `json:"tokens"`
}

type ListenConfig struct {
	Host    string `json:"host"`
	Port    int    `json:"port"`
	Enabled bool   `json:"enabled"`
}

type Defaults struct {
	BindHost string `json:"bind_host"`
	Proxy    string `json:"proxy"`
	ExitMode string `json:"exit_mode"`
	CIDR     string `json:"cidr"`
}

type Node struct {
	Name        string `json:"name"`
	ExitMode    string `json:"exit_mode"`
	Proxy       string `json:"proxy"`
	BindHost    string `json:"bind_host"`
	LocalPort   int    `json:"local_port"`
	PublicIP    string `json:"public_ip,omitempty"`
	Country     string `json:"country,omitempty"`
	WGDevice    string `json:"wg_device,omitempty"`
	WGAddress   string `json:"wg_address,omitempty"`
	Endpoint    string `json:"endpoint,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
	LastUpdated string `json:"last_updated,omitempty"`
}

type DeployToken struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expires_at"`
	Used      bool   `json:"used"`
}

func Default() Config {
	return Config{
		Version: 1,
		Listen: ListenConfig{
			Host:    "0.0.0.0",
			Port:    18080,
			Enabled: false,
		},
		Defaults: Defaults{
			BindHost: "127.0.0.1",
			Proxy:    ProxyMixed,
			ExitMode: ExitModeDirect,
			CIDR:     "10.200.0.0/16",
		},
		Nodes:  []Node{},
		Tokens: []DeployToken{},
	}
}

func DefaultPath() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("ProgramData"), "warppool", "config.json")
	}
	return "/etc/warppool/config.json"
}

func Load(path string) (Config, error) {
	if path == "" {
		path = DefaultPath()
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config %s: %w", path, err)
	}
	return cfg, nil
}

func Save(path string, cfg Config, force bool) error {
	if path == "" {
		path = DefaultPath()
	}

	if !force {
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("config already exists: %s", path)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	return os.WriteFile(path, data, 0o600)
}

func ValidatePort(port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535: %d", port)
	}
	return nil
}

func ValidateProxy(proxy string) error {
	switch proxy {
	case ProxySocks5, ProxyHTTP, ProxyMixed:
		return nil
	default:
		return fmt.Errorf("unsupported proxy protocol %q, expected socks5, http, or mixed", proxy)
	}
}

func ValidateExitMode(mode string) error {
	switch mode {
	case ExitModeDirect, ExitModeWarp:
		return nil
	default:
		return fmt.Errorf("unsupported exit mode %q, expected direct or warp", mode)
	}
}

func ValidateBindHost(host string) error {
	if host == "" {
		return errors.New("bind host cannot be empty")
	}
	if ip := net.ParseIP(host); ip == nil {
		return fmt.Errorf("invalid bind host: %s", host)
	}
	return nil
}

func CheckPortAvailable(host string, port int) error {
	if err := ValidateBindHost(host); err != nil {
		return err
	}
	if err := ValidatePort(port); err != nil {
		return err
	}

	ln, err := net.Listen("tcp", net.JoinHostPort(host, fmt.Sprintf("%d", port)))
	if err != nil {
		return fmt.Errorf("port %s:%d is not available: %w", host, port, err)
	}
	return ln.Close()
}

func ValidateNode(cfg Config, node Node) error {
	if node.Name == "" {
		return errors.New("node name cannot be empty")
	}
	if err := ValidateExitMode(node.ExitMode); err != nil {
		return err
	}
	if err := ValidateProxy(node.Proxy); err != nil {
		return err
	}
	if err := ValidateBindHost(node.BindHost); err != nil {
		return err
	}
	if err := ValidatePort(node.LocalPort); err != nil {
		return err
	}

	for _, existing := range cfg.Nodes {
		if existing.Name == node.Name {
			return fmt.Errorf("node already exists: %s", node.Name)
		}
		if existing.BindHost == node.BindHost && existing.LocalPort == node.LocalPort {
			return fmt.Errorf("local port already used by node %s: %s:%d", existing.Name, node.BindHost, node.LocalPort)
		}
	}

	return nil
}
