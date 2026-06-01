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
	if !strings.Contains(text, "proxy check failed: all HTTP checks failed") || !strings.Contains(text, "proxy refused") {
		t.Fatalf("unexpected output:\n%s", text)
	}
}

func TestPingCommandHTTPFallbackAndLatency(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := config.Default()
	cfg.Nodes = []config.Node{
		{
			Name:            "US1",
			ExitMode:        config.ExitModeDirect,
			Proxy:           config.ProxyMixed,
			BindHost:        "127.0.0.1",
			LocalPort:       10016,
			PublicIP:        "204.197.163.238",
			WGServerAddress: "10.200.0.1/30",
			WGClientAddress: "10.200.0.2/30",
			Endpoint:        "204.197.163.238:41704",
		},
	}
	if err := config.Save(path, cfg, true); err != nil {
		t.Fatal(err)
	}

	oldConfigPath := configPath
	configPath = path
	t.Cleanup(func() { configPath = oldConfigPath })

	var calls []string
	var out bytes.Buffer
	cmd := newPingCommandWithChecks(
		func(rawURL string, proxyURL string, timeout time.Duration) (string, error) {
			calls = append(calls, rawURL+"|"+proxyURL)
			if strings.Contains(rawURL, "bad.example") {
				return "", fmt.Errorf("bad url")
			}
			if proxyURL == "" {
				return "主服务器IP", nil
			}
			return "节点IP", nil
		},
		func(target string, count int, timeout time.Duration) (string, error) {
			if target != "204.197.163.238" {
				t.Fatalf("unexpected icmp target: %s", target)
			}
			return "rtt min/avg/max/mdev = 10.000/20.000/30.000/1.000 ms", nil
		},
	)
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"US1", "--url", "https://bad.example/ip,https://api.ipify.org"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	text := out.String()
	for _, want := range []string{
		"node latency avg: 20.000 ms",
		"direct HTTP check url: https://api.ipify.org",
		"direct HTTP check ok: 主服务器IP",
		"proxy check url: https://api.ipify.org",
		"proxy check ok: 节点IP",
		"latency:",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in:\n%s", want, text)
		}
	}
	if len(calls) != 4 {
		t.Fatalf("expected direct+proxy fallback calls, got %d: %#v", len(calls), calls)
	}
}

func TestPingAverageLatencyWindowsOutput(t *testing.T) {
	output := "Minimum = 10ms, Maximum = 30ms, Average = 21ms"
	if got := pingAverageLatency(output); got != "21ms" {
		t.Fatalf("unexpected average latency: %s", got)
	}
}
