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

func TestSafeFilePart(t *testing.T) {
	if got := safeFilePart("美国NAT01"); got != "nat01" {
		t.Fatalf("unexpected safe file part: %s", got)
	}
	if got := safeFilePart("美国节点"); got != "node" {
		t.Fatalf("unexpected fallback safe file part: %s", got)
	}
}
