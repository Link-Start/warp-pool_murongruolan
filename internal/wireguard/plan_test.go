package wireguard

import (
	"strings"
	"testing"

	"github.com/murongruolan/warp-pool/internal/config"
)

func TestAllocatePair(t *testing.T) {
	cfg := config.Default()
	server, client, err := AllocatePair(cfg, "10.200.0.0/16")
	if err != nil {
		t.Fatalf("allocate pair: %v", err)
	}
	if server != "10.200.0.1" || client != "10.200.0.2" {
		t.Fatalf("unexpected pair: %s %s", server, client)
	}

	cfg.Nodes = append(cfg.Nodes, config.Node{
		WGServerAddress: "10.200.0.1/30",
		WGClientAddress: "10.200.0.2/30",
	})
	server, client, err = AllocatePair(cfg, "10.200.0.0/16")
	if err != nil {
		t.Fatalf("allocate second pair: %v", err)
	}
	if server != "10.200.0.5" || client != "10.200.0.6" {
		t.Fatalf("unexpected second pair: %s %s", server, client)
	}
}

func TestAllocatePairSkipsPreparedDeployTokenAddresses(t *testing.T) {
	cfg := config.Default()
	cfg.Tokens = append(cfg.Tokens, config.DeployToken{
		Token:    "token-1",
		Prepared: true,
		Node: config.Node{
			WGServerAddress: "10.200.0.1/30",
			WGClientAddress: "10.200.0.2/30",
		},
	})

	server, client, err := AllocatePair(cfg, "10.200.0.0/16")
	if err != nil {
		t.Fatalf("allocate pair: %v", err)
	}
	if server != "10.200.0.5" || client != "10.200.0.6" {
		t.Fatalf("unexpected pair: %s %s", server, client)
	}
}

func TestBuildPlan(t *testing.T) {
	cfg := config.Default()
	plan, err := BuildPlan(cfg, Options{
		Node: config.Node{
			Name: "nat-1",
		},
		Endpoint: "203.0.113.1",
	})
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}

	if plan.Device != "wpnat-1" {
		t.Fatalf("unexpected device: %s", plan.Device)
	}
	if !strings.Contains(plan.ServerConfig, "ListenPort = 51820") {
		t.Fatalf("server config missing listen port:\n%s", plan.ServerConfig)
	}
	if !strings.Contains(plan.ClientConfig, "Endpoint = 203.0.113.1:51820") {
		t.Fatalf("client config missing endpoint:\n%s", plan.ClientConfig)
	}
}

func TestBuildPlanDualAllocatesTwoClientAddresses(t *testing.T) {
	cfg := config.Default()
	plan, err := BuildPlan(cfg, Options{
		Node: config.Node{
			Name:     "nat-1",
			ExitMode: config.ExitModeDual,
		},
		Endpoint:         "203.0.113.1",
		EgressInterface:  "eth0",
		EnableForwarding: true,
	})
	if err != nil {
		t.Fatalf("build dual plan: %v", err)
	}
	if !plan.DualMode {
		t.Fatal("expected dual mode plan")
	}
	if plan.ServerAddress != "10.200.0.1/29" || plan.ClientAddress != "10.200.0.2/32" || plan.WarpClientAddress != "10.200.0.3/32" {
		t.Fatalf("unexpected dual addresses: server=%s client=%s warp=%s", plan.ServerAddress, plan.ClientAddress, plan.WarpClientAddress)
	}
	if strings.Count(plan.ServerConfig, "[Peer]") != 2 {
		t.Fatalf("expected two peers in dual server config:\n%s", plan.ServerConfig)
	}
	if !strings.Contains(plan.ServerConfig, "iptables -t nat -A POSTROUTING -s 10.200.0.2/32 -o eth0 -j MASQUERADE") {
		t.Fatalf("dual server config should MASQUERADE direct client only:\n%s", plan.ServerConfig)
	}
	if plan.WarpClientConfig == "" || !strings.Contains(plan.WarpClientConfig, "Address = 10.200.0.3/32") {
		t.Fatalf("missing warp client config:\n%s", plan.WarpClientConfig)
	}
}

func TestAllocatePairSkipsDualAddressBlock(t *testing.T) {
	cfg := config.Default()
	cfg.Nodes = append(cfg.Nodes, config.Node{
		WGServerAddress:     "10.200.0.1/29",
		WGClientAddress:     "10.200.0.2/32",
		WGWarpClientAddress: "10.200.0.3/32",
	})
	server, client, err := AllocatePair(cfg, "10.200.0.0/16")
	if err != nil {
		t.Fatalf("allocate pair: %v", err)
	}
	if server != "10.200.0.9" || client != "10.200.0.10" {
		t.Fatalf("expected allocation after dual /29 block, got %s %s", server, client)
	}
}

func TestBuildPlanUsesSeparateEndpointPort(t *testing.T) {
	cfg := config.Default()
	plan, err := BuildPlan(cfg, Options{
		Node: config.Node{
			Name: "nat-1",
		},
		Endpoint:     "203.0.113.1",
		EndpointPort: 30021,
		ListenPort:   51820,
	})
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	if !strings.Contains(plan.ClientConfig, "Endpoint = 203.0.113.1:30021") {
		t.Fatalf("client config missing mapped endpoint port:\n%s", plan.ClientConfig)
	}
	if !strings.Contains(plan.ServerConfig, "ListenPort = 51820") {
		t.Fatalf("server config missing listen port:\n%s", plan.ServerConfig)
	}
}

