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
	ExitModeDual   = "dual"

	ProxySocks5 = "socks5"
	ProxyHTTP   = "http"
	ProxyMixed  = "mixed"
)

type Config struct {
	Version    int             `json:"version"`
	Language   string          `json:"language,omitempty"`
	Listen     ListenConfig    `json:"listen"`
	Defaults   Defaults        `json:"defaults"`
	Nodes      []Node          `json:"nodes"`
	Tokens     []DeployToken   `json:"tokens"`
	ModeTokens []NodeModeToken `json:"mode_tokens,omitempty"`
}

const DefaultDeployTokenTTL = 15 * time.Minute
const DefaultNodeModeTokenTTL = 15 * time.Minute

const (
	WarpInstallAuto      = "auto"
	WarpInstallReuse     = "reuse"
	WarpInstallReinstall = "reinstall"
)

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
	CIDR6    string `json:"cidr6,omitempty"`
}

type Node struct {
	Name                    string `json:"name"`
	ExitMode                string `json:"exit_mode"`
	Proxy                   string `json:"proxy"`
	BindHost                string `json:"bind_host"`
	LocalPort               int    `json:"local_port"`
	WarpLocalPort           int    `json:"warp_local_port,omitempty"`
	PublicIP                string `json:"public_ip,omitempty"`
	Country                 string `json:"country,omitempty"`
	WGDevice                string `json:"wg_device,omitempty"`
	WGAddress               string `json:"wg_address,omitempty"`
	WGServerAddress         string `json:"wg_server_address,omitempty"`
	WGClientAddress         string `json:"wg_client_address,omitempty"`
	WGServerIPv6Address     string `json:"wg_server_ipv6_address,omitempty"`
	WGClientIPv6Address     string `json:"wg_client_ipv6_address,omitempty"`
	WGListenPort            int    `json:"wg_listen_port,omitempty"`
	WGServerPublicKey       string `json:"wg_server_public_key,omitempty"`
	WGClientPublicKey       string `json:"wg_client_public_key,omitempty"`
	WGClientPrivateKey      string `json:"wg_client_private_key,omitempty"`
	WGClientConfig          string `json:"wg_client_config,omitempty"`
	WGWarpClientAddress     string `json:"wg_warp_client_address,omitempty"`
	WGWarpClientIPv6Address string `json:"wg_warp_client_ipv6_address,omitempty"`
	WGWarpClientPublicKey   string `json:"wg_warp_client_public_key,omitempty"`
	WGWarpClientPrivateKey  string `json:"wg_warp_client_private_key,omitempty"`
	WGWarpClientConfig      string `json:"wg_warp_client_config,omitempty"`
	WGLocalDevice           string `json:"wg_local_device,omitempty"`
	WGLocalConfigPath       string `json:"wg_local_config_path,omitempty"`
	Endpoint                string `json:"endpoint,omitempty"`
	SSHHost                 string `json:"ssh_host,omitempty"`
	SSHPort                 int    `json:"ssh_port,omitempty"`
	SSHUser                 string `json:"ssh_user,omitempty"`
	SSHKeyPath              string `json:"ssh_key_path,omitempty"`
	SSHKnownHostsPath       string `json:"ssh_known_hosts_path,omitempty"`
	SSHInsecureHostKey      bool   `json:"ssh_insecure_skip_host_key_check,omitempty"`
	CreatedAt               string `json:"created_at,omitempty"`
	LastUpdated             string `json:"last_updated,omitempty"`
}

type DeployToken struct {
	Token        string `json:"token"`
	Node         Node   `json:"node"`
	ExpiresAt    string `json:"expires_at"`
	Used         bool   `json:"used"`
	Prepared     bool   `json:"prepared,omitempty"`
	Registered   bool   `json:"registered"`
	RegisteredAt string `json:"registered_at,omitempty"`
	AutoStart    bool   `json:"auto_start,omitempty"`
}

type NodeModeToken struct {
	Token       string `json:"token"`
	NodeName    string `json:"node_name"`
	TargetMode  string `json:"target_mode"`
	Node        Node   `json:"node"`
	ExpiresAt   string `json:"expires_at"`
	Used        bool   `json:"used"`
	Completed   bool   `json:"completed"`
	CompletedAt string `json:"completed_at,omitempty"`
	WarpInstall string `json:"warp_install,omitempty"`
	RemoveWarp  bool   `json:"remove_warp,omitempty"`
	AutoStart   bool   `json:"auto_start,omitempty"`
}

type NodeModeTokenStatus string

const (
	NodeModeTokenStatusCompleted NodeModeTokenStatus = "completed"
	NodeModeTokenStatusUsed      NodeModeTokenStatus = "used"
	NodeModeTokenStatusExpired   NodeModeTokenStatus = "expired"
	NodeModeTokenStatusUnused    NodeModeTokenStatus = "unused"
)

