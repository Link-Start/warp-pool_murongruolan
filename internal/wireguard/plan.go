package wireguard

import (
	"fmt"
	"hash/fnv"
	"net"
	"strings"

	"github.com/murongruolan/warp-pool/internal/config"
)

const DefaultListenPort = 51820

type Plan struct {
	Device               string
	ListenPort           int
	Endpoint             string
	EgressInterface      string
	EnableForwarding     bool
	DualMode             bool
	ServerAddress        string
	ClientAddress        string
	WarpClientAddress    string
	ServerPrivateKey     string
	ServerPublicKey      string
	ClientPrivateKey     string
	ClientPublicKey      string
	WarpClientPrivateKey string
	WarpClientPublicKey  string
	ServerConfig         string
	ClientConfig         string
	WarpClientConfig     string
}

type Options struct {
	Node                 config.Node
	CIDR                 string
	Endpoint             string
	EndpointPort         int
	ListenPort           int
	EgressInterface      string
	EnableForwarding     bool
	ServerPrivateKey     string
	ServerPublicKey      string
	ClientPrivateKey     string
	ClientPublicKey      string
	WarpClientPrivateKey string
	WarpClientPublicKey  string
}

func BuildPlan(cfg config.Config, opts Options) (Plan, error) {
	if opts.CIDR == "" {
		opts.CIDR = cfg.Defaults.CIDR
	}
	if opts.ListenPort == 0 {
		opts.ListenPort = DefaultListenPort
	}
	if opts.EndpointPort == 0 {
		opts.EndpointPort = opts.ListenPort
	}
	if opts.Endpoint == "" {
		return Plan{}, fmt.Errorf("wireguard endpoint is required")
	}

	serverKey := KeyPair{
		PrivateKey: opts.ServerPrivateKey,
		PublicKey:  opts.ServerPublicKey,
	}
	if serverKey.PrivateKey == "" || serverKey.PublicKey == "" {
		var err error
		serverKey, err = GenerateKeyPair()
		if err != nil {
			return Plan{}, err
		}
	}

	clientKey := KeyPair{
		PrivateKey: opts.ClientPrivateKey,
		PublicKey:  opts.ClientPublicKey,
	}
	if clientKey.PrivateKey == "" || clientKey.PublicKey == "" {
		var err error
		clientKey, err = GenerateKeyPair()
		if err != nil {
			return Plan{}, err
		}
	}

	dualMode := opts.Node.ExitMode == config.ExitModeDual
	var warpClientKey KeyPair
	if dualMode {
		warpClientKey = KeyPair{
			PrivateKey: opts.WarpClientPrivateKey,
			PublicKey:  opts.WarpClientPublicKey,
		}
		if warpClientKey.PrivateKey == "" || warpClientKey.PublicKey == "" {
			var err error
			warpClientKey, err = GenerateKeyPair()
			if err != nil {
				return Plan{}, err
			}
		}
	}

	var serverIP, clientIP, warpClientIP string
	var err error
	if dualMode {
		serverIP, clientIP, warpClientIP, err = AllocateTriple(cfg, opts.CIDR)
		if err != nil {
			return Plan{}, err
		}
	} else {
		serverIP, clientIP, err = AllocatePair(cfg, opts.CIDR)
		if err != nil {
			return Plan{}, err
		}
	}

	device := SafeDeviceName(opts.Node.Name)
	plan := Plan{
		Device:               device,
		ListenPort:           opts.ListenPort,
		Endpoint:             FormatEndpoint(opts.Endpoint, opts.EndpointPort),
		EgressInterface:      opts.EgressInterface,
		EnableForwarding:     opts.EnableForwarding,
		DualMode:             dualMode,
		ServerPrivateKey:     serverKey.PrivateKey,
		ServerPublicKey:      serverKey.PublicKey,
		ClientPrivateKey:     clientKey.PrivateKey,
		ClientPublicKey:      clientKey.PublicKey,
		WarpClientPrivateKey: warpClientKey.PrivateKey,
		WarpClientPublicKey:  warpClientKey.PublicKey,
	}
	if dualMode {
		plan.ServerAddress = serverIP + "/29"
		plan.ClientAddress = clientIP + "/32"
		plan.WarpClientAddress = warpClientIP + "/32"
	} else {
		plan.ServerAddress = serverIP + "/30"
		plan.ClientAddress = clientIP + "/30"
	}

	plan.ServerConfig = RenderServerConfig(plan)
	plan.ClientConfig = RenderClientConfig(plan)
	if dualMode {
		plan.WarpClientConfig = RenderWarpClientConfig(plan)
	}
	return plan, nil
}

