package deploy

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/murongruolan/warp-pool/internal/config"
	"github.com/murongruolan/warp-pool/internal/sshclient"
	"github.com/murongruolan/warp-pool/internal/wireguard"
)

type SSHOptions struct {
	Host                  string
	Port                  int
	User                  string
	Password              string
	KeyPath               string
	KnownHostsPath        string
	InsecureIgnoreHostKey bool
}

type PushOptions struct {
	SSH            SSHOptions
	Node           config.Node
	DryRun         bool
	RemoteDir      string
	AssetsDir      string
	WGEndpoint     string
	WGEndpointPort int
	WGListenPort   int
	SkipWGUp       bool
	SkipForwarding bool
	SkipPortCheck  bool
	WarpPort       int
}

type PushResult struct {
	Node config.Node
	Logs []string
}

func Push(cfg config.Config, opts PushOptions) (config.Config, PushResult, error) {
	if opts.RemoteDir == "" {
		opts.RemoteDir = "/tmp/warppool-install"
	}
	if opts.AssetsDir == "" {
		opts.AssetsDir = "assets"
	}
	if opts.Node.ExitMode == "" {
		opts.Node.ExitMode = cfg.Defaults.ExitMode
	}
	if opts.Node.Proxy == "" {
		opts.Node.Proxy = cfg.Defaults.Proxy
	}
	if opts.Node.BindHost == "" {
		opts.Node.BindHost = cfg.Defaults.BindHost
	}
	if opts.WGEndpoint == "" {
		opts.WGEndpoint = opts.SSH.Host
	}
	if opts.WGEndpointPort == 0 {
		opts.WGEndpointPort = opts.WGListenPort
	}
	if opts.Node.PublicIP == "" {
		opts.Node.PublicIP = opts.SSH.Host
	}

	if err := config.ValidateNode(cfg, opts.Node); err != nil {
		return cfg, PushResult{}, err
	}
	if !opts.SkipPortCheck {
		if err := config.CheckPortAvailable(opts.Node.BindHost, opts.Node.LocalPort); err != nil {
			return cfg, PushResult{}, err
		}
	}

	result := PushResult{Node: opts.Node}
	wgOptions := wireguard.Options{
		Node:             opts.Node,
		Endpoint:         opts.WGEndpoint,
		EndpointPort:     opts.WGEndpointPort,
		ListenPort:       opts.WGListenPort,
		EnableForwarding: opts.Node.ExitMode == config.ExitModeDirect && !opts.SkipForwarding,
	}

	if opts.DryRun {
		if wgOptions.EnableForwarding {
			wgOptions.EgressInterface = "<egress>"
		}
		wgPlan, err := wireguard.BuildPlan(cfg, wgOptions)
		if err != nil {
			return cfg, result, err
		}
		result.Node = wireguard.ApplyPlan(opts.Node, wgPlan)
		result.Logs = append(result.Logs, "dry-run: skip ssh connect")
		result.Logs = append(result.Logs, fmt.Sprintf("dry-run: upload assets to %s", opts.RemoteDir))
		result.Logs = append(result.Logs, fmt.Sprintf("dry-run: run bash %s/install.sh --dry-run mode=%s", opts.RemoteDir, opts.Node.ExitMode))
		result.Logs = append(result.Logs, fmt.Sprintf("dry-run: run WireGuard preflight for %s", wgPlan.Device))
		result.Logs = append(result.Logs, fmt.Sprintf("dry-run: write WireGuard config /etc/wireguard/%s.conf", wgPlan.Device))
		result.Logs = append(result.Logs, fmt.Sprintf("dry-run: run wg-quick up %s", wgPlan.Device))
		if wgOptions.EnableForwarding {
			result.Logs = append(result.Logs, "dry-run: enable IPv4 forwarding and direct MASQUERADE")
		}
		if opts.Node.ExitMode == config.ExitModeWarp {
			result.Logs = append(result.Logs, fmt.Sprintf("dry-run: enable WARP forwarding for %s", wgPlan.Device))
		}
		return cfg, result, nil
	}

	client, err := sshclient.Dial(sshclient.Config{
		Host: opts.SSH.Host,
		Port: opts.SSH.Port,
		User: opts.SSH.User,
		Auth: sshclient.Auth{
			Password: opts.SSH.Password,
			KeyPath:  opts.SSH.KeyPath,
		},
		Timeout:               20 * time.Second,
		KnownHostsPath:        opts.SSH.KnownHostsPath,
		InsecureIgnoreHostKey: opts.SSH.InsecureIgnoreHostKey,
	})
	if err != nil {
		return cfg, result, err
	}
	defer client.Close()

	if err := uploadAssets(client, opts.AssetsDir, opts.RemoteDir, &result); err != nil {
		return cfg, result, err
	}

	command := fmt.Sprintf("bash %s mode=%s", shellPath(filepath.ToSlash(filepath.Join(opts.RemoteDir, "install.sh"))), opts.Node.ExitMode)
	remoteResult, err := client.Run(command)
	result.Logs = append(result.Logs, remoteResult.Stdout)
	if remoteResult.Stderr != "" {
		result.Logs = append(result.Logs, remoteResult.Stderr)
	}
	if err != nil {
		return cfg, result, err
	}

	if wgOptions.EnableForwarding {
		egress, err := detectEgressInterface(client)
		if err != nil {
			return cfg, result, err
		}
		wgOptions.EgressInterface = egress
		result.Logs = append(result.Logs, "detected egress interface: "+egress)
	}

	wgPlan, err := wireguard.BuildPlan(cfg, wgOptions)
	if err != nil {
		return cfg, result, err
	}
	opts.Node = wireguard.ApplyPlan(opts.Node, wgPlan)
	result.Node = opts.Node

	if err := configureRemoteWireGuard(client, wgPlan, opts.RemoteDir, opts.SkipWGUp, &result); err != nil {
		return cfg, result, err
	}
	if opts.Node.ExitMode == config.ExitModeWarp && !opts.SkipWGUp {
		if err := configureRemoteWarpForwarding(client, wgPlan, opts.RemoteDir, opts.WarpPort, &result); err != nil {
			return cfg, result, err
		}
	}

	next, err := config.AddNode(cfg, opts.Node)
	return next, result, err
}

