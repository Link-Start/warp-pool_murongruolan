package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"time"
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

const DefaultDeployTokenTTL = 15 * time.Minute

type ListenConfig struct {
	Host       string `json:"host"`
	PublicHost string `json:"public_host,omitempty"`
	Port       int    `json:"port"`
	Enabled    bool   `json:"enabled"`
}

type Defaults struct {
	BindHost string `json:"bind_host"`
	Proxy    string `json:"proxy"`
	ExitMode string `json:"exit_mode"`
	CIDR     string `json:"cidr"`
}

type Node struct {
	Name               string `json:"name"`
	ExitMode           string `json:"exit_mode"`
	Proxy              string `json:"proxy"`
	BindHost           string `json:"bind_host"`
	LocalPort          int    `json:"local_port"`
	PublicIP           string `json:"public_ip,omitempty"`
	Country            string `json:"country,omitempty"`
	WGDevice           string `json:"wg_device,omitempty"`
	WGAddress          string `json:"wg_address,omitempty"`
	WGServerAddress    string `json:"wg_server_address,omitempty"`
	WGClientAddress    string `json:"wg_client_address,omitempty"`
	WGListenPort       int    `json:"wg_listen_port,omitempty"`
	WGServerPublicKey  string `json:"wg_server_public_key,omitempty"`
	WGClientPublicKey  string `json:"wg_client_public_key,omitempty"`
	WGClientPrivateKey string `json:"wg_client_private_key,omitempty"`
	WGClientConfig     string `json:"wg_client_config,omitempty"`
	WGLocalDevice      string `json:"wg_local_device,omitempty"`
	WGLocalConfigPath  string `json:"wg_local_config_path,omitempty"`
	Endpoint           string `json:"endpoint,omitempty"`
	CreatedAt          string `json:"created_at,omitempty"`
	LastUpdated        string `json:"last_updated,omitempty"`
}

type DeployToken struct {
	Token        string `json:"token"`
	Node         Node   `json:"node"`
	ExpiresAt    string `json:"expires_at"`
	Used         bool   `json:"used"`
	Prepared     bool   `json:"prepared,omitempty"`
	Registered   bool   `json:"registered"`
	RegisteredAt string `json:"registered_at,omitempty"`
}

