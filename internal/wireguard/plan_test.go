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