func configureRemoteWarpForwarding(client *sshclient.Client, plan wireguard.Plan, remoteDir string, warpPort int, result *PushResult) error {
	if warpPort == 0 {
		warpPort = 14000
	}
	command := warpForwardCommand(plan, remoteDir, warpPort)
	remoteResult, err := client.Run(command)
	if remoteResult.Stdout != "" {
		result.Logs = append(result.Logs, remoteResult.Stdout)
	}
	if remoteResult.Stderr != "" {
		result.Logs = append(result.Logs, remoteResult.Stderr)
	}
	if err != nil {
		return fmt.Errorf("warp forwarding failed: %w", err)
	}
	result.Logs = append(result.Logs, "WARP forwarding enabled: "+plan.Device)
	return nil
}

type ModeSwitchOptions struct {
	SSH            SSHOptions
	Node           config.Node
	TargetMode     string
	RemoteDir      string
	AssetsDir      string
	WarpInstall    string
	RemoveWarp     bool
	DryRun         bool
	WarpPort       int
	AutoStartProxy bool
}

type ModeSwitchResult struct {
	Node config.Node
	Logs []string
}

func SwitchModeSSH(opts ModeSwitchOptions) (ModeSwitchResult, error) {
	if opts.RemoteDir == "" {
		opts.RemoteDir = "/tmp/warppool-mode"
	}
	if opts.AssetsDir == "" {
		opts.AssetsDir = "assets"
	}
	if opts.WarpInstall == "" {
		opts.WarpInstall = config.WarpInstallAuto
	}
	if opts.WarpPort == 0 {
		opts.WarpPort = 14000
	}
	if err := config.ValidateExitMode(opts.TargetMode); err != nil {
		return ModeSwitchResult{}, err
	}
	if err := config.ValidateWarpInstall(opts.WarpInstall); err != nil {
		return ModeSwitchResult{}, err
	}
	if opts.Node.WGDevice == "" || opts.Node.WGServerAddress == "" || opts.Node.WGClientAddress == "" {
		return ModeSwitchResult{}, fmt.Errorf("node %s has incomplete WireGuard metadata; deploy it first", opts.Node.Name)
	}

	result := ModeSwitchResult{Node: opts.Node}
	if opts.DryRun {
		result.Logs = append(result.Logs, "dry-run: skip ssh connect")
		result.Logs = append(result.Logs, fmt.Sprintf("dry-run: upload assets to %s", opts.RemoteDir))
		result.Logs = append(result.Logs, "dry-run: run node_mode.sh with local metadata")
		result.Node.ExitMode = opts.TargetMode
		return result, nil
	}

	client, err := sshclient.Dial(sshclient.Config{
		Host: opts.SSH.Host,
		Port: opts.SSH.Port,
		User: opts.SSH.User,
		Auth: sshclient.Auth{
			Password: opts.SSH.Password,
			KeyPath:  opts.SSH.KeyPath,
		},
		Timeout:               20 * time.Second,
		KnownHostsPath:        opts.SSH.KnownHostsPath,
		InsecureIgnoreHostKey: opts.SSH.InsecureIgnoreHostKey,
	})
	if err != nil {
		return result, err
	}
	defer client.Close()

	uploadResult := PushResult{}
	if err := uploadAssets(client, opts.AssetsDir, opts.RemoteDir, &uploadResult); err != nil {
		return result, err
	}
	result.Logs = append(result.Logs, uploadResult.Logs...)

	command := nodeModeSSHCommand(opts)
	remoteResult, err := client.Run(command)
	if remoteResult.Stdout != "" {
		result.Logs = append(result.Logs, remoteResult.Stdout)
	}
	if remoteResult.Stderr != "" {
		result.Logs = append(result.Logs, remoteResult.Stderr)
	}
	if err != nil {
		return result, err
	}
	result.Node.ExitMode = opts.TargetMode
	return result, nil
}