func Default() Config {
	return Config{
		Version:  1,
		Language: "en",
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
			CIDR6:    "fd7a:7761:7270::/64",
		},
		Nodes:      []Node{},
		Tokens:     []DeployToken{},
		ModeTokens: []NodeModeToken{},
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
	cfg = ApplyDefaults(cfg)
	return cfg, nil
}

func ApplyDefaults(cfg Config) Config {
	def := Default()
	if cfg.Language == "" {
		cfg.Language = def.Language
	}
	if cfg.Listen.Host == "" {
		cfg.Listen.Host = def.Listen.Host
	}
	if cfg.Listen.Port == 0 {
		cfg.Listen.Port = def.Listen.Port
	}
	if cfg.Defaults.BindHost == "" {
		cfg.Defaults.BindHost = def.Defaults.BindHost
	}
	if cfg.Defaults.Proxy == "" {
		cfg.Defaults.Proxy = def.Defaults.Proxy
	}
	if cfg.Defaults.ExitMode == "" {
		cfg.Defaults.ExitMode = def.Defaults.ExitMode
	}
	if cfg.Defaults.CIDR == "" {
		cfg.Defaults.CIDR = def.Defaults.CIDR
	}
	if cfg.Defaults.CIDR6 == "" {
		cfg.Defaults.CIDR6 = def.Defaults.CIDR6
	}
	if cfg.Nodes == nil {
		cfg.Nodes = []Node{}
	}
	if cfg.Tokens == nil {
		cfg.Tokens = []DeployToken{}
	}
	if cfg.ModeTokens == nil {
		cfg.ModeTokens = []NodeModeToken{}
	}
	return cfg
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
	if err := ValidateLanguage(cfg.Language); err != nil {
		return err
	}
	cfg.Language = NormalizeLanguage(cfg.Language)

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	return os.WriteFile(path, data, 0o600)
}

func NormalizeLanguage(language string) string {
	switch language {
	case "zh", "zh_CN", "zh-CN", "cn", "CN":
		return "zh"
	case "en", "en_US", "en-US", "english", "English", "":
		return "en"
	default:
		return language
	}
}

func ValidateLanguage(language string) error {
	switch NormalizeLanguage(language) {
	case "zh", "en":
		return nil
	default:
		return fmt.Errorf("unsupported language %q, expected zh or en", language)
	}
}

func SaveExisting(path string, cfg Config) error {
	if path == "" {
		path = DefaultPath()
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := ValidateLanguage(cfg.Language); err != nil {
		return err
	}
	cfg.Language = NormalizeLanguage(cfg.Language)

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
	case ExitModeDirect, ExitModeWarp, ExitModeDual:
		return nil
	default:
		return fmt.Errorf("unsupported exit mode %q, expected direct, warp, or dual", mode)
	}
}

