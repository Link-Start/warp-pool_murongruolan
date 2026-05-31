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

func TestDeployTokenStatus(t *testing.T) {
	now := time.Now().UTC()
	cases := []struct {
		name string
		item config.DeployToken
		want string
	}{
		{
			name: "registered",
			item: config.DeployToken{Used: true, Registered: true},
			want: "registered",
		},
		{
			name: "used",
			item: config.DeployToken{Used: true},
			want: "used",
		},
		{
			name: "expired",
			item: config.DeployToken{ExpiresAt: now.Add(-time.Minute).Format(time.RFC3339)},
			want: "expired",
		},
		{
			name: "prepared",
			item: config.DeployToken{ExpiresAt: now.Add(time.Minute).Format(time.RFC3339), Prepared: true},
			want: "prepared",
		},
		{
			name: "unused",
			item: config.DeployToken{ExpiresAt: now.Add(time.Minute).Format(time.RFC3339)},
			want: "unused",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := deployTokenStatus(tc.item, now); got != tc.want {
				t.Fatalf("expected %s, got %s", tc.want, got)
			}
		})
	}
}

func TestShortDeployToken(t *testing.T) {
	got := shortDeployToken("1234567890abcdef")
	if !strings.HasPrefix(got, "123456...") || !strings.HasSuffix(got, "abcdef") {
		t.Fatalf("unexpected short token: %s", got)
	}
}

func TestDeployTokenOutputIncludesTokenAndWireGuardPorts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := config.Default()
	cfg.Language = "zh"
	cfg.Listen.Enabled = true
	if err := config.Save(path, cfg, true); err != nil {
		t.Fatal(err)
	}
	oldConfigPath := configPath
	oldInput := inputReader
	configPath = path
	inputReader = bytes.NewBufferString("")
	t.Cleanup(func() {
		configPath = oldConfigPath
		inputReader = oldInput
	})

	var out bytes.Buffer
	cmd := newDeployTokenCreateCommandWithHooks(
		func(string, int) error { return nil },
		func(string, int) error { return nil },
		func(...string) error { return nil },
		func(string) error { return nil },
	)
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{
		"--name", "节点01",
		"--exit-mode", config.ExitModeDirect,
		"--proxy", config.ProxyMixed,
		"--port", "10013",
		"--wg-listen-port", "51820",
		"--wg-endpoint-port", "30021",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, want := range []string{"Deploy Token", "安装命令", "wg_listen_port=51820", "wg_endpoint_port=30021", "======================"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in output:\n%s", want, text)
		}
	}
}

func TestEnsureRegistrationListenerReportsPortConflict(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := config.Default()
	cfg.Language = "zh"
	var out bytes.Buffer
	prompt := promptIO{in: bytes.NewBufferString("\n"), out: &out, language: "zh"}

	err := ensureRegistrationListener(prompt, path, &cfg, false, listenerHooks{
		CheckPort:      func(string, int) error { return fmt.Errorf("port busy") },
		CheckReachable: func(string, int) error { return fmt.Errorf("not reachable") },
		Systemctl:      func(...string) error { return nil },
		EnsureService:  func(string) error { return nil },
	})
	if err == nil || !strings.Contains(err.Error(), "监听端口已被其他进程占用") {
		t.Fatalf("unexpected error: %v", err)
	}
}
