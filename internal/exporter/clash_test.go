package exporter

import (
	"strings"
	"testing"

	"github.com/murongruolan/warp-pool/internal/config"
)

func TestClashExportsMixedAsSocks5(t *testing.T) {
	cfg := config.Default()
	cfg.Nodes = []config.Node{
		{
			Name:      "nat1",
			ExitMode:  config.ExitModeDirect,
			Proxy:     config.ProxyMixed,
			BindHost:  "127.0.0.1",
			LocalPort: 10013,
			Country:   "US",
		},
	}

	out, err := Clash(cfg, ClashOptions{})
	if err != nil {
		t.Fatalf("export clash: %v", err)
	}

	for _, want := range []string{
		`name: "WarpPool-US-nat1"`,
		"type: socks5",
		"server: 127.0.0.1",
		"port: 10013",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected output to contain %q, got:\n%s", want, out)
		}
	}
}

func TestClashAllowsHTTPOverride(t *testing.T) {
	cfg := config.Default()
	cfg.Nodes = []config.Node{
		{
			Name:      "nat1",
			ExitMode:  config.ExitModeDirect,
			Proxy:     config.ProxyMixed,
			BindHost:  "127.0.0.1",
			LocalPort: 10013,
		},
	}

	out, err := Clash(cfg, ClashOptions{ProxyType: config.ProxyHTTP})
	if err != nil {
		t.Fatalf("export clash: %v", err)
	}
	if !strings.Contains(out, "type: http") {
		t.Fatalf("expected HTTP proxy type, got:\n%s", out)
	}
}

func TestClashRejectsInvalidOverride(t *testing.T) {
	cfg := config.Default()
	_, err := Clash(cfg, ClashOptions{ProxyType: config.ProxyMixed})
	if err == nil {
		t.Fatal("expected invalid override error")
	}
}
