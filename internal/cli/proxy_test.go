package cli

import (
	"net"
	"strings"
	"testing"

	"github.com/murongruolan/warp-pool/internal/config"
	"github.com/murongruolan/warp-pool/internal/singbox"
)

func TestRenderProxyService(t *testing.T) {
	service := renderProxyService("/usr/local/bin/warppool", "/etc/warppool/config.json", "/usr/local/lib/warppool/bin/sing-box")
	for _, want := range []string{
		"Description=WarpPool Local Proxy",
		"Environment='WARPOOL_SINGBOX_BIN=/usr/local/lib/warppool/bin/sing-box'",
		"ExecStart='/usr/local/bin/warppool' --config '/etc/warppool/config.json' proxy run",
		"Restart=on-failure",
		"WantedBy=multi-user.target",
	} {
		if !strings.Contains(service, want) {
			t.Fatalf("missing %q in service:\n%s", want, service)
		}
	}
}

func TestBuildProxyConfigRestartIgnoresAllConfiguredNodePorts(t *testing.T) {
	ln := listenOnLocalhost(t)
	defer ln.Close()

	port := ln.Addr().(*net.TCPAddr).Port
	cfg := config.Default()
	cfg.Nodes = []config.Node{
		testProxyNode("美国1", port),
		testProxyNode("圣保罗2", 10017),
	}

	if _, err := buildProxyConfig(cfg, singbox.Options{}, proxyConfigRestart, &cfg.Nodes[1]); err != nil {
		t.Fatalf("restart config should ignore already managed node ports: %v", err)
	}
}

func TestBuildProxyConfigRestartIgnoresDualWarpPort(t *testing.T) {
	ln := listenOnLocalhost(t)
	defer ln.Close()

	node := testProxyNode("双模式1", 10021)
	node.ExitMode = config.ExitModeDual
	node.WarpLocalPort = ln.Addr().(*net.TCPAddr).Port
	node.WGWarpClientAddress = "10.200.0.3/32"
	node.WGWarpClientPrivateKey = "warp-client-private-key"

	cfg := config.Default()
	cfg.Nodes = []config.Node{node}

	if _, err := buildProxyConfig(cfg, singbox.Options{}, proxyConfigRestart, nil); err != nil {
		t.Fatalf("restart config should ignore already managed dual warp port: %v", err)
	}
}

func TestBuildProxyConfigStrictRejectsBusyNodePort(t *testing.T) {
	ln := listenOnLocalhost(t)
	defer ln.Close()

	cfg := config.Default()
	cfg.Nodes = []config.Node{testProxyNode("美国1", ln.Addr().(*net.TCPAddr).Port)}

	if _, err := buildProxyConfig(cfg, singbox.Options{}, proxyConfigStrict, nil); err == nil {
		t.Fatal("strict config should reject busy local proxy port")
	}
}

func listenOnLocalhost(t *testing.T) *net.TCPListener {
	t.Helper()
	ln, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	return ln
}

func testProxyNode(name string, localPort int) config.Node {
	return config.Node{
		Name:               name,
		ExitMode:           config.ExitModeDirect,
		Proxy:              config.ProxyMixed,
		BindHost:           "127.0.0.1",
		LocalPort:          localPort,
		WGClientAddress:    "10.200.0.2/30",
		WGClientPrivateKey: "client-private-key",
		WGServerPublicKey:  "server-public-key",
		Endpoint:           "203.0.113.1:51820",
	}
}
