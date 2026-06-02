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
	Progress       ProgressFunc
}

type PushResult struct {
	Node config.Node
	Logs []string
}

type ProgressFunc func(key string, args ...string)

func reportProgress(progress ProgressFunc, key string, args ...string) {
	if progress != nil {
		progress(key, args...)
	}
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
	opts.Node = ApplySSHMetadata(opts.Node, opts.SSH)

	if err := config.ValidateNode(cfg, opts.Node); err != nil {
		return cfg, PushResult{}, err
	}
	if !opts.SkipPortCheck {
		reportProgress(opts.Progress, "checking_local_port")
		if err := config.CheckPortAvailable(opts.Node.BindHost, opts.Node.LocalPort); err != nil {
			return cfg, PushResult{}, err
		}
		if opts.Node.ExitMode == config.ExitModeDual {
			if err := config.CheckPortAvailable(opts.Node.BindHost, opts.Node.WarpLocalPort); err != nil {
				return cfg, PushResult{}, fmt.Errorf("warp local port: %w", err)
			}
		}
	}

	result := PushResult{Node: opts.Node}
	wgOptions := wireguard.Options{
		Node:             opts.Node,
		Endpoint:         opts.WGEndpoint,
		EndpointPort:     opts.WGEndpointPort,
		ListenPort:       opts.WGListenPort,
		EnableForwarding: (opts.Node.ExitMode == config.ExitModeDirect || opts.Node.ExitMode == config.ExitModeDual) && !opts.SkipForwarding,
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
		if opts.Node.ExitMode == config.ExitModeWarp || opts.Node.ExitMode == config.ExitModeDual {
			result.Logs = append(result.Logs, fmt.Sprintf("dry-run: enable WARP forwarding for %s", wgPlan.Device))
		}
		return cfg, result, nil
	}

	reportProgress(opts.Progress, "ssh_connect", fmt.Sprintf("%s:%d", opts.SSH.Host, opts.SSH.Port))
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
	reportProgress(opts.Progress, "ssh_connected")

	if err := uploadAssets(client, opts.AssetsDir, opts.RemoteDir, &result, opts.Progress); err != nil {
		return cfg, result, err
	}

	reportProgress(opts.Progress, "detect_privilege")
	runner, err := detectRemoteRunner(client, opts.SSH.User, opts.SSH.Password)
	if err != nil {
		return cfg, result, err
	}
	if runner.UsesSudo {
		reportProgress(opts.Progress, "using_sudo")
		result.Logs = append(result.Logs, "remote user is not root; using sudo for privileged commands")
	}

	reportProgress(opts.Progress, "install_node")
	command := fmt.Sprintf("bash %s mode=%s", shellPath(filepath.ToSlash(filepath.Join(opts.RemoteDir, "install.sh"))), opts.Node.ExitMode)
	remoteResult, err := runner.Run(command)
	result.Logs = append(result.Logs, remoteResult.Stdout)
	if remoteResult.Stderr != "" {
		result.Logs = append(result.Logs, remoteResult.Stderr)
	}
	if err != nil {
		return cfg, result, err
	}

	if wgOptions.EnableForwarding {
		reportProgress(opts.Progress, "detect_egress")
		egress, err := detectEgressInterface(runner)
		if err != nil {
			return cfg, result, err
		}
		wgOptions.EgressInterface = egress
		result.Logs = append(result.Logs, "detected egress interface: "+egress)
	}

	reportProgress(opts.Progress, "generate_wireguard")
	wgPlan, err := wireguard.BuildPlan(cfg, wgOptions)
	if err != nil {
		return cfg, result, err
	}
	opts.Node = wireguard.ApplyPlan(opts.Node, wgPlan)
	result.Node = opts.Node

	reportProgress(opts.Progress, "configure_wireguard")
	if err := configureRemoteWireGuard(runner, wgPlan, opts.RemoteDir, opts.SkipWGUp, &result); err != nil {
		return cfg, result, err
	}
	if (opts.Node.ExitMode == config.ExitModeWarp || opts.Node.ExitMode == config.ExitModeDual) && !opts.SkipWGUp {
		reportProgress(opts.Progress, "configure_warp")
		if err := configureRemoteWarpForwarding(runner, wgPlan, opts.RemoteDir, opts.WarpPort, &result); err != nil {
			return cfg, result, err
		}
	}

	next, err := config.AddNode(cfg, opts.Node)
	return next, result, err
}

