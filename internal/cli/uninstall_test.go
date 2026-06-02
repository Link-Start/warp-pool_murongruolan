package cli

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/murongruolan/warp-pool/internal/config"
)

type uninstallFakeRunner struct {
	commands []string
}

func (r *uninstallFakeRunner) Run(name string, args ...string) ([]byte, error) {
	r.commands = append(r.commands, name+" "+strings.Join(args, " "))
	return nil, nil
}

func TestUninstallAllCanCleanWGAndProxy(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	wgPath := filepath.Join(dir, "wpcnat1.conf")
	stateDir := filepath.Join(dir, "state")
	installDir := filepath.Join(dir, "install")
	binaryPath := filepath.Join(dir, "warppool")
	aliasPath := filepath.Join(dir, "wpl")
	if err := os.WriteFile(wgPath, []byte("wg"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{stateDir, installDir} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(binaryPath, []byte("bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(aliasPath, []byte("alias"), 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	cfg.Nodes = append(cfg.Nodes, config.Node{
		Name:              "nat1",
		ExitMode:          config.ExitModeDirect,
		Proxy:             config.ProxyMixed,
		BindHost:          "127.0.0.1",
		LocalPort:         19001,
		WGLocalDevice:     "wpcnat1",
		WGLocalConfigPath: wgPath,
	})
	if err := config.Save(cfgPath, cfg, true); err != nil {
		t.Fatal(err)
	}

	runner := &uninstallFakeRunner{}
	_, err := uninstallAll(uninstallOptions{
		ConfigPath:    cfgPath,
		StateDir:      stateDir,
		InstallDir:    installDir,
		BinaryPath:    binaryPath,
		AliasPath:     aliasPath,
		RuntimeOS:     "test",
		Runner:        runner,
		CleanWG:       true,
		CleanProxy:    true,
		CleanWGSet:    true,
		CleanProxySet: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(wgPath); !os.IsNotExist(err) {
		t.Fatalf("expected wg config removed, stat err=%v", err)
	}
	if _, err := os.Stat(stateDir); !os.IsNotExist(err) {
		t.Fatalf("expected state dir removed, stat err=%v", err)
	}
	if _, err := os.Stat(binaryPath); !os.IsNotExist(err) {
		t.Fatalf("expected binary removed, stat err=%v", err)
	}
	if _, err := os.Stat(aliasPath); err != nil {
		t.Fatalf("expected non-symlink alias preserved, stat err=%v", err)
	}
}

func TestUninstallAllDryRunDoesNotRemoveFiles(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	stateDir := filepath.Join(dir, "state")
	installDir := filepath.Join(dir, "install")
	binaryPath := filepath.Join(dir, "warppool")
	for _, path := range []string{stateDir, installDir} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(binaryPath, []byte("bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := config.Save(cfgPath, config.Default(), true); err != nil {
		t.Fatal(err)
	}

	runner := &uninstallFakeRunner{}
	result, err := uninstallAll(uninstallOptions{
		ConfigPath: cfgPath,
		StateDir:   stateDir,
		InstallDir: installDir,
		BinaryPath: binaryPath,
		RuntimeOS:  "test",
		DryRun:     true,
		Runner:     runner,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(cfgPath); err != nil {
		t.Fatalf("dry-run removed config: %v", err)
	}
	if !hasUninstallLog(result.Logs, "dry-run: remove "+cfgPath) {
		t.Fatalf("expected dry-run remove config log: %#v", result.Logs)
	}
}

func TestUninstallAllPromptsForWGAndProxy(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	stateDir := filepath.Join(dir, "state")
	installDir := filepath.Join(dir, "install")
	binaryPath := filepath.Join(dir, "warppool")
	for _, path := range []string{stateDir, installDir} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(binaryPath, []byte("bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := config.Save(cfgPath, config.Default(), true); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	_, err := uninstallAll(uninstallOptions{
		ConfigPath: cfgPath,
		StateDir:   stateDir,
		InstallDir: installDir,
		BinaryPath: binaryPath,
		RuntimeOS:  "test",
		Runner:     &uninstallFakeRunner{},
		Prompt:     promptIO{in: bufio.NewReader(bytes.NewBufferString("n\ny\n")), out: &out},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stateDir); !os.IsNotExist(err) {
		t.Fatalf("expected proxy state removed after yes answer, stat err=%v", err)
	}
	if !strings.Contains(out.String(), "WireGuard") || !strings.Contains(out.String(), "proxy") {
		t.Fatalf("expected prompts, got: %s", out.String())
	}
}

func hasUninstallLog(logs []string, want string) bool {
	for _, log := range logs {
		if strings.Contains(log, want) {
			return true
		}
	}
	return false
}

func TestAliasTargetAllowedOnlyAllowsWarpPoolTargets(t *testing.T) {
	aliasPath := "/usr/local/bin/wpl"
	if !aliasTargetAllowed(aliasPath, "/usr/local/bin/warppool", []string{"/usr/local/bin/warppool"}) {
		t.Fatal("expected primary warppool target allowed")
	}
	if !aliasTargetAllowed(aliasPath, "warppool", []string{"/usr/local/bin/warppool"}) {
		t.Fatal("expected relative warppool target allowed")
	}
	if aliasTargetAllowed(aliasPath, "/usr/bin/other-wpl", []string{"/usr/local/bin/warppool"}) {
		t.Fatal("expected unrelated target rejected")
	}
}