func TestBuildPlanKeepsExplicitHostPort(t *testing.T) {
	cfg := config.Default()
	plan, err := BuildPlan(cfg, Options{
		Node: config.Node{
			Name: "nat-1",
		},
		Endpoint:   "203.0.113.1:30021",
		ListenPort: 51820,
	})
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	if !strings.Contains(plan.ClientConfig, "Endpoint = 203.0.113.1:30021") {
		t.Fatalf("client config did not keep explicit endpoint:\n%s", plan.ClientConfig)
	}
}

func TestBuildPlanWithIPv6EndpointAddsIPv6Tunnel(t *testing.T) {
	cfg := config.Default()
	plan, err := BuildPlan(cfg, Options{
		Node: config.Node{
			Name: "nat-1",
		},
		Endpoint:         "2001:db8::10",
		EgressInterface:  "eth0",
		EnableForwarding: true,
	})
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	if plan.Endpoint != "[2001:db8::10]:51820" {
		t.Fatalf("unexpected endpoint: %s", plan.Endpoint)
	}
	if plan.ServerIPv6Address != "fd7a:7761:7270::1/126" || plan.ClientIPv6Address != "fd7a:7761:7270::2/126" {
		t.Fatalf("unexpected ipv6 addresses: server=%s client=%s", plan.ServerIPv6Address, plan.ClientIPv6Address)
	}
	for _, want := range []string{
		"Address = 10.200.0.1/30, fd7a:7761:7270::1/126",
		"AllowedIPs = 10.200.0.2/32, fd7a:7761:7270::2/128",
		"Address = 10.200.0.2/30, fd7a:7761:7270::2/126",
		"AllowedIPs = 10.200.0.1/32, fd7a:7761:7270::1/128",
		"ip6tables -t nat -A POSTROUTING -s fd7a:7761:7270::2/128 -o eth0 -j MASQUERADE",
	} {
		if !strings.Contains(plan.ServerConfig+"\n"+plan.ClientConfig, want) {
			t.Fatalf("missing %q:\nserver:\n%s\nclient:\n%s", want, plan.ServerConfig, plan.ClientConfig)
		}
	}
}

func TestBuildPlanDualWithIPv6EndpointAllocatesIPv6WarpAddress(t *testing.T) {
	cfg := config.Default()
	plan, err := BuildPlan(cfg, Options{
		Node: config.Node{
			Name:     "nat-1",
			ExitMode: config.ExitModeDual,
		},
		Endpoint: "2001:db8::10",
	})
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	if plan.ServerIPv6Address != "fd7a:7761:7270::1/125" ||
		plan.ClientIPv6Address != "fd7a:7761:7270::2/128" ||
		plan.WarpClientIPv6Address != "fd7a:7761:7270::3/128" {
		t.Fatalf("unexpected dual ipv6 addresses: server=%s client=%s warp=%s", plan.ServerIPv6Address, plan.ClientIPv6Address, plan.WarpClientIPv6Address)
	}
	if !strings.Contains(plan.WarpClientConfig, "Address = 10.200.0.3/32, fd7a:7761:7270::3/128") {
		t.Fatalf("missing warp ipv6 client config:\n%s", plan.WarpClientConfig)
	}
}

func TestSafeDeviceNameAddsHashWhenTruncated(t *testing.T) {
	first := SafeDeviceName("nat-preflight-1")
	second := SafeDeviceName("nat-preflight-2")
	if first == second {
		t.Fatalf("expected long names to stay unique, both got %s", first)
	}
	if len(first) > 15 || len(second) > 15 {
		t.Fatalf("device names exceed Linux limit: %s %s", first, second)
	}
	if !strings.HasPrefix(first, "wpnat-pref") || !strings.HasPrefix(second, "wpnat-pref") {
		t.Fatalf("unexpected device names: %s %s", first, second)
	}
}

func TestSafeDeviceNameNormalizesNonASCIISeparators(t *testing.T) {
	if got := SafeDeviceName("美国NAT01"); got != "wpnat01" {
		t.Fatalf("unexpected device name: %s", got)
	}
	if got := SafeDeviceName("美国节点"); !strings.HasPrefix(got, "wpnode-") {
		t.Fatalf("unexpected fallback device name: %s", got)
	}
}

func TestBuildPlanWithDirectForwarding(t *testing.T) {
	cfg := config.Default()
	plan, err := BuildPlan(cfg, Options{
		Node: config.Node{
			Name: "nat-1",
		},
		Endpoint:         "203.0.113.1",
		EgressInterface:  "eth0",
		EnableForwarding: true,
	})
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}

	for _, want := range []string{
		"PostUp = sysctl -w net.ipv4.ip_forward=1",
		"iptables -A FORWARD -i %i -j ACCEPT",
		"iptables -t nat -A POSTROUTING -s 10.200.0.2/32 -o eth0 -j MASQUERADE",
		"PostDown = iptables -D FORWARD -i %i -j ACCEPT",
	} {
		if !strings.Contains(plan.ServerConfig, want) {
			t.Fatalf("server config missing %q:\n%s", want, plan.ServerConfig)
		}
	}
}

func TestBuildPlanUsesProvidedKeys(t *testing.T) {
	cfg := config.Default()
	plan, err := BuildPlan(cfg, Options{
		Node: config.Node{
			Name: "nat-1",
		},
		Endpoint:         "203.0.113.1",
		ServerPrivateKey: "server-private",
		ServerPublicKey:  "server-public",
		ClientPrivateKey: "client-private",
		ClientPublicKey:  "client-public",
	})
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}

	if plan.ServerPrivateKey != "server-private" || plan.ServerPublicKey != "server-public" {
		t.Fatalf("server keys not applied: %#v", plan)
	}
	if plan.ClientPrivateKey != "client-private" || plan.ClientPublicKey != "client-public" {
		t.Fatalf("client keys not applied: %#v", plan)
	}
}
