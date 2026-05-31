package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestValidatePort(t *testing.T) {
	tests := []struct {
		name string
		port int
		ok   bool
	}{
		{name: "min", port: 1, ok: true},
		{name: "max", port: 65535, ok: true},
		{name: "zero", port: 0, ok: false},
		{name: "too high", port: 65536, ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePort(tt.port)
			if tt.ok && err != nil {
				t.Fatalf("expected valid port, got %v", err)
			}
			if !tt.ok && err == nil {
				t.Fatal("expected invalid port")
			}
		})
	}
}

func TestValidateProxy(t *testing.T) {
	for _, proxy := range []string{ProxySocks5, ProxyHTTP, ProxyMixed} {
		if err := ValidateProxy(proxy); err != nil {
			t.Fatalf("expected valid proxy %s: %v", proxy, err)
		}
	}

	if err := ValidateProxy("mixin"); err == nil {
		t.Fatal("expected invalid proxy")
	}
}

func TestValidateExitMode(t *testing.T) {
	for _, mode := range []string{ExitModeDirect, ExitModeWarp} {
		if err := ValidateExitMode(mode); err != nil {
			t.Fatalf("expected valid mode %s: %v", mode, err)
		}
	}

	if err := ValidateExitMode("warp-only"); err == nil {
		t.Fatal("expected invalid exit mode")
	}
}

func TestValidateNodeRejectsDuplicatePort(t *testing.T) {
	cfg := Default()
	cfg.Nodes = append(cfg.Nodes, Node{
		Name:      "nat1",
		ExitMode:  ExitModeDirect,
		Proxy:     ProxyMixed,
		BindHost:  "127.0.0.1",
		LocalPort: 10013,
	})

	node := Node{
		Name:      "nat2",
		ExitMode:  ExitModeWarp,
		Proxy:     ProxyMixed,
		BindHost:  "127.0.0.1",
		LocalPort: 10013,
	}

	if err := ValidateNode(cfg, node); err == nil {
		t.Fatal("expected duplicate port error")
	}
}

func TestAddNodeAppliesDefaults(t *testing.T) {
	cfg := Default()
	node := Node{
		Name:      "nat1",
		LocalPort: 10013,
	}

	next, err := AddNode(cfg, node)
	if err != nil {
		t.Fatalf("add node: %v", err)
	}

	if len(next.Nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(next.Nodes))
	}

	got := next.Nodes[0]
	if got.ExitMode != ExitModeDirect {
		t.Fatalf("expected default exit mode %s, got %s", ExitModeDirect, got.ExitMode)
	}
	if got.Proxy != ProxyMixed {
		t.Fatalf("expected default proxy %s, got %s", ProxyMixed, got.Proxy)
	}
	if got.BindHost != "127.0.0.1" {
		t.Fatalf("expected default bind host, got %s", got.BindHost)
	}
	if got.CreatedAt == "" || got.LastUpdated == "" {
		t.Fatal("expected timestamps")
	}
}

func TestFindAndRemoveNode(t *testing.T) {
	cfg := Default()
	var err error
	cfg, err = AddNode(cfg, Node{Name: "nat1", LocalPort: 10013})
	if err != nil {
		t.Fatalf("add node: %v", err)
	}

	if _, ok := FindNode(cfg, "nat1"); !ok {
		t.Fatal("expected node to exist")
	}

	next, removed, err := RemoveNode(cfg, "nat1")
	if err != nil {
		t.Fatalf("remove node: %v", err)
	}
	if removed.Name != "nat1" {
		t.Fatalf("unexpected removed node: %s", removed.Name)
	}
	if len(next.Nodes) != 0 {
		t.Fatalf("expected no nodes, got %d", len(next.Nodes))
	}
}

func TestUpdateNode(t *testing.T) {
	cfg := Default()
	var err error
	cfg, err = AddNode(cfg, Node{Name: "nat1", LocalPort: 10013})
	if err != nil {
		t.Fatalf("add node: %v", err)
	}

	node, ok := FindNode(cfg, "nat1")
	if !ok {
		t.Fatal("expected node")
	}
	node.WGLocalDevice = "wpnat1-cli"

	next, err := UpdateNode(cfg, node)
	if err != nil {
		t.Fatalf("update node: %v", err)
	}
	got, ok := FindNode(next, "nat1")
	if !ok {
		t.Fatal("expected updated node")
	}
	if got.WGLocalDevice != "wpnat1-cli" {
		t.Fatalf("unexpected local device: %s", got.WGLocalDevice)
	}
	if got.CreatedAt == "" || got.LastUpdated == "" {
		t.Fatal("expected timestamps")
	}
}

func TestDeployTokenLifecycle(t *testing.T) {
	cfg := Default()
	cfg.Listen.Enabled = true

	var err error
	cfg, err = AddDeployToken(cfg, DeployToken{
		Token:     "token-1",
		ExpiresAt: time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
		Node: Node{
			Name:      "nat1",
			ExitMode:  ExitModeDirect,
			Proxy:     ProxyMixed,
			BindHost:  "127.0.0.1",
			LocalPort: 10013,
		},
	})
	if err != nil {
		t.Fatalf("add token: %v", err)
	}

	next, node, err := UseDeployToken(cfg, "token-1", time.Now().UTC())
	if err != nil {
		t.Fatalf("use token: %v", err)
	}
	if node.Name != "nat1" {
		t.Fatalf("unexpected node: %s", node.Name)
	}
	if len(next.Nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(next.Nodes))
	}
	if !next.Tokens[0].Used || !next.Tokens[0].Registered {
		t.Fatal("expected token to be used and registered")
	}

	if _, _, err := UseDeployToken(next, "token-1", time.Now().UTC()); err == nil {
		t.Fatal("expected used token error")
	}
}

