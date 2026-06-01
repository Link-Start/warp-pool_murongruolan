package deploy

import (
	"strings"
	"testing"

	"github.com/murongruolan/warp-pool/internal/config"
	"github.com/murongruolan/warp-pool/internal/wireguard"
)

func TestPushDryRunAddsNode(t *testing.T) {
	cfg := config.Default()
	next, result, err := Push(cfg, PushOptions{
		DryRun:        true,
		SkipPortCheck: true,
		WGEndpoint:    "203.0.113.1",
		Node: config.Node{
			Name:      "nat1",
			ExitMode:  config.ExitModeDirect,
			Proxy:     config.ProxyMixed,
			BindHost:  "127.0.0.1",
			LocalPort: 10013,
		},
	})
	if err != nil {
		t.Fatalf("push dry-run: %v", err)
	}
	if len(next.Nodes) != 0 {
		t.Fatalf("expected dry-run to skip saving nodes, got %d", len(next.Nodes))
	}
	if !containsLog(result.Logs, "dry-run: skip ssh connect") {
		t.Fatalf("expected dry-run log, got %#v", result.Logs)
	}
	if !containsLog(result.Logs, "dry-run: enable IPv4 forwarding and direct MASQUERADE") {
		t.Fatalf("expected direct forwarding dry-run log, got %#v", result.Logs)
	}
	if !containsLog(result.Logs, "dry-run: run WireGuard preflight") {
		t.Fatalf("expected preflight dry-run log, got %#v", result.Logs)
	}
}

func TestPushDryRunWarpSkipsDirectForwarding(t *testing.T) {
	cfg := config.Default()
	_, result, err := Push(cfg, PushOptions{
		DryRun:        true,
		SkipPortCheck: true,
		WGEndpoint:    "203.0.113.1",
		Node: config.Node{
			Name:      "nat1",
			ExitMode:  config.ExitModeWarp,
			Proxy:     config.ProxyMixed,
			BindHost:  "127.0.0.1",
			LocalPort: 10013,
		},
	})
	if err != nil {
		t.Fatalf("push dry-run: %v", err)
	}
	if containsLog(result.Logs, "dry-run: enable IPv4 forwarding and direct MASQUERADE") {
		t.Fatalf("expected warp mode to skip direct forwarding log, got %#v", result.Logs)
	}
	if !containsLog(result.Logs, "dry-run: enable WARP forwarding") {
		t.Fatalf("expected warp forwarding dry-run log, got %#v", result.Logs)
	}
}

func TestPushDryRunRejectsDuplicatePort(t *testing.T) {
	cfg := config.Default()
	var err error
	cfg, err = config.AddNode(cfg, config.Node{
		Name:      "nat1",
		ExitMode:  config.ExitModeDirect,
		Proxy:     config.ProxyMixed,
		BindHost:  "127.0.0.1",
		LocalPort: 10013,
	})
	if err != nil {
		t.Fatalf("add node: %v", err)
	}

	_, _, err = Push(cfg, PushOptions{
		DryRun:        true,
		SkipPortCheck: true,
		WGEndpoint:    "203.0.113.1",
		Node: config.Node{
			Name:      "nat2",
			ExitMode:  config.ExitModeWarp,
			Proxy:     config.ProxyMixed,
			BindHost:  "127.0.0.1",
			LocalPort: 10013,
		},
	})
	if err == nil {
		t.Fatal("expected duplicate port error")
	}
}

func TestPushDryRunStoresSSHMetadata(t *testing.T) {
	cfg := config.Default()
	_, result, err := Push(cfg, PushOptions{
		DryRun:        true,
		SkipPortCheck: true,
		WGEndpoint:    "198.51.100.10",
		SSH: SSHOptions{
			Host:                  "203.0.113.9",
			Port:                  25122,
			User:                  "ubuntu",
			KeyPath:               "/root/.ssh/id_ed25519",
			KnownHostsPath:        "/root/.ssh/known_hosts",
			InsecureIgnoreHostKey: true,
		},
		Node: config.Node{
			Name:      "nat1",
			ExitMode:  config.ExitModeDirect,
			Proxy:     config.ProxyMixed,
			BindHost:  "127.0.0.1",
			LocalPort: 10013,
		},
	})
	if err != nil {
		t.Fatalf("push dry-run: %v", err)
	}
	if result.Node.SSHHost != "203.0.113.9" {
		t.Fatalf("unexpected ssh host: %q", result.Node.SSHHost)
	}
	if result.Node.SSHPort != 25122 {
		t.Fatalf("unexpected ssh port: %d", result.Node.SSHPort)
	}
	if result.Node.SSHUser != "ubuntu" {
		t.Fatalf("unexpected ssh user: %q", result.Node.SSHUser)
	}
	if result.Node.SSHKeyPath != "/root/.ssh/id_ed25519" {
		t.Fatalf("unexpected ssh key path: %q", result.Node.SSHKeyPath)
	}
	if result.Node.SSHKnownHostsPath != "/root/.ssh/known_hosts" {
		t.Fatalf("unexpected known_hosts path: %q", result.Node.SSHKnownHostsPath)
	}
	if !result.Node.SSHInsecureHostKey {
		t.Fatal("expected insecure host key flag to be stored")
	}
}

