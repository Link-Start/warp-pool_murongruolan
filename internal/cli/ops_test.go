package cli

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/murongruolan/warp-pool/internal/config"
)

func TestPingCommandUsesProxyCheckForEmbeddedWireGuard(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := config.Default()
	cfg.Nodes = []config.Node{
		{
			Name:               "US1",
			ExitMode:           config.ExitModeDirect,
			Proxy:              config.ProxyMixed,
			BindHost:           "127.0.0.1",
			LocalPort:          10016,
			WGServerAddress:    "10.200.0.1/30",
			WGClientAddress:    "10.200.0.2/30",
			WGClientPrivateKey: "client-private-key",
			WGServerPublicKey:  "server-public-key",
			Endpoint:           "204.197.163.238:41704",
		},
	}
	if err := config.Save(path, cfg, true); err != nil {
		t.Fatal(err)
	}

	oldConfigPath := configPath
	configPath = path
	t.Cleanup(func() { configPath = oldConfigPath })

	var gotURL string
	var gotProxy string
	var out bytes.Buffer
	cmd := newPingCommandWithHTTPCheck(func(rawURL string, proxyURL string, timeout time.Duration) (string, error) {
		gotURL = rawURL
		gotProxy = proxyURL
		return "204.197.163.238", nil
	})
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"US1"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	text := out.String()
	for _, want := range []string{"mode: sing-box embedded wireguard proxy check", "proxy check ok: 204.197.163.238"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in:\n%s", want, text)
		}
	}
	if gotURL != "https://api.ipify.org" {
		t.Fatalf("unexpected check url: %s", gotURL)
	}
	if gotProxy != "socks5://127.0.0.1:10016" {
		t.Fatalf("unexpected proxy url: %s", gotProxy)
	}
	if strings.Contains(text, "10.200.0.1") || strings.Contains(text, "ping failed") {
		t.Fatalf("embedded WireGuard ping should not use system ICMP:\n%s", text)
	}
}

func TestPingCommandReportsProxyCheckFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := config.Default()
	cfg.Nodes = []config.Node{
		{
			Name:               "US1",
			ExitMode:           config.ExitModeDirect,
			Proxy:              config.ProxyMixed,
			BindHost:           "127.0.0.1",
			LocalPort:          10016,
			WGServerAddress:    "10.200.0.1/30",
			WGClientAddress:    "10.200.0.2/30",
			WGClientPrivateKey: "client-private-key",
			WGServerPublicKey:  "server-public-key",
			Endpoint:           "204.197.163.238:41704",
		},
	}
	if err := config.Save(path, cfg, true); err != nil {
		t.Fatal(err)
	}

	oldConfigPath := configPath
	configPath = path
	t.Cleanup(func() { configPath = oldConfigPath })

	var out bytes.Buffer
	cmd := newPingCommandWithHTTPCheck(func(string, string, time.Duration) (string, error) {
		return "", fmt.Errorf("proxy refused")
	})
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"US1"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	text := out.String()
	if !strings.Contains(text, "proxy check failed: proxy refused") {
		t.Fatalf("unexpected output:\n%s", text)
	}
}
