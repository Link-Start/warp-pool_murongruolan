package config

import "testing"

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
