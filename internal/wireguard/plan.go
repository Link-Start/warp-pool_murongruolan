package wireguard

import (
	"fmt"
	"net"
	"strings"

	"github.com/murongruolan/warp-pool/internal/config"
)

const DefaultListenPort = 51820

type Plan struct {
	Device           string
	ListenPort       int
	Endpoint         string
	ServerAddress    string
	ClientAddress    string
	ServerPrivateKey string
	ServerPublicKey  string
	ClientPrivateKey string
	ClientPublicKey  string
	ServerConfig     string
	ClientConfig     string
}

type Options struct {
	Node       config.Node
	CIDR       string
	Endpoint   string
	ListenPort int
}

func BuildPlan(cfg config.Config, opts Options) (Plan, error) {
	if opts.CIDR == "" {
		opts.CIDR = cfg.Defaults.CIDR
	}
	if opts.ListenPort == 0 {
		opts.ListenPort = DefaultListenPort
	}
	if opts.Endpoint == "" {
		return Plan{}, fmt.Errorf("wireguard endpoint is required")
	}

	serverKey, err := GenerateKeyPair()
	if err != nil {
		return Plan{}, err
	}
	clientKey, err := GenerateKeyPair()
	if err != nil {
		return Plan{}, err
	}

	serverIP, clientIP, err := AllocatePair(cfg, opts.CIDR)
	if err != nil {
		return Plan{}, err
	}

	device := SafeDeviceName(opts.Node.Name)
	plan := Plan{
		Device:           device,
		ListenPort:       opts.ListenPort,
		Endpoint:         fmt.Sprintf("%s:%d", opts.Endpoint, opts.ListenPort),
		ServerAddress:    serverIP + "/30",
		ClientAddress:    clientIP + "/30",
		ServerPrivateKey: serverKey.PrivateKey,
		ServerPublicKey:  serverKey.PublicKey,
		ClientPrivateKey: clientKey.PrivateKey,
		ClientPublicKey:  clientKey.PublicKey,
	}

	plan.ServerConfig = RenderServerConfig(plan)
	plan.ClientConfig = RenderClientConfig(plan)
	return plan, nil
}

func ApplyPlan(node config.Node, plan Plan) config.Node {
	node.WGDevice = plan.Device
	node.WGAddress = plan.ClientAddress
	node.WGServerAddress = plan.ServerAddress
	node.WGClientAddress = plan.ClientAddress
	node.WGListenPort = plan.ListenPort
	node.WGServerPublicKey = plan.ServerPublicKey
	node.WGClientPublicKey = plan.ClientPublicKey
	node.WGClientPrivateKey = plan.ClientPrivateKey
	node.WGClientConfig = plan.ClientConfig
	node.Endpoint = plan.Endpoint
	return node
}

func RenderServerConfig(plan Plan) string {
	clientIP := strings.TrimSuffix(plan.ClientAddress, "/30")
	return fmt.Sprintf(`[Interface]
PrivateKey = %s
Address = %s
ListenPort = %d
SaveConfig = false

[Peer]
PublicKey = %s
AllowedIPs = %s/32
`, plan.ServerPrivateKey, plan.ServerAddress, plan.ListenPort, plan.ClientPublicKey, clientIP)
}

func RenderClientConfig(plan Plan) string {
	serverIP := strings.TrimSuffix(plan.ServerAddress, "/30")
	return fmt.Sprintf(`[Interface]
PrivateKey = %s
Address = %s

[Peer]
PublicKey = %s
Endpoint = %s
AllowedIPs = %s/32
PersistentKeepalive = 25
`, plan.ClientPrivateKey, plan.ClientAddress, plan.ServerPublicKey, plan.Endpoint, serverIP)
}

func SafeDeviceName(name string) string {
	var b strings.Builder
	b.WriteString("wp")
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
	if out == "" || out == "wp" {
		out = "wp-node"
	}
	if len(out) > 15 {
		out = out[:15]
		out = strings.TrimRight(out, "-")
	}
	return out
}

func AllocatePair(cfg config.Config, cidr string) (string, string, error) {
	ip, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", "", fmt.Errorf("parse wireguard CIDR: %w", err)
	}
	ip4 := ip.To4()
	if ip4 == nil {
		return "", "", fmt.Errorf("only IPv4 wireguard CIDR is supported: %s", cidr)
	}

	used := map[string]bool{}
	for _, node := range cfg.Nodes {
		for _, value := range []string{node.WGServerAddress, node.WGClientAddress, node.WGAddress} {
			if value == "" {
				continue
			}
			host := strings.Split(value, "/")[0]
			used[host] = true
		}
	}

	networkIP := ipNet.IP.To4()
	if networkIP == nil {
		return "", "", fmt.Errorf("only IPv4 wireguard CIDR is supported: %s", cidr)
	}
	base := uint32(networkIP[0])<<24 | uint32(networkIP[1])<<16 | uint32(networkIP[2])<<8 | uint32(networkIP[3])
	for offset := uint32(0); offset < 1<<16; offset += 4 {
		server := uint32ToIP(base + offset + 1)
		client := uint32ToIP(base + offset + 2)
		if !ipNet.Contains(net.ParseIP(server)) || !ipNet.Contains(net.ParseIP(client)) {
			break
		}
		if used[server] || used[client] {
			continue
		}
		return server, client, nil
	}

	return "", "", fmt.Errorf("no available wireguard address pair in %s", cidr)
}

func uint32ToIP(value uint32) string {
	return net.IPv4(byte(value>>24), byte(value>>16), byte(value>>8), byte(value)).String()
}