func nodeModeSSHCommand(opts ModeSwitchOptions) string {
	scriptPath := filepath.ToSlash(filepath.Join(opts.RemoteDir, "node_mode.sh"))
	removeWarp := "false"
	if opts.RemoveWarp {
		removeWarp = "true"
	}
	return fmt.Sprintf(
		"bash %s %s %s %s %s %s %s %s %s",
		shellPath(scriptPath),
		shellPath("mode="+opts.TargetMode),
		shellPath("node="+opts.Node.Name),
		shellPath("device="+opts.Node.WGDevice),
		shellPath("client_addr="+opts.Node.WGClientAddress),
		shellPath("server_addr="+opts.Node.WGServerAddress),
		shellPath("warp_install="+opts.WarpInstall),
		shellPath("remove_warp="+removeWarp),
		shellPath(fmt.Sprintf("transparent_port=%d", opts.WarpPort)),
	)
}

func warpForwardCommand(plan wireguard.Plan, remoteDir string, warpPort int) string {
	scriptPath := filepath.ToSlash(filepath.Join(remoteDir, "warp_forward.sh"))
	return fmt.Sprintf(
		"if [ -x %s ]; then bash %s %s %s %s %s %s; else echo '[WarpPool][warp-forward][ERROR] warp_forward.sh not found in deploy assets' >&2; exit 1; fi",
		shellPath(scriptPath),
		shellPath(scriptPath),
		shellPath("action=up"),
		shellPath("device="+plan.Device),
		shellPath("client_addr="+plan.ClientAddress),
		shellPath("server_addr="+plan.ServerAddress),
		shellPath(fmt.Sprintf("transparent_port=%d", warpPort)),
	)
}

func uploadAssets(client *sshclient.Client, assetsDir string, remoteDir string, result *PushResult) error {
	if _, err := client.Run("mkdir -p " + shellPath(remoteDir)); err != nil {
		return err
	}

	entries, err := os.ReadDir(assetsDir)
	if err != nil {
		return fmt.Errorf("read assets dir: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sh") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(assetsDir, entry.Name()))
		if err != nil {
			return err
		}
		remotePath := filepath.ToSlash(filepath.Join(remoteDir, entry.Name()))
		if err := client.Upload(remotePath, data, "0755"); err != nil {
			return err
		}
		result.Logs = append(result.Logs, "uploaded: "+remotePath)
	}
	return nil
}

