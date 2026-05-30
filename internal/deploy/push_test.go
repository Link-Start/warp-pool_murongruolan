package deploy

import (
	"strings"
	"testing"

	"github.com/murongruolan/warp-pool/internal/config"
)

func TestPushDryRunAddsNode(t *testing.T) {
	cfg := config.Default()
	next, result, err := Push(cfg, PushOptions{
		DryRun:        true,
		SkipPortCheck: true,
		WGEndpoint:    "203.0.113.1",
		Node: config.Node{
			Name:      "nat1",
			ExitMode:  config.ExitModeDirect,
			Proxy:     config.ProxyMixed,
			BindHost:  "127.0.0.1",
			LocalPort: 10013,
		},
	})
	if err != nil {
		t.Fatalf("push dry-run: %v", err)
	}
	if len(next.Nodes) != 0 {
		t.Fatalf("expected dry-run to skip saving nodes, got %d", len(next.Nodes))
	}
	if !containsLog(result.Logs, "dry-run: skip ssh connect") {
		t.Fatalf("expected dry-run log, got %#v", result.Logs)
	}
}

func TestPushDryRunRejectsDuplicatePort(t *testing.T) {
	cfg := config.Default()
	var err error
	cfg, err = config.AddNode(cfg, config.Node{
		Name:      "nat1",
		ExitMode:  config.ExitModeDirect,
		Proxy:     config.ProxyMixed,
		BindHost:  "127.0.0.1",
		LocalPort: 10013,
	})
	if err != nil {
		t.Fatalf("add node: %v", err)
	}

	_, _, err = Push(cfg, PushOptions{
		DryRun:        true,
		SkipPortCheck: true,
		WGEndpoint:    "203.0.113.1",
		Node: config.Node{
			Name:      "nat2",
			ExitMode:  config.ExitModeWarp,
			Proxy:     config.ProxyMixed,
			BindHost:  "127.0.0.1",
			LocalPort: 10013,
		},
	})
	if err == nil {
		t.Fatal("expected duplicate port error")
	}
}

func containsLog(logs []string, want string) bool {
	for _, item := range logs {
		if strings.Contains(item, want) {
			return true
		}
	}
	return false
}
