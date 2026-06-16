package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/murongruolan/warp-pool/internal/config"
)

func TestRenderListenService(t *testing.T) {
	service := renderListenService("/usr/local/bin/warppool", "/etc/warppool/config.json")
	for _, want := range []string{
		"Description=WarpPool Deploy Token Listener",
		"ExecStart='/usr/local/bin/warppool' --config '/etc/warppool/config.json' listen run",
		"Restart=on-failure",
		"WantedBy=multi-user.target",
	} {
		if !strings.Contains(service, want) {
			t.Fatalf("missing %q in service:\n%s", want, service)
		}
	}
}

func TestListenURLFormatsIPv6Literal(t *testing.T) {
	if got := listenURL("2001:db8::1", 8080); got != "http://[2001:db8::1]:8080" {
		t.Fatalf("unexpected ipv6 listen url: %s", got)
	}
}

func TestListenStartUsesConfiguredLanguage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := config.Default()
	cfg.Language = "zh"
	cfg.Listen.Port = 18080
	if err := config.Save(path, cfg, true); err != nil {
		t.Fatal(err)
	}

	oldConfigPath := configPath
	configPath = path
	t.Cleanup(func() { configPath = oldConfigPath })

	var out bytes.Buffer
	cmd := newListenStartCommandWithHooks(
		func(string) error { return nil },
		func(...string) error { return nil },
		"linux",
	)
	cmd.SetOut(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, want := range []string{"注册监听服务已启动", "停止监听命令：warppool listen stop", "服务商控制台放行这些入站端口：18080/tcp"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in:\n%s", want, text)
		}
	}
}