func TestDeployTokenRejectsExpired(t *testing.T) {
	cfg := Default()
	cfg, err := AddDeployToken(cfg, DeployToken{
		Token:     "token-1",
		ExpiresAt: time.Now().UTC().Add(-time.Hour).Format(time.RFC3339),
		Node: Node{
			Name:      "nat1",
			ExitMode:  ExitModeDirect,
			Proxy:     ProxyMixed,
			BindHost:  "127.0.0.1",
			LocalPort: 10013,
		},
	})
	if err != nil {
		t.Fatalf("add token: %v", err)
	}

	if _, _, err := UseDeployToken(cfg, "token-1", time.Now().UTC()); err == nil {
		t.Fatal("expected expired token error")
	}
}

func TestPrepareAndCompleteDeployToken(t *testing.T) {
	cfg := Default()
	cfg, err := AddDeployToken(cfg, DeployToken{
		Token:     "token-1",
		ExpiresAt: time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
		Node: Node{
			Name:      "nat1",
			ExitMode:  ExitModeDirect,
			Proxy:     ProxyMixed,
			BindHost:  "127.0.0.1",
			LocalPort: 10013,
		},
	})
	if err != nil {
		t.Fatalf("add token: %v", err)
	}

	prepared := cfg.Tokens[0].Node
	prepared.WGDevice = "wpnat1"
	prepared.WGClientConfig = "client"
	cfg, err = PrepareDeployToken(cfg, "token-1", prepared, time.Now().UTC())
	if err != nil {
		t.Fatalf("prepare token: %v", err)
	}
	if !cfg.Tokens[0].Prepared {
		t.Fatal("expected token prepared")
	}

	next, node, err := CompleteDeployToken(cfg, "token-1", time.Now().UTC())
	if err != nil {
		t.Fatalf("complete token: %v", err)
	}
	if node.WGDevice != "wpnat1" {
		t.Fatalf("unexpected node: %#v", node)
	}
	if len(next.Nodes) != 1 || !next.Tokens[0].Used || !next.Tokens[0].Registered {
		t.Fatalf("unexpected completed config: %#v", next)
	}
}

func TestRemoveDeployTokens(t *testing.T) {
	cfg := Default()
	expiresAt := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	cfg, err := AddDeployToken(cfg, DeployToken{
		Token:     "token-1",
		ExpiresAt: expiresAt,
		Node: Node{
			Name:      "nat1",
			ExitMode:  ExitModeDirect,
			Proxy:     ProxyMixed,
			BindHost:  "127.0.0.1",
			LocalPort: 10013,
		},
	})
	if err != nil {
		t.Fatalf("add token: %v", err)
	}
	cfg, err = AddDeployToken(cfg, DeployToken{
		Token:     "token-2",
		ExpiresAt: expiresAt,
		Node: Node{
			Name:      "nat2",
			ExitMode:  ExitModeDirect,
			Proxy:     ProxyMixed,
			BindHost:  "127.0.0.1",
			LocalPort: 10014,
		},
	})
	if err != nil {
		t.Fatalf("add token: %v", err)
	}
	cfg.Tokens[1].Used = true

	next, removed := RemoveDeployTokens(cfg, "nat1", false)
	if removed != 1 || len(next.Tokens) != 1 || next.Tokens[0].Token != "token-2" {
		t.Fatalf("unexpected remove result: removed=%d tokens=%#v", removed, next.Tokens)
	}

	next, removed = RemoveDeployTokens(next, "token-2", false)
	if removed != 0 || len(next.Tokens) != 1 {
		t.Fatalf("used token should be kept without includeUsed: removed=%d tokens=%#v", removed, next.Tokens)
	}

	next, removed = RemoveDeployTokens(next, "token-2", true)
	if removed != 1 || len(next.Tokens) != 0 {
		t.Fatalf("used token should be removed with includeUsed: removed=%d tokens=%#v", removed, next.Tokens)
	}
}

func TestPruneExpiredDeployTokens(t *testing.T) {
	cfg := Default()
	cfg.Tokens = []DeployToken{
		{
			Token:     "expired",
			ExpiresAt: time.Now().UTC().Add(-time.Hour).Format(time.RFC3339),
			Node:      Node{Name: "old"},
		},
		{
			Token:     "used-expired",
			ExpiresAt: time.Now().UTC().Add(-time.Hour).Format(time.RFC3339),
			Used:      true,
			Node:      Node{Name: "used"},
		},
		{
			Token:     "active",
			ExpiresAt: time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
			Node:      Node{Name: "active"},
		},
	}

	next, removed := PruneExpiredDeployTokens(cfg, time.Now().UTC())
	if removed != 1 || len(next.Tokens) != 2 {
		t.Fatalf("unexpected prune result: removed=%d tokens=%#v", removed, next.Tokens)
	}
	if next.Tokens[0].Token != "used-expired" || next.Tokens[1].Token != "active" {
		t.Fatalf("unexpected remaining tokens: %#v", next.Tokens)
	}
}

func TestLoadRejectsOpenPermissionsOnUnix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows permissions differ")
	}
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"version":1}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(path); err == nil {
		t.Fatal("expected open permissions error")
	}
}
