package singbox

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"strings"

	"github.com/murongruolan/warp-pool/internal/config"
)

type Config struct {
	Log       Log        `json:"log,omitempty"`
	Inbounds  []Inbound  `json:"inbounds"`
	Outbounds []Outbound `json:"outbounds"`
	Endpoints []Endpoint `json:"endpoints,omitempty"`
	Route     Route      `json:"route"`
}

type Log struct {
	Level string `json:"level,omitempty"`
}

type Inbound struct {
	Type       string `json:"type"`
	Tag        string `json:"tag"`
	Listen     string `json:"listen"`
	ListenPort int    `json:"listen_port"`
}

type Outbound struct {
	Type string `json:"type"`
	Tag  string `json:"tag"`
}

type Endpoint struct {
	Type           string         `json:"type"`
	Tag            string         `json:"tag"`
	System         bool           `json:"system"`
	Name           string         `json:"name"`
	MTU            int            `json:"mtu,omitempty"`
	Address        []string       `json:"address"`
	PrivateKey     string         `json:"private_key"`
	Peers          []EndpointPeer `json:"peers"`
	DomainStrategy string         `json:"domain_strategy,omitempty"`
}

type EndpointPeer struct {
	Address             string   `json:"address"`
	Port                int      `json:"port"`
	PublicKey           string   `json:"public_key"`
	AllowedIPs          []string `json:"allowed_ips"`
	PersistentKeepalive int      `json:"persistent_keepalive_interval,omitempty"`
}

type Route struct {
	Rules               []RouteRule `json:"rules"`
	AutoDetectInterface bool        `json:"auto_detect_interface,omitempty"`
}

type RouteRule struct {
	Inbound  []string `json:"inbound,omitempty"`
	Outbound string   `json:"outbound"`
}

type Options struct {
	LogLevel string
	MTU      int
}

func Build(cfg config.Config, opts Options) (Config, error) {
	if opts.LogLevel == "" {
		opts.LogLevel = "info"
	}
	if opts.MTU == 0 {
		opts.MTU = 1420
	}

	out := Config{
		Log: Log{Level: opts.LogLevel},
		Outbounds: []Outbound{
			{Type: "direct", Tag: "direct"},
			{Type: "block", Tag: "block"},
		},
		Route: Route{AutoDetectInterface: true},
	}

	for _, node := range cfg.Nodes {
		inbound, endpoint, rule, err := buildNode(node, opts)
		if err != nil {
			return Config{}, err
		}
		out.Inbounds = append(out.Inbounds, inbound)
		out.Endpoints = append(out.Endpoints, endpoint)
		out.Route.Rules = append(out.Route.Rules, rule)
	}

	if len(out.Inbounds) == 0 {
		return Config{}, fmt.Errorf("no nodes configured")
	}
	return out, nil
}

func Marshal(cfg Config) ([]byte, error) {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func BuildJSON(cfg config.Config, opts Options) ([]byte, error) {
	sb, err := Build(cfg, opts)
	if err != nil {
		return nil, err
	}
	return Marshal(sb)
}

func buildNode(node config.Node, opts Options) (Inbound, Endpoint, RouteRule, error) {
	if err := validateNode(node); err != nil {
		return Inbound{}, Endpoint{}, RouteRule{}, err
	}

	inboundTag := "in-" + safeTag(node.Name)
	endpointTag := "wg-" + safeTag(node.Name)
	inboundType := node.Proxy
	if inboundType == config.ProxySocks5 {
		inboundType = "socks"
	}

	host, port, err := splitEndpoint(node.Endpoint)
	if err != nil {
		return Inbound{}, Endpoint{}, RouteRule{}, fmt.Errorf("node %s endpoint: %w", node.Name, err)
	}

	endpointName := node.WGLocalDevice
	if endpointName == "" {
		endpointName = "wpc-" + safeTag(node.Name)
	}

	inbound := Inbound{
		Type:       inboundType,
		Tag:        inboundTag,
		Listen:     node.BindHost,
		ListenPort: node.LocalPort,
	}
	endpoint := Endpoint{
		Type:       "wireguard",
		Tag:        endpointTag,
		System:     false,
		Name:       endpointName,
		MTU:        opts.MTU,
		Address:    []string{node.WGClientAddress},
		PrivateKey: node.WGClientPrivateKey,
		Peers: []EndpointPeer{
			{
				Address:             host,
				Port:                port,
				PublicKey:           node.WGServerPublicKey,
				AllowedIPs:          []string{"0.0.0.0/0"},
				PersistentKeepalive: 25,
			},
		},
	}
	rule := RouteRule{
		Inbound:  []string{inboundTag},
		Outbound: endpointTag,
	}
	return inbound, endpoint, rule, nil
}

func validateNode(node config.Node) error {
	if node.Name == "" {
		return fmt.Errorf("node name cannot be empty")
	}
	if err := config.ValidateProxy(node.Proxy); err != nil {
		return fmt.Errorf("node %s: %w", node.Name, err)
	}
	if err := config.ValidateBindHost(node.BindHost); err != nil {
		return fmt.Errorf("node %s: %w", node.Name, err)
	}
	if err := config.ValidatePort(node.LocalPort); err != nil {
		return fmt.Errorf("node %s: %w", node.Name, err)
	}
	for _, field := range []struct {
		name  string
		value string
	}{
		{name: "wg_client_address", value: node.WGClientAddress},
		{name: "wg_client_private_key", value: node.WGClientPrivateKey},
		{name: "wg_server_public_key", value: node.WGServerPublicKey},
		{name: "endpoint", value: node.Endpoint},
	} {
		if strings.TrimSpace(field.value) == "" {
			return fmt.Errorf("node %s missing %s; deploy it first", node.Name, field.name)
		}
	}
	return nil
}

func splitEndpoint(endpoint string) (string, int, error) {
	parts := strings.LastIndex(endpoint, ":")
	if parts < 0 {
		return "", 0, fmt.Errorf("expected host:port, got %s", endpoint)
	}
	host := endpoint[:parts]
	portText := endpoint[parts+1:]
	port, err := netip.ParseAddrPort("127.0.0.1:" + portText)
	if err != nil {
		return "", 0, fmt.Errorf("invalid endpoint port %q", portText)
	}
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		host = strings.TrimPrefix(strings.TrimSuffix(host, "]"), "[")
	}
	if strings.TrimSpace(host) == "" {
		return "", 0, fmt.Errorf("empty endpoint host")
	}
	return host, int(port.Port()), nil
}

func safeTag(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "node"
	}
	return out
}