func Default() Config {
	return Config{
		Version: 1,
		Listen: ListenConfig{
			Host:       "0.0.0.0",
			PublicHost: "",
			Port:       8080,
			Enabled:    false,
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

	if err := checkConfigPermissions(path); err != nil {
		return Config{}, err
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

func checkConfigPermissions(path string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("config file permissions are too open: %s has mode %04o, expected 0600 or stricter", path, info.Mode().Perm())
	}
	return nil
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

func SaveExisting(path string, cfg Config) error {
	if path == "" {
		path = DefaultPath()
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

func ValidateListenHost(host string) error {
	return ValidateBindHost(host)
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

func AddNode(cfg Config, node Node) (Config, error) {
	if node.ExitMode == "" {
		node.ExitMode = cfg.Defaults.ExitMode
	}
	if node.Proxy == "" {
		node.Proxy = cfg.Defaults.Proxy
	}
	if node.BindHost == "" {
		node.BindHost = cfg.Defaults.BindHost
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if node.CreatedAt == "" {
		node.CreatedAt = now
	}
	node.LastUpdated = now

	if err := ValidateNode(cfg, node); err != nil {
		return cfg, err
	}

	cfg.Nodes = append(cfg.Nodes, node)
	return cfg, nil
}

func FindNode(cfg Config, name string) (Node, bool) {
	for _, node := range cfg.Nodes {
		if node.Name == name {
			return node, true
		}
	}
	return Node{}, false
}

func UpdateNode(cfg Config, node Node) (Config, error) {
	if node.Name == "" {
		return cfg, errors.New("node name cannot be empty")
	}

	for i, existing := range cfg.Nodes {
		if existing.Name != node.Name {
			continue
		}
		if node.CreatedAt == "" {
			node.CreatedAt = existing.CreatedAt
		}
		node.LastUpdated = time.Now().UTC().Format(time.RFC3339)
		cfg.Nodes[i] = node
		return cfg, nil
	}

	return cfg, fmt.Errorf("node not found: %s", node.Name)
}

func RemoveNode(cfg Config, name string) (Config, Node, error) {
	for i, node := range cfg.Nodes {
		if node.Name == name {
			cfg.Nodes = append(cfg.Nodes[:i], cfg.Nodes[i+1:]...)
			return cfg, node, nil
		}
	}
	return cfg, Node{}, fmt.Errorf("node not found: %s", name)
}

func SetListenConfig(cfg Config, listen ListenConfig) (Config, error) {
	if err := ValidateListenHost(listen.Host); err != nil {
		return cfg, err
	}
	if err := ValidatePort(listen.Port); err != nil {
		return cfg, err
	}

	cfg.Listen.Host = listen.Host
	cfg.Listen.PublicHost = listen.PublicHost
	cfg.Listen.Port = listen.Port
	cfg.Listen.Enabled = listen.Enabled
	return cfg, nil
}

func SetListenEnabled(cfg Config, enabled bool) Config {
	cfg.Listen.Enabled = enabled
	return cfg
}

func AddDeployToken(cfg Config, token DeployToken) (Config, error) {
	if token.Token == "" {
		return cfg, errors.New("deploy token cannot be empty")
	}
	if token.ExpiresAt == "" {
		return cfg, errors.New("deploy token expires_at cannot be empty")
	}
	if err := ValidateNode(cfg, token.Node); err != nil {
		return cfg, err
	}

	for _, existing := range cfg.Tokens {
		if existing.Token == token.Token {
			return cfg, errors.New("deploy token already exists")
		}
		if existing.Used {
			continue
		}
		if existing.Node.Name == token.Node.Name {
			return cfg, fmt.Errorf("unused deploy token already exists for node: %s", token.Node.Name)
		}
		if existing.Node.BindHost == token.Node.BindHost && existing.Node.LocalPort == token.Node.LocalPort {
			return cfg, fmt.Errorf("unused deploy token already reserves local port: %s:%d", token.Node.BindHost, token.Node.LocalPort)
		}
	}

	cfg.Tokens = append(cfg.Tokens, token)
	return cfg, nil
}

func RemoveDeployTokens(cfg Config, target string, includeUsed bool) (Config, int) {
	if target == "" {
		return cfg, 0
	}

	next := cfg.Tokens[:0]
	removed := 0
	for _, token := range cfg.Tokens {
		matches := token.Token == target || token.Node.Name == target
		if matches && (includeUsed || !token.Used) {
			removed++
			continue
		}
		next = append(next, token)
	}
	cfg.Tokens = next
	return cfg, removed
}

func PruneExpiredDeployTokens(cfg Config, now time.Time) (Config, int) {
	next := cfg.Tokens[:0]
	removed := 0
	for _, token := range cfg.Tokens {
		if !token.Used && deployTokenExpired(token, now) {
			removed++
			continue
		}
		next = append(next, token)
	}
	cfg.Tokens = next
	return cfg, removed
}

func deployTokenExpired(token DeployToken, now time.Time) bool {
	expiresAt, err := time.Parse(time.RFC3339, token.ExpiresAt)
	if err != nil {
		return true
	}
	return now.After(expiresAt)
}

func UseDeployToken(cfg Config, tokenValue string, now time.Time) (Config, Node, error) {
	for i, token := range cfg.Tokens {
		if token.Token != tokenValue {
			continue
		}
		if token.Used {
			return cfg, Node{}, errors.New("deploy token already used")
		}

		expiresAt, err := time.Parse(time.RFC3339, token.ExpiresAt)
		if err != nil {
			return cfg, Node{}, fmt.Errorf("invalid deploy token expiry: %w", err)
		}
		if now.After(expiresAt) {
			return cfg, Node{}, errors.New("deploy token expired")
		}

		next, err := AddNode(cfg, token.Node)
		if err != nil {
			return cfg, Node{}, err
		}
		next.Tokens[i].Used = true
		next.Tokens[i].Registered = true
		next.Tokens[i].RegisteredAt = now.UTC().Format(time.RFC3339)
		return next, token.Node, nil
	}

	return cfg, Node{}, errors.New("deploy token not found")
}

func FindDeployToken(cfg Config, tokenValue string, now time.Time) (int, DeployToken, error) {
	for i, token := range cfg.Tokens {
		if token.Token != tokenValue {
			continue
		}
		if token.Used {
			return i, token, errors.New("deploy token already used")
		}

		expiresAt, err := time.Parse(time.RFC3339, token.ExpiresAt)
		if err != nil {
			return i, token, fmt.Errorf("invalid deploy token expiry: %w", err)
		}
		if now.After(expiresAt) {
			return i, token, errors.New("deploy token expired")
		}
		return i, token, nil
	}
	return -1, DeployToken{}, errors.New("deploy token not found")
}

func PrepareDeployToken(cfg Config, tokenValue string, node Node, now time.Time) (Config, error) {
	index, token, err := FindDeployToken(cfg, tokenValue, now)
	if err != nil {
		return cfg, err
	}
	if err := ValidateNode(cfg, node); err != nil {
		return cfg, err
	}
	token.Node = node
	token.Prepared = true
	cfg.Tokens[index] = token
	return cfg, nil
}

func CompleteDeployToken(cfg Config, tokenValue string, now time.Time) (Config, Node, error) {
	index, token, err := FindDeployToken(cfg, tokenValue, now)
	if err != nil {
		return cfg, Node{}, err
	}
	if !token.Prepared {
		return cfg, Node{}, errors.New("deploy token is not prepared")
	}

	next, err := AddNode(cfg, token.Node)
	if err != nil {
		return cfg, Node{}, err
	}
	next.Tokens[index].Used = true
	next.Tokens[index].Registered = true
	next.Tokens[index].RegisteredAt = now.UTC().Format(time.RFC3339)
	return next, token.Node, nil
}