func shellPath(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func configureRemoteWireGuard(client *sshclient.Client, plan wireguard.Plan, remoteDir string, skipUp bool, result *PushResult) error {
	if _, err := client.Run("mkdir -p /etc/wireguard"); err != nil {
		return err
	}
	if err := installRemoteNodeUninstaller(client, remoteDir, result); err != nil {
		return err
	}
	if err := runWireGuardPreflight(client, plan, remoteDir, result); err != nil {
		return err
	}
	if plan.EnableForwarding {
		if _, err := client.Run("mkdir -p /etc/sysctl.d && printf 'net.ipv4.ip_forward=1\\n' >/etc/sysctl.d/99-warppool.conf && sysctl -w net.ipv4.ip_forward=1"); err != nil {
			return err
		}
		result.Logs = append(result.Logs, "enabled IPv4 forwarding")
	}

	remotePath := "/etc/wireguard/" + plan.Device + ".conf"
	if err := client.Upload(remotePath, []byte(plan.ServerConfig), "0600"); err != nil {
		return err
	}
	result.Logs = append(result.Logs, "uploaded WireGuard config: "+remotePath)

	if skipUp {
		result.Logs = append(result.Logs, "skip WireGuard startup requested")
		return nil
	}

	commands := []string{
		"wg-quick down " + shellPath(plan.Device) + " >/dev/null 2>&1 || true",
		"wg-quick up " + shellPath(plan.Device),
		"systemctl enable " + shellPath("wg-quick@"+plan.Device) + " >/dev/null 2>&1 || true",
	}
	for _, command := range commands {
		remoteResult, err := client.Run(command)
		if remoteResult.Stdout != "" {
			result.Logs = append(result.Logs, remoteResult.Stdout)
		}
		if remoteResult.Stderr != "" {
			result.Logs = append(result.Logs, remoteResult.Stderr)
		}
		if err != nil {
			return err
		}
	}

	result.Logs = append(result.Logs, "WireGuard started: "+plan.Device)
	return nil
}

func installRemoteNodeUninstaller(client *sshclient.Client, remoteDir string, result *PushResult) error {
	command := installRemoteNodeUninstallerCommand(remoteDir)
	remoteResult, err := client.Run(command)
	if remoteResult.Stdout != "" {
		result.Logs = append(result.Logs, remoteResult.Stdout)
	}
	if remoteResult.Stderr != "" {
		result.Logs = append(result.Logs, remoteResult.Stderr)
	}
	if err != nil {
		return fmt.Errorf("install remote node uninstaller: %w", err)
	}
	result.Logs = append(result.Logs, "installed remote node uninstaller: /usr/local/bin/warppool-node-uninstall")
	return nil
}

func installRemoteNodeUninstallerCommand(remoteDir string) string {
	scriptPath := filepath.ToSlash(filepath.Join(remoteDir, "node_uninstall.sh"))
	return fmt.Sprintf(
		"if [ -x %s ]; then cp %s /usr/local/bin/warppool-node-uninstall && chmod 0755 /usr/local/bin/warppool-node-uninstall; else echo '[WarpPool][node-uninstall][WARN] node_uninstall.sh not found in deploy assets' >&2; fi",
		shellPath(scriptPath),
		shellPath(scriptPath),
	)
}

func runWireGuardPreflight(client *sshclient.Client, plan wireguard.Plan, remoteDir string, result *PushResult) error {
	command := wireGuardPreflightCommand(wireGuardPreflightOptions{
		RemoteDir:     remoteDir,
		Device:        plan.Device,
		ServerAddress: plan.ServerAddress,
		ClientAddress: plan.ClientAddress,
		ListenPort:    plan.ListenPort,
	})
	remoteResult, err := client.Run(command)
	if remoteResult.Stdout != "" {
		result.Logs = append(result.Logs, remoteResult.Stdout)
	}
	if remoteResult.Stderr != "" {
		result.Logs = append(result.Logs, remoteResult.Stderr)
	}
	if err != nil {
		return fmt.Errorf("wireguard preflight failed: %w", err)
	}
	return nil
}

type wireGuardPreflightOptions struct {
	RemoteDir     string
	Device        string
	ServerAddress string
	ClientAddress string
	ListenPort    int
}

func wireGuardPreflightCommand(opts wireGuardPreflightOptions) string {
	scriptPath := filepath.ToSlash(filepath.Join(opts.RemoteDir, "wg_preflight.sh"))
	return fmt.Sprintf(
		"if [ -x %s ]; then bash %s %s %s %s %s auto_fix=true; else echo '[WarpPool][wg-preflight][ERROR] wg_preflight.sh not found in deploy assets' >&2; exit 1; fi",
		shellPath(scriptPath),
		shellPath(scriptPath),
		shellPath("device="+opts.Device),
		shellPath("server_addr="+opts.ServerAddress),
		shellPath("client_addr="+opts.ClientAddress),
		shellPath(fmt.Sprintf("listen_port=%d", opts.ListenPort)),
	)
}

func detectEgressInterface(client *sshclient.Client) (string, error) {
	result, err := client.Run("ip route show default 0.0.0.0/0 | awk 'NR==1 {for (i=1;i<=NF;i++) if ($i==\"dev\") {print $(i+1); exit}}'")
	if err != nil {
		return "", err
	}
	iface := strings.TrimSpace(result.Stdout)
	if iface == "" {
		return "", fmt.Errorf("cannot detect default egress interface")
	}
	return iface, nil
}
