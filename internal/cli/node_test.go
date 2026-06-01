package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/murongruolan/warp-pool/internal/config"
)

func TestPrintNodeDetailsChinese(t *testing.T) {
	var out bytes.Buffer
	node := config.Node{
		Name:            "美国NAT01",
		ExitMode:        config.ExitModeDirect,
		Proxy:           config.ProxyMixed,
		BindHost:        "127.0.0.1",
		LocalPort:       10013,
		WGServerAddress: "10.200.0.1/30",
		WGClientAddress: "10.200.0.2/30",
		Endpoint:        "203.0.113.10:30021",
	}

	if err := printNodeDetails(&out, "zh", node, false); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, want := range []string{"节点名称:", "出口模式:", "本地代理监听:", "WireGuard 公网端点:"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in:\n%s", want, text)
		}
	}
}

func TestPrintNodeDetailsEmbeddedWireGuardDoesNotCallSystemWG(t *testing.T) {
	var out bytes.Buffer
	node := config.Node{
		Name:               "US1",
		ExitMode:           config.ExitModeDirect,
		Proxy:              config.ProxyMixed,
		BindHost:           "127.0.0.1",
		LocalPort:          10016,
		WGDevice:           "wpus1",
		WGServerAddress:    "10.200.0.1/30",
		WGClientAddress:    "10.200.0.2/30",
		WGClientPrivateKey: "client-private-key",
		WGServerPublicKey:  "server-public-key",
		Endpoint:           "204.197.163.238:41704",
	}

	if err := printNodeDetails(&out, "zh", node, true); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, want := range []string{"本地 WireGuard endpoint:", "sing-box 内置 endpoint", "由 sing-box 管理"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in:\n%s", want, text)
		}
	}
	if strings.Contains(text, "WireGuard 错误") || strings.Contains(text, "wg show") {
		t.Fatalf("embedded WireGuard status should not call system wg:\n%s", text)
	}
}

func TestSafeFilePart(t *testing.T) {
	if got := safeFilePart("美国NAT01"); got != "nat01" {
		t.Fatalf("unexpected safe file part: %s", got)
	}
	if got := safeFilePart("美国节点"); got != "node" {
		t.Fatalf("unexpected fallback safe file part: %s", got)
	}
}

func TestNodeSSHHostDefault(t *testing.T) {
	node := config.Node{
		SSHHost:  "ssh.example.com",
		PublicIP: "203.0.113.10",
		Endpoint: "198.51.100.10:51820",
	}
	if got := nodeSSHHostDefault(node); got != "ssh.example.com" {
		t.Fatalf("unexpected ssh host default: %s", got)
	}

	node.SSHHost = ""
	if got := nodeSSHHostDefault(node); got != "203.0.113.10" {
		t.Fatalf("unexpected public ip fallback: %s", got)
	}

	node.PublicIP = ""
	if got := nodeSSHHostDefault(node); got != "198.51.100.10" {
		t.Fatalf("unexpected endpoint fallback: %s", got)
	}
}

func TestEndpointHostIPv6(t *testing.T) {
	if got := endpointHost("[2001:db8::1]:51820"); got != "2001:db8::1" {
		t.Fatalf("unexpected ipv6 endpoint host: %s", got)
	}
}