func TestRunWireGuardPreflightCommandUsesRemoteDir(t *testing.T) {
	command := wireGuardPreflightCommand(wireGuardPreflightOptions{
		RemoteDir:     "/tmp/custom dir",
		Device:        "wpnat-1",
		ServerAddress: "10.200.0.1/30",
		ClientAddress: "10.200.0.2/30",
		ListenPort:    51821,
	})

	for _, want := range []string{
		"'/tmp/custom dir/wg_preflight.sh'",
		"'device=wpnat-1'",
		"'server_addr=10.200.0.1/30'",
		"'client_addr=10.200.0.2/30'",
		"'listen_port=51821'",
		"auto_fix=true",
		"wg_preflight.sh not found in deploy assets",
		"exit 1",
	} {
		if !strings.Contains(command, want) {
			t.Fatalf("preflight command missing %q:\n%s", want, command)
		}
	}
}

func TestExplainWireGuardStartupFailureForUnsupportedKernel(t *testing.T) {
	msg := explainWireGuardStartupFailure("Error: Unknown device type.\nUnable to access interface: Protocol not supported")
	if !strings.Contains(msg, "remote kernel does not support WireGuard") {
		t.Fatalf("unexpected explanation: %s", msg)
	}
}

func TestWarpForwardCommandIncludesServerAddress(t *testing.T) {
	command := warpForwardCommand(wireguardPlan(), "/tmp/custom dir", 14000)
	for _, want := range []string{
		"'/tmp/custom dir/warp_forward.sh'",
		"'action=up'",
		"'device=wpnat-warp'",
		"'client_addr=10.200.0.2/30'",
		"'server_addr=10.200.0.1/30'",
		"'transparent_port=14000'",
	} {
		if !strings.Contains(command, want) {
			t.Fatalf("warp forward command missing %q:\n%s", want, command)
		}
	}
}

func TestInstallRemoteNodeUninstallerCommandPathEscapesRemoteDir(t *testing.T) {
	command := installRemoteNodeUninstallerCommand("/tmp/custom dir")
	if !strings.Contains(command, "'/tmp/custom dir/node_uninstall.sh'") {
		t.Fatalf("missing escaped node_uninstall.sh path:\n%s", command)
	}
	if !strings.Contains(command, "/usr/local/bin/warppool-node-uninstall") {
		t.Fatalf("missing install target:\n%s", command)
	}
}

func TestNodeModeSSHCommandIncludesMetadata(t *testing.T) {
	command := nodeModeSSHCommand(ModeSwitchOptions{
		Node: config.Node{
			Name:            "nat1",
			WGDevice:        "wpnat1",
			WGServerAddress: "10.200.0.1/30",
			WGClientAddress: "10.200.0.2/30",
		},
		TargetMode:  config.ExitModeWarp,
		RemoteDir:   "/tmp/custom dir",
		WarpInstall: config.WarpInstallReuse,
		WarpPort:    14000,
	})
	for _, want := range []string{
		"'/tmp/custom dir/node_mode.sh'",
		"'mode=warp'",
		"'node=nat1'",
		"'device=wpnat1'",
		"'client_addr=10.200.0.2/30'",
		"'server_addr=10.200.0.1/30'",
		"'warp_install=reuse'",
		"'lang=en'",
	} {
		if !strings.Contains(command, want) {
			t.Fatalf("node mode command missing %q:\n%s", want, command)
		}
	}
}

func wireguardPlan() wireguard.Plan {
	return wireguard.Plan{
		Device:        "wpnat-warp",
		ServerAddress: "10.200.0.1/30",
		ClientAddress: "10.200.0.2/30",
	}
}

func containsLog(logs []string, want string) bool {
	for _, item := range logs {
		if strings.Contains(item, want) {
			return true
		}
	}
	return false
}