func ValidateWarpInstall(value string) error {
	switch value {
	case "", WarpInstallAuto, WarpInstallReuse, WarpInstallReinstall:
		return nil
	default:
		return fmt.Errorf("unsupported warp install policy %q, expected auto, reuse, or reinstall", value)
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
	if node.ExitMode == ExitModeDual {
		if err := ValidatePort(node.WarpLocalPort); err != nil {
			return fmt.Errorf("warp local port: %w", err)
		}
		if node.WarpLocalPort == node.LocalPort {
			return fmt.Errorf("dual mode requires different direct and warp local ports: %d", node.LocalPort)
		}
	}

	for _, existing := range cfg.Nodes {
		if existing.Name == node.Name {
			return fmt.Errorf("node already exists: %s", node.Name)
		}
		if portUsedByNode(existing, node.BindHost, node.LocalPort) {
			return fmt.Errorf("local port already used by node %s: %s:%d", existing.Name, node.BindHost, node.LocalPort)
		}
		if node.ExitMode == ExitModeDual && portUsedByNode(existing, node.BindHost, node.WarpLocalPort) {
			return fmt.Errorf("warp local port already used by node %s: %s:%d", existing.Name, node.BindHost, node.WarpLocalPort)
		}
	}

	return nil
}

func portUsedByNode(node Node, bindHost string, port int) bool {
	if node.BindHost != bindHost {
		return false
	}
	if node.LocalPort == port {
		return true
	}
	return node.ExitMode == ExitModeDual && node.WarpLocalPort == port
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

	now := time.Now().UTC()
	nextTokens := cfg.Tokens[:0]
	for _, existing := range cfg.Tokens {
		if !existing.Used && deployTokenExpired(existing, now) {
			continue
		}
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
		if token.Node.ExitMode == ExitModeDual && portUsedByNode(existing.Node, token.Node.BindHost, token.Node.WarpLocalPort) {
			return cfg, fmt.Errorf("unused deploy token already reserves warp local port: %s:%d", token.Node.BindHost, token.Node.WarpLocalPort)
		}
		if existing.Node.ExitMode == ExitModeDual && portUsedByNode(token.Node, existing.Node.BindHost, existing.Node.WarpLocalPort) {
			return cfg, fmt.Errorf("unused deploy token already reserves local port: %s:%d", existing.Node.BindHost, existing.Node.WarpLocalPort)
		}
		nextTokens = append(nextTokens, existing)
	}

	cfg.Tokens = nextTokens
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
	next.Tokens[index].AutoStart = token.AutoStart
	return next, token.Node, nil
}

func AddNodeModeToken(cfg Config, item NodeModeToken) (Config, error) {
	if item.Token == "" {
		return cfg, errors.New("node mode token cannot be empty")
	}
	if item.NodeName == "" {
		return cfg, errors.New("node mode token node_name cannot be empty")
	}
	if err := ValidateExitMode(item.TargetMode); err != nil {
		return cfg, err
	}
	if err := ValidateWarpInstall(item.WarpInstall); err != nil {
		return cfg, err
	}
	if item.WarpInstall == "" {
		item.WarpInstall = WarpInstallAuto
	}
	if item.ExpiresAt == "" {
		return cfg, errors.New("node mode token expires_at cannot be empty")
	}

	node, ok := FindNode(cfg, item.NodeName)
	if !ok {
		return cfg, fmt.Errorf("node not found: %s", item.NodeName)
	}
	if item.Node.Name == "" {
		item.Node = node
	}
	if item.Node.WGDevice == "" || item.Node.WGServerAddress == "" || item.Node.WGClientAddress == "" {
		return cfg, fmt.Errorf("node %s has incomplete WireGuard metadata; deploy it first", item.NodeName)
	}
	if item.TargetMode == ExitModeDual && item.Node.WGWarpClientAddress == "" {
		return cfg, fmt.Errorf("node %s has no dual-mode WireGuard metadata; redeploy it as dual first", item.NodeName)
	}

	nextModeTokens := cfg.ModeTokens[:0]
	for _, existing := range cfg.ModeTokens {
		if existing.Token == item.Token {
			return cfg, errors.New("node mode token already exists")
		}
		if !existing.Used && existing.NodeName == item.NodeName {
			continue
		}
		nextModeTokens = append(nextModeTokens, existing)
	}

	cfg.ModeTokens = nextModeTokens
	cfg.ModeTokens = append(cfg.ModeTokens, item)
	return cfg, nil
}

func FindNodeModeToken(cfg Config, tokenValue string, now time.Time) (int, NodeModeToken, error) {
	for i, token := range cfg.ModeTokens {
		if token.Token != tokenValue {
			continue
		}
		if token.Used {
			return i, token, errors.New("node mode token already used")
		}

		expiresAt, err := time.Parse(time.RFC3339, token.ExpiresAt)
		if err != nil {
			return i, token, fmt.Errorf("invalid node mode token expiry: %w", err)
		}
		if now.After(expiresAt) {
			return i, token, errors.New("node mode token expired")
		}
		return i, token, nil
	}
	return -1, NodeModeToken{}, errors.New("node mode token not found")
}

func CompleteNodeModeToken(cfg Config, tokenValue string, now time.Time) (Config, Node, error) {
	index, modeToken, err := FindNodeModeToken(cfg, tokenValue, now)
	if err != nil {
		return cfg, Node{}, err
	}

	node, ok := FindNode(cfg, modeToken.NodeName)
	if !ok {
		return cfg, Node{}, fmt.Errorf("node not found: %s", modeToken.NodeName)
	}
	node.ExitMode = modeToken.TargetMode

	next, err := UpdateNode(cfg, node)
	if err != nil {
		return cfg, Node{}, err
	}
	modeToken.Used = true
	modeToken.Completed = true
	modeToken.CompletedAt = now.UTC().Format(time.RFC3339)
	next.ModeTokens[index] = modeToken
	return next, node, nil
}

func NodeModeTokenStatusOf(token NodeModeToken, now time.Time) NodeModeTokenStatus {
	if token.Used {
		if token.Completed {
			return NodeModeTokenStatusCompleted
		}
		return NodeModeTokenStatusUsed
	}
	expiresAt, err := time.Parse(time.RFC3339, token.ExpiresAt)
	if err != nil || now.After(expiresAt) {
		return NodeModeTokenStatusExpired
	}
	return NodeModeTokenStatusUnused
}

func PruneExpiredNodeModeTokens(cfg Config, now time.Time) (Config, int) {
	next := cfg.ModeTokens[:0]
	removed := 0
	for _, token := range cfg.ModeTokens {
		if !token.Used && NodeModeTokenStatusOf(token, now) == NodeModeTokenStatusExpired {
			removed++
			continue
		}
		next = append(next, token)
	}
	cfg.ModeTokens = next
	return cfg, removed
}