func ApplySSHMetadata(node config.Node, ssh SSHOptions) config.Node {
	if strings.TrimSpace(ssh.Host) != "" {
		node.SSHHost = strings.TrimSpace(ssh.Host)
	}
	if ssh.Port != 0 {
		node.SSHPort = ssh.Port
	}
	if strings.TrimSpace(ssh.User) != "" {
		node.SSHUser = strings.TrimSpace(ssh.User)
	}
	if strings.TrimSpace(ssh.KeyPath) != "" {
		node.SSHKeyPath = strings.TrimSpace(ssh.KeyPath)
	}
	if strings.TrimSpace(ssh.KnownHostsPath) != "" {
		node.SSHKnownHostsPath = strings.TrimSpace(ssh.KnownHostsPath)
	}
	if ssh.InsecureIgnoreHostKey {
		node.SSHInsecureHostKey = true
	}
	return node
}

type RemoteRunner struct {
	Client   *sshclient.Client
	Sudo     string
	Password string
	UsesSudo bool
}

func detectRemoteRunner(client *sshclient.Client, user string, password string) (RemoteRunner, error) {
	runner := RemoteRunner{Client: client, Password: password}
	result, err := client.Run("id -u")
	if err != nil {
		return runner, err
	}
	if strings.TrimSpace(result.Stdout) == "0" {
		return runner, nil
	}

	if _, err := client.Run("command -v sudo >/dev/null 2>&1"); err != nil {
		return runner, fmt.Errorf("remote user %q is not root and sudo is not available; use root or install/configure sudo", user)
	}
	if password != "" {
		if _, err := client.RunWithInput("sudo -S -p '' true", password+"\n"); err == nil {
			runner.Sudo = "sudo -S -p ''"
			runner.UsesSudo = true
			return runner, nil
		}
	}
	if _, err := client.Run("sudo -n true"); err == nil {
		runner.Sudo = "sudo -n"
		runner.UsesSudo = true
		return runner, nil
	}
	return runner, fmt.Errorf("remote user %q is not root and passwordless sudo failed; use root or configure sudo", user)
}

func (r RemoteRunner) Run(command string) (sshclient.Result, error) {
	if !r.UsesSudo {
		return r.Client.Run(command)
	}
	wrapped := r.Sudo + " sh -c " + shellPath(command)
	input := ""
	if strings.Contains(r.Sudo, "-S") && r.Password != "" {
		input = r.Password + "\n"
	}
	return r.Client.RunWithInput(wrapped, input)
}

