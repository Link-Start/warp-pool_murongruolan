package wireguard

import (
	"fmt"
	"hash/fnv"
	"math/big"
	"net"
	"strings"

	"github.com/murongruolan/warp-pool/internal/config"
)

const DefaultListenPort = 51820

type Plan struct {
	Device                string
	ListenPort            int
	Endpoint              string
	EgressInterface       string
	EnableForwarding      bool
	EnableIPv6Forwarding  bool
	IPv6Enabled           bool
	DualMode              bool
	ServerAddress         string
	ClientAddress         string
	ServerIPv6Address     string
	ClientIPv6Address     string
	WarpClientAddress     string
	WarpClientIPv6Address string
	ServerPrivateKey      string
	ServerPublicKey       string
	ClientPrivateKey      string
	ClientPublicKey       string
	WarpClientPrivateKey  string
	WarpClientPublicKey   string
	ServerConfig          string
	ClientConfig          string
	WarpClientConfig      string
}

type Options struct {
	Node                 config.Node
	CIDR                 string
	CIDR6                string
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
	if opts.CIDR6 == "" {
		opts.CIDR6 = cfg.Defaults.CIDR6
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

	ipv6Enabled := endpointIsIPv6(opts.Endpoint)
	if opts.Node.WGServerIPv6Address != "" || opts.Node.WGClientIPv6Address != "" || opts.Node.WGWarpClientIPv6Address != "" {
		ipv6Enabled = true
	}
	var serverIP, clientIP, warpClientIP string
	var serverIP6, clientIP6, warpClientIP6 string
	var err error
	if dualMode {
		serverIP, clientIP, warpClientIP, err = AllocateTriple(cfg, opts.CIDR)
		if err != nil {
			return Plan{}, err
		}
		if ipv6Enabled && (opts.Node.WGServerIPv6Address == "" || opts.Node.WGClientIPv6Address == "" || opts.Node.WGWarpClientIPv6Address == "") {
			serverIP6, clientIP6, warpClientIP6, err = AllocateIPv6Triple(cfg, opts.CIDR6)
			if err != nil {
				return Plan{}, err
			}
		} else {
			serverIP6 = addressIP(opts.Node.WGServerIPv6Address)
			clientIP6 = addressIP(opts.Node.WGClientIPv6Address)
			warpClientIP6 = addressIP(opts.Node.WGWarpClientIPv6Address)
		}
	} else {
		serverIP, clientIP, err = AllocatePair(cfg, opts.CIDR)
		if err != nil {
			return Plan{}, err
		}
		if ipv6Enabled && (opts.Node.WGServerIPv6Address == "" || opts.Node.WGClientIPv6Address == "") {
			serverIP6, clientIP6, err = AllocateIPv6Pair(cfg, opts.CIDR6)
			if err != nil {
				return Plan{}, err
			}
		} else {
			serverIP6 = addressIP(opts.Node.WGServerIPv6Address)
			clientIP6 = addressIP(opts.Node.WGClientIPv6Address)
		}
	}

	device := SafeDeviceName(opts.Node.Name)
	plan := Plan{
		Device:               device,
		ListenPort:           opts.ListenPort,
		Endpoint:             FormatEndpoint(opts.Endpoint, opts.EndpointPort),
		EgressInterface:      opts.EgressInterface,
		EnableForwarding:     opts.EnableForwarding,
		EnableIPv6Forwarding: opts.EnableForwarding && ipv6Enabled,
		IPv6Enabled:          ipv6Enabled,
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
		if ipv6Enabled {
			plan.ServerIPv6Address = serverIP6 + "/125"
			plan.ClientIPv6Address = clientIP6 + "/128"
			plan.WarpClientIPv6Address = warpClientIP6 + "/128"
		}
	} else {
		plan.ServerAddress = serverIP + "/30"
		plan.ClientAddress = clientIP + "/30"
		if ipv6Enabled {
			plan.ServerIPv6Address = serverIP6 + "/126"
			plan.ClientIPv6Address = clientIP6 + "/126"
		}
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

func endpointIsIPv6(endpoint string) bool {
	host := strings.TrimSpace(endpoint)
	if host == "" {
		return false
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	ip := net.ParseIP(host)
	return ip != nil && ip.To4() == nil
}

func ApplyPlan(node config.Node, plan Plan) config.Node {
	node.WGDevice = plan.Device
	node.WGAddress = plan.ClientAddress
	node.WGServerAddress = plan.ServerAddress
	node.WGClientAddress = plan.ClientAddress
	node.WGServerIPv6Address = plan.ServerIPv6Address
	node.WGClientIPv6Address = plan.ClientIPv6Address
	node.WGListenPort = plan.ListenPort
	node.WGServerPublicKey = plan.ServerPublicKey
	node.WGClientPublicKey = plan.ClientPublicKey
	node.WGClientPrivateKey = plan.ClientPrivateKey
	node.WGClientConfig = plan.ClientConfig
	node.WGWarpClientAddress = plan.WarpClientAddress
	node.WGWarpClientIPv6Address = plan.WarpClientIPv6Address
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
	addresses := plan.ServerAddress
	allowedIPs := fmt.Sprintf("%s/32", clientIP)
	if plan.IPv6Enabled {
		addresses += ", " + plan.ServerIPv6Address
		allowedIPs += fmt.Sprintf(", %s/128", addressIP(plan.ClientIPv6Address))
	}
	configText := fmt.Sprintf(`[Interface]
PrivateKey = %s
Address = %s
ListenPort = %d
SaveConfig = false
%s

[Peer]
PublicKey = %s
AllowedIPs = %s
`, plan.ServerPrivateKey, addresses, plan.ListenPort, hooks, plan.ClientPublicKey, allowedIPs)
	if plan.DualMode {
		warpClientIP := addressIP(plan.WarpClientAddress)
		warpAllowedIPs := fmt.Sprintf("%s/32", warpClientIP)
		if plan.IPv6Enabled {
			warpAllowedIPs += fmt.Sprintf(", %s/128", addressIP(plan.WarpClientIPv6Address))
		}
		configText += fmt.Sprintf(`
[Peer]
PublicKey = %s
AllowedIPs = %s
`, plan.WarpClientPublicKey, warpAllowedIPs)
	}
	return configText
}

func RenderClientConfig(plan Plan) string {
	return renderClientConfig(plan.ClientPrivateKey, plan.ServerPublicKey, plan.Endpoint, plan.ClientAddress, plan.ClientIPv6Address, plan.ServerAddress, plan.ServerIPv6Address)
}

func RenderWarpClientConfig(plan Plan) string {
	return renderClientConfig(plan.WarpClientPrivateKey, plan.ServerPublicKey, plan.Endpoint, plan.WarpClientAddress, plan.WarpClientIPv6Address, plan.ServerAddress, plan.ServerIPv6Address)
}

func renderClientConfig(privateKey string, serverPublicKey string, endpoint string, clientAddress string, clientIPv6Address string, serverAddress string, serverIPv6Address string) string {
	serverIP := addressIP(serverAddress)
	addresses := clientAddress
	allowedIPs := fmt.Sprintf("%s/32", serverIP)
	if clientIPv6Address != "" && serverIPv6Address != "" {
		addresses += ", " + clientIPv6Address
		allowedIPs += fmt.Sprintf(", %s/128", addressIP(serverIPv6Address))
	}
	return fmt.Sprintf(`[Interface]
PrivateKey = %s
Address = %s

[Peer]
PublicKey = %s
Endpoint = %s
AllowedIPs = %s
PersistentKeepalive = 25
`, privateKey, addresses, serverPublicKey, endpoint, allowedIPs)
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

func AllocateIPv6Pair(cfg config.Config, cidr string) (string, string, error) {
	addresses, err := allocateIPv6Block(cfg, cidr, 4, 2)
	if err != nil {
		return "", "", err
	}
	return addresses[0], addresses[1], nil
}

func AllocateIPv6Triple(cfg config.Config, cidr string) (string, string, string, error) {
	addresses, err := allocateIPv6Block(cfg, cidr, 8, 3)
	if err != nil {
		return "", "", "", err
	}
	return addresses[0], addresses[1], addresses[2], nil
}

func allocateIPv6Block(cfg config.Config, cidr string, blockSize uint64, count int) ([]string, error) {
	ip, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, fmt.Errorf("parse wireguard IPv6 CIDR: %w", err)
	}
	if ip.To4() != nil || ip.To16() == nil {
		return nil, fmt.Errorf("only IPv6 wireguard CIDR is supported: %s", cidr)
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

	networkIP := ipNet.IP.To16()
	if networkIP == nil || networkIP.To4() != nil {
		return nil, fmt.Errorf("only IPv6 wireguard CIDR is supported: %s", cidr)
	}
	for block := uint64(0); block < 65536; block++ {
		offset := block * blockSize
		addresses := make([]string, 0, count)
		ok := true
		for index := 1; index <= count; index++ {
			candidate := addIPv6(networkIP, offset+uint64(index))
			parsed := net.ParseIP(candidate)
			if parsed == nil || !ipNet.Contains(parsed) || used[candidate] {
				ok = false
				break
			}
			addresses = append(addresses, candidate)
		}
		if ok {
			return addresses, nil
		}
	}
	return nil, fmt.Errorf("no available wireguard IPv6 address block in %s", cidr)
}

func addIPv6(base net.IP, offset uint64) string {
	value := new(big.Int).SetBytes(base.To16())
	value.Add(value, new(big.Int).SetUint64(offset))
	return net.IP(value.FillBytes(make([]byte, net.IPv6len))).String()
}

func markUsedAddresses(used map[string]bool, node config.Node) {
	markUsedCIDRAddresses(used, node.WGServerAddress)
	markUsedCIDRAddresses(used, node.WGServerIPv6Address)
	for _, value := range []string{node.WGClientAddress, node.WGClientIPv6Address, node.WGWarpClientAddress, node.WGWarpClientIPv6Address, node.WGAddress} {
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
	clientIP6 := addressIP(plan.ClientIPv6Address)
	ipv4Up := "sysctl -w net.ipv4.ip_forward=1; iptables -C FORWARD -i %i -j ACCEPT 2>/dev/null || iptables -A FORWARD -i %i -j ACCEPT; iptables -C FORWARD -o %i -m state --state RELATED,ESTABLISHED -j ACCEPT 2>/dev/null || iptables -A FORWARD -o %i -m state --state RELATED,ESTABLISHED -j ACCEPT"
	ipv4Down := "iptables -D FORWARD -i %i -j ACCEPT 2>/dev/null || true; iptables -D FORWARD -o %i -m state --state RELATED,ESTABLISHED -j ACCEPT 2>/dev/null || true"
	ipv6Up := "sysctl -w net.ipv6.conf.all.forwarding=1; ip6tables -C FORWARD -i %i -j ACCEPT 2>/dev/null || ip6tables -A FORWARD -i %i -j ACCEPT; ip6tables -C FORWARD -o %i -m state --state RELATED,ESTABLISHED -j ACCEPT 2>/dev/null || ip6tables -A FORWARD -o %i -m state --state RELATED,ESTABLISHED -j ACCEPT"
	ipv6Down := "ip6tables -D FORWARD -i %i -j ACCEPT 2>/dev/null || true; ip6tables -D FORWARD -o %i -m state --state RELATED,ESTABLISHED -j ACCEPT 2>/dev/null || true"
	if !plan.EnableIPv6Forwarding {
		if plan.EgressInterface == "" {
			return fmt.Sprintf("PostUp = %s\nPostDown = %s", ipv4Up, ipv4Down)
		}
		return fmt.Sprintf("PostUp = %s; iptables -t nat -C POSTROUTING -s %s/32 -o %s -j MASQUERADE 2>/dev/null || iptables -t nat -A POSTROUTING -s %s/32 -o %s -j MASQUERADE\nPostDown = %s; iptables -t nat -D POSTROUTING -s %s/32 -o %s -j MASQUERADE 2>/dev/null || true", ipv4Up, clientIP, plan.EgressInterface, clientIP, plan.EgressInterface, ipv4Down, clientIP, plan.EgressInterface)
	}
	if plan.EgressInterface == "" {
		return fmt.Sprintf("PostUp = %s; %s\nPostDown = %s; %s", ipv4Up, ipv6Up, ipv4Down, ipv6Down)
	}

	return fmt.Sprintf("PostUp = %s; iptables -t nat -C POSTROUTING -s %s/32 -o %s -j MASQUERADE 2>/dev/null || iptables -t nat -A POSTROUTING -s %s/32 -o %s -j MASQUERADE; %s; ip6tables -t nat -C POSTROUTING -s %s/128 -o %s -j MASQUERADE 2>/dev/null || ip6tables -t nat -A POSTROUTING -s %s/128 -o %s -j MASQUERADE\nPostDown = %s; iptables -t nat -D POSTROUTING -s %s/32 -o %s -j MASQUERADE 2>/dev/null || true; %s; ip6tables -t nat -D POSTROUTING -s %s/128 -o %s -j MASQUERADE 2>/dev/null || true", ipv4Up, clientIP, plan.EgressInterface, clientIP, plan.EgressInterface, ipv6Up, clientIP6, plan.EgressInterface, clientIP6, plan.EgressInterface, ipv4Down, clientIP, plan.EgressInterface, ipv6Down, clientIP6, plan.EgressInterface)
}

func addressIP(value string) string {
	return strings.Split(strings.TrimSpace(value), "/")[0]
}