func FormatEndpoint(host string, port int) string {
	if strings.Contains(host, ":") {
		if _, _, err := net.SplitHostPort(host); err == nil {
			return host
		}
		if ip := net.ParseIP(host); ip != nil {
			return net.JoinHostPort(host, fmt.Sprintf("%d", port))
		}
	}
	return fmt.Sprintf("%s:%d", host, port)
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
	node.WGWarpClientAddress = plan.WarpClientAddress
	node.WGWarpClientPublicKey = plan.WarpClientPublicKey
	node.WGWarpClientPrivateKey = plan.WarpClientPrivateKey
	node.WGWarpClientConfig = plan.WarpClientConfig
	node.Endpoint = plan.Endpoint
	return node
}

func RenderServerConfig(plan Plan) string {
	clientIP := addressIP(plan.ClientAddress)
	var hooks string
	if plan.EnableForwarding {
		hooks = RenderDirectForwardingHooks(plan)
	}
	configText := fmt.Sprintf(`[Interface]
PrivateKey = %s
Address = %s
ListenPort = %d
SaveConfig = false
%s

[Peer]
PublicKey = %s
AllowedIPs = %s/32
`, plan.ServerPrivateKey, plan.ServerAddress, plan.ListenPort, hooks, plan.ClientPublicKey, clientIP)
	if plan.DualMode {
		warpClientIP := addressIP(plan.WarpClientAddress)
		configText += fmt.Sprintf(`
[Peer]
PublicKey = %s
AllowedIPs = %s/32
`, plan.WarpClientPublicKey, warpClientIP)
	}
	return configText
}

func RenderClientConfig(plan Plan) string {
	return renderClientConfig(plan.ClientPrivateKey, plan.ServerPublicKey, plan.Endpoint, plan.ClientAddress, plan.ServerAddress)
}

func RenderWarpClientConfig(plan Plan) string {
	return renderClientConfig(plan.WarpClientPrivateKey, plan.ServerPublicKey, plan.Endpoint, plan.WarpClientAddress, plan.ServerAddress)
}

func renderClientConfig(privateKey string, serverPublicKey string, endpoint string, clientAddress string, serverAddress string) string {
	serverIP := addressIP(serverAddress)
	return fmt.Sprintf(`[Interface]
PrivateKey = %s
Address = %s

[Peer]
PublicKey = %s
Endpoint = %s
AllowedIPs = %s/32
PersistentKeepalive = 25
`, privateKey, clientAddress, serverPublicKey, endpoint, serverIP)
}

func SafeDeviceName(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			if b.Len() > 0 {
				b.WriteRune('-')
			}
		}
	}
	safe := strings.Trim(b.String(), "-")
	for strings.Contains(safe, "--") {
		safe = strings.ReplaceAll(safe, "--", "-")
	}
	if safe == "" {
		return "wpnode-" + shortHash(name)
	}
	out := "wp" + safe
	if len(out) > 15 {
		suffix := shortHash(name)
		prefixLen := 15 - len(suffix) - 1
		out = strings.TrimRight(out[:prefixLen], "-") + "-" + suffix
	}
	return out
}

func shortHash(value string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(value))
	return fmt.Sprintf("%04x", h.Sum32()&0xffff)
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
		markUsedAddresses(used, node)
	}
	for _, token := range cfg.Tokens {
		if token.Used {
			continue
		}
		markUsedAddresses(used, token.Node)
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

func AllocateTriple(cfg config.Config, cidr string) (string, string, string, error) {
	ip, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", "", "", fmt.Errorf("parse wireguard CIDR: %w", err)
	}
	ip4 := ip.To4()
	if ip4 == nil {
		return "", "", "", fmt.Errorf("only IPv4 wireguard CIDR is supported: %s", cidr)
	}

	used := map[string]bool{}
	for _, node := range cfg.Nodes {
		markUsedAddresses(used, node)
	}
	for _, token := range cfg.Tokens {
		if token.Used {
			continue
		}
		markUsedAddresses(used, token.Node)
	}

	networkIP := ipNet.IP.To4()
	if networkIP == nil {
		return "", "", "", fmt.Errorf("only IPv4 wireguard CIDR is supported: %s", cidr)
	}
	base := uint32(networkIP[0])<<24 | uint32(networkIP[1])<<16 | uint32(networkIP[2])<<8 | uint32(networkIP[3])
	for offset := uint32(0); offset < 1<<16; offset += 8 {
		server := uint32ToIP(base + offset + 1)
		client := uint32ToIP(base + offset + 2)
		warpClient := uint32ToIP(base + offset + 3)
		if !ipNet.Contains(net.ParseIP(server)) || !ipNet.Contains(net.ParseIP(client)) || !ipNet.Contains(net.ParseIP(warpClient)) {
			break
		}
		if used[server] || used[client] || used[warpClient] {
			continue
		}
		return server, client, warpClient, nil
	}

	return "", "", "", fmt.Errorf("no available dual wireguard address block in %s", cidr)
}