func configureRemoteWarpForwarding(runner RemoteRunner, plan wireguard.Plan, remoteDir string, warpPort int, result *PushResult) error {
	if warpPort == 0 {
		warpPort = 14000
	}
	clientAddr := plan.ClientAddress
	if plan.DualMode && plan.WarpClientAddress != "" {
		clientAddr = plan.WarpClientAddress
	}
	command := warpForwardCommandForClient(plan, remoteDir, warpPort, clientAddr)
	remoteResult, err := runner.Run(command)
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
	Language       string
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
	if opts.Language == "" {
		opts.Language = "en"
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
	if err := uploadAssets(client, opts.AssetsDir, opts.RemoteDir, &uploadResult, nil); err != nil {
		return result, err
	}
	result.Logs = append(result.Logs, uploadResult.Logs...)

	runner, err := detectRemoteRunner(client, opts.SSH.User, opts.SSH.Password)
	if err != nil {
		return result, err
	}
	if runner.UsesSudo {
		result.Logs = append(result.Logs, "remote user is not root; using sudo for privileged commands")
	}

	command := nodeModeSSHCommand(opts)
	remoteResult, err := runner.Run(command)
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
	language := config.NormalizeLanguage(opts.Language)
	if language != "zh" {
		language = "en"
	}
	return fmt.Sprintf(
		"bash %s %s %s %s %s %s %s %s %s %s",
		shellPath(scriptPath),
		shellPath("mode="+opts.TargetMode),
		shellPath("node="+opts.Node.Name),
		shellPath("device="+opts.Node.WGDevice),
		shellPath("client_addr="+opts.Node.WGClientAddress),
		shellPath("server_addr="+opts.Node.WGServerAddress),
		shellPath("warp_install="+opts.WarpInstall),
		shellPath("remove_warp="+removeWarp),
		shellPath(fmt.Sprintf("transparent_port=%d", opts.WarpPort)),
		shellPath("lang="+language),
	)
}

func warpForwardCommand(plan wireguard.Plan, remoteDir string, warpPort int) string {
	return warpForwardCommandForClient(plan, remoteDir, warpPort, plan.ClientAddress)
}

func warpForwardCommandForClient(plan wireguard.Plan, remoteDir string, warpPort int, clientAddress string) string {
	scriptPath := filepath.ToSlash(filepath.Join(remoteDir, "warp_forward.sh"))
	return fmt.Sprintf(
		"if [ -x %s ]; then bash %s %s %s %s %s %s; else echo '[WarpPool][warp-forward][ERROR] warp_forward.sh not found in deploy assets' >&2; exit 1; fi",
		shellPath(scriptPath),
		shellPath(scriptPath),
		shellPath("action=up"),
		shellPath("device="+plan.Device),
		shellPath("client_addr="+clientAddress),
		shellPath("server_addr="+plan.ServerAddress),
		shellPath(fmt.Sprintf("transparent_port=%d", warpPort)),
	)
}

func uploadAssets(client *sshclient.Client, assetsDir string, remoteDir string, result *PushResult, progress ProgressFunc) error {
	reportProgress(progress, "prepare_remote_dir")
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
		reportProgress(progress, "upload_asset", entry.Name())
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

func configureRemoteWireGuard(runner RemoteRunner, plan wireguard.Plan, remoteDir string, skipUp bool, result *PushResult) error {
	if _, err := runner.Run("mkdir -p /etc/wireguard"); err != nil {
		return err
	}
	if err := installRemoteNodeUninstaller(runner, remoteDir, result); err != nil {
		return err
	}
	if err := runWireGuardPreflight(runner, plan, remoteDir, result); err != nil {
		return err
	}
	if plan.EnableForwarding {
		if _, err := runner.Run("mkdir -p /etc/sysctl.d && printf 'net.ipv4.ip_forward=1\\n' >/etc/sysctl.d/99-warppool.conf && sysctl -w net.ipv4.ip_forward=1"); err != nil {
			return err
		}
		result.Logs = append(result.Logs, "enabled IPv4 forwarding")
	}

	remotePath := "/etc/wireguard/" + plan.Device + ".conf"
	if _, err := runner.Run(fmt.Sprintf("cat > %s <<'EOF'\n%s\nEOF\nchmod 0600 %s", shellPath(remotePath), plan.ServerConfig, shellPath(remotePath))); err != nil {
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
		remoteResult, err := runner.Run(command)
		if remoteResult.Stdout != "" {
			result.Logs = append(result.Logs, remoteResult.Stdout)
		}
		if remoteResult.Stderr != "" {
			result.Logs = append(result.Logs, remoteResult.Stderr)
		}
		if err != nil {
			return fmt.Errorf("%s: %w", explainWireGuardStartupFailure(remoteResult.Stdout+"\n"+remoteResult.Stderr), err)
		}
	}

	result.Logs = append(result.Logs, "WireGuard started: "+plan.Device)
	return nil
}

func explainWireGuardStartupFailure(output string) string {
	lower := strings.ToLower(output)
	switch {
	case strings.Contains(lower, "unknown device type"), strings.Contains(lower, "protocol not supported"):
		return "remote kernel does not support WireGuard; reboot into a kernel with WireGuard support or reinstall a supported kernel/OS image, then redeploy"
	default:
		return "remote WireGuard startup failed"
	}
}

func installRemoteNodeUninstaller(runner RemoteRunner, remoteDir string, result *PushResult) error {
	command := installRemoteNodeUninstallerCommand(remoteDir)
	remoteResult, err := runner.Run(command)
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

func runWireGuardPreflight(runner RemoteRunner, plan wireguard.Plan, remoteDir string, result *PushResult) error {
	command := wireGuardPreflightCommand(wireGuardPreflightOptions{
		RemoteDir:     remoteDir,
		Device:        plan.Device,
		ServerAddress: plan.ServerAddress,
		ClientAddress: plan.ClientAddress,
		ListenPort:    plan.ListenPort,
	})
	remoteResult, err := runner.Run(command)
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

func detectEgressInterface(runner RemoteRunner) (string, error) {
	result, err := runner.Run("ip route show default 0.0.0.0/0 | awk 'NR==1 {for (i=1;i<=NF;i++) if ($i==\"dev\") {print $(i+1); exit}}'")
	if err != nil {
		return "", err
	}
	iface := strings.TrimSpace(result.Stdout)
	if iface == "" {
		return "", fmt.Errorf("cannot detect default egress interface")
	}
	return iface, nil
}
