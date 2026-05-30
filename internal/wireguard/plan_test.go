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