func markUsedAddresses(used map[string]bool, node config.Node) {
	markUsedCIDRAddresses(used, node.WGServerAddress)
	for _, value := range []string{node.WGClientAddress, node.WGWarpClientAddress, node.WGAddress} {
		if value == "" {
			continue
		}
		host := addressIP(value)
		used[host] = true
	}
}

func markUsedCIDRAddresses(used map[string]bool, cidr string) {
	if strings.TrimSpace(cidr) == "" {
		return
	}
	ip, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		used[addressIP(cidr)] = true
		return
	}
	ip4 := ip.To4()
	networkIP := ipNet.IP.To4()
	if ip4 == nil || networkIP == nil {
		used[addressIP(cidr)] = true
		return
	}
	ones, bits := ipNet.Mask.Size()
	if bits != 32 || ones < 24 {
		used[addressIP(cidr)] = true
		return
	}
	base := uint32(networkIP[0])<<24 | uint32(networkIP[1])<<16 | uint32(networkIP[2])<<8 | uint32(networkIP[3])
	size := uint32(1) << uint32(32-ones)
	for offset := uint32(1); offset+1 < size; offset++ {
		used[uint32ToIP(base+offset)] = true
	}
}

func uint32ToIP(value uint32) string {
	return net.IPv4(byte(value>>24), byte(value>>16), byte(value>>8), byte(value)).String()
}

func RenderDirectForwardingHooks(plan Plan) string {
	clientIP := addressIP(plan.ClientAddress)
	if plan.EgressInterface == "" {
		return fmt.Sprintf("PostUp = sysctl -w net.ipv4.ip_forward=1; iptables -C FORWARD -i %%i -j ACCEPT 2>/dev/null || iptables -A FORWARD -i %%i -j ACCEPT; iptables -C FORWARD -o %%i -m state --state RELATED,ESTABLISHED -j ACCEPT 2>/dev/null || iptables -A FORWARD -o %%i -m state --state RELATED,ESTABLISHED -j ACCEPT\nPostDown = iptables -D FORWARD -i %%i -j ACCEPT 2>/dev/null || true; iptables -D FORWARD -o %%i -m state --state RELATED,ESTABLISHED -j ACCEPT 2>/dev/null || true")
	}

	return fmt.Sprintf("PostUp = sysctl -w net.ipv4.ip_forward=1; iptables -C FORWARD -i %%i -j ACCEPT 2>/dev/null || iptables -A FORWARD -i %%i -j ACCEPT; iptables -C FORWARD -o %%i -m state --state RELATED,ESTABLISHED -j ACCEPT 2>/dev/null || iptables -A FORWARD -o %%i -m state --state RELATED,ESTABLISHED -j ACCEPT; iptables -t nat -C POSTROUTING -s %s/32 -o %s -j MASQUERADE 2>/dev/null || iptables -t nat -A POSTROUTING -s %s/32 -o %s -j MASQUERADE\nPostDown = iptables -D FORWARD -i %%i -j ACCEPT 2>/dev/null || true; iptables -D FORWARD -o %%i -m state --state RELATED,ESTABLISHED -j ACCEPT 2>/dev/null || true; iptables -t nat -D POSTROUTING -s %s/32 -o %s -j MASQUERADE 2>/dev/null || true", clientIP, plan.EgressInterface, clientIP, plan.EgressInterface, clientIP, plan.EgressInterface)
}

func addressIP(value string) string {
	return strings.Split(strings.TrimSpace(value), "/")[0]
}
