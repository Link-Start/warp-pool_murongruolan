package cli

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"runtime"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/murongruolan/warp-pool/internal/config"
	"github.com/murongruolan/warp-pool/internal/deploy"
	"github.com/murongruolan/warp-pool/internal/singbox"
	"github.com/murongruolan/warp-pool/internal/token"
	"github.com/murongruolan/warp-pool/internal/wgclient"
	"github.com/spf13/cobra"
)

func newNodeCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "node",
		Short: "Manage local node records",
	}

	cmd.AddCommand(newNodeAddCommand())
	cmd.AddCommand(newNodeListCommand())
	cmd.AddCommand(newNodeShowCommand())
	cmd.AddCommand(newNodeStartCommand())
	cmd.AddCommand(newNodeStopCommand())
	cmd.AddCommand(newNodeStatusCommand())
	cmd.AddCommand(newNodeModeCommand())
	cmd.AddCommand(newNodeModeTokenCommand())
	cmd.AddCommand(newNodeRemoveCommand())
	return cmd
}

func newNodeAddCommand() *cobra.Command {
	var node config.Node
	var skipPortCheck bool

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a node to local config",
		RunE: func(cmd *cobra.Command, args []string) error {
			path := resolvedConfigPath()
			cfg, err := config.Load(path)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			if !skipPortCheck {
				if err := config.CheckPortAvailable(node.BindHost, node.LocalPort); err != nil {
					return err
				}
				if node.ExitMode == config.ExitModeDual {
					if err := config.CheckPortAvailable(node.BindHost, node.WarpLocalPort); err != nil {
						return fmt.Errorf("warp local port: %w", err)
					}
				}
			}

			cfg, err = config.AddNode(cfg, node)
			if err != nil {
				return err
			}

			if err := config.SaveExisting(path, cfg); err != nil {
				return fmt.Errorf("save config: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "added node: %s\n", node.Name)
			return nil
		},
	}

	cmd.Flags().StringVar(&node.Name, "name", "", "node name")
	cmd.Flags().StringVar(&node.ExitMode, "exit-mode", config.ExitModeDirect, "exit mode: direct, warp, or dual")
	cmd.Flags().StringVar(&node.Proxy, "proxy", config.ProxyMixed, "local proxy protocol: socks5, http, or mixed")
	cmd.Flags().StringVar(&node.BindHost, "bind-host", "127.0.0.1", "local proxy bind host")
	cmd.Flags().IntVar(&node.LocalPort, "port", 0, "local proxy port")
	cmd.Flags().IntVar(&node.WarpLocalPort, "warp-port", 0, "local proxy port for WARP in dual mode")
	cmd.Flags().StringVar(&node.PublicIP, "public-ip", "", "node public IP")
	cmd.Flags().StringVar(&node.Country, "country", "", "node country or region")
	cmd.Flags().StringVar(&node.WGDevice, "wg-device", "", "WireGuard device name")
	cmd.Flags().StringVar(&node.WGAddress, "wg-address", "", "WireGuard address")
	cmd.Flags().StringVar(&node.Endpoint, "endpoint", "", "WireGuard endpoint")
	cmd.Flags().BoolVar(&skipPortCheck, "skip-port-check", false, "skip system port availability check")

	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("port")
	return cmd
}

func newNodeListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List local nodes",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(resolvedConfigPath())
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tMODE\tPROXY\tLISTEN\tPUBLIC_IP\tCOUNTRY")
			for _, node := range cfg.Nodes {
				listen := nodeListenSummary(node)
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", node.Name, node.ExitMode, node.Proxy, listen, node.PublicIP, node.Country)
			}
			return w.Flush()
		},
	}
	return cmd
}

func newNodeShowCommand() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "show <name>",
		Short: "Show one local node",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(resolvedConfigPath())
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			node, ok := config.FindNode(cfg, args[0])
			if !ok {
				return fmt.Errorf("node not found: %s", args[0])
			}
			if jsonOut {
				data, err := json.MarshalIndent(redactNode(node), "", "  ")
				if err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), string(data))
				return nil
			}
			return printNodeDetails(cmd.OutOrStdout(), cfgLanguage(cfg), node, true)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "print raw JSON")
	return cmd
}

func newNodeStartCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start <name>",
		Short: "Start local proxy service for a node",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := resolvedConfigPath()
			cfg, node, err := loadConfigAndNode(path, args[0])
			if err != nil {
				return err
			}
			if err := startProxyForNode(path, cfg, node); err != nil {
				return err
			}
			language := cfgLanguage(cfg)
			fmt.Fprintf(cmd.OutOrStdout(), "%s\n", tr(language, "started local proxy service for node: "+args[0], "已启动节点本地代理服务："+args[0]))
			return nil
		},
	}
	return cmd
}

func newNodeStopCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stop <name>",
		Short: "Stop local proxy service",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := loadNode(args[0]); err != nil {
				return err
			}
			if runtime.GOOS != "linux" {
				status, err := singbox.Stop(singbox.ManagerOptions{})
				if status.Message != "" {
					fmt.Fprintln(cmd.OutOrStdout(), status.Message)
				}
				return err
			}
			if err := stopProxyService(); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "stopped local proxy service for node: %s\n", args[0])
			return nil
		},
	}
	return cmd
}

func newNodeStatusCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status <name>",
		Short: "Show local node runtime status",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, node, err := loadConfigAndNode(resolvedConfigPath(), args[0])
			if err != nil {
				return err
			}
			return printNodeDetails(cmd.OutOrStdout(), cfgLanguage(cfg), node, true)
		},
	}
	return cmd
}

func newNodeModeCommand() *cobra.Command {
	var method string
	var printCommand bool
	var repoBaseURL string
	var publicHost string
	var ttl time.Duration
	var warpInstall string
	var removeWarp bool
	var dryRun bool
	var autoStartListener bool
	var ssh deploy.SSHOptions
	var remoteDir string
	var assetsDir string
	var warpPort int

	cmd := &cobra.Command{
		Use:   "mode <name> <direct|warp>",
		Short: "Switch node exit mode",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := resolvedConfigPath()
			cfg, err := config.Load(path)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			language := cfgLanguage(cfg)
			prompt := newPromptIOWithLanguage(cmd.OutOrStdout(), language)

			node, ok := config.FindNode(cfg, args[0])
			if !ok {
				return fmt.Errorf("node not found: %s", args[0])
			}
			targetMode := strings.TrimSpace(args[1])
			if err := config.ValidateExitMode(targetMode); err != nil {
				return err
			}
			if node.ExitMode == config.ExitModeDual || targetMode == config.ExitModeDual {
				return fmt.Errorf("%s", tr(language,
					"node mode switching does not currently change dual-mode wiring; redeploy or refresh the node instead",
					"当前 node mode 切换不会改写 dual 双模式 WireGuard 通道；请重新部署该节点，或后续使用配置刷新流程重新下发",
				))
			}
			if node.WGDevice == "wpshared" {
				return fmt.Errorf("%s", tr(language,
					"this node uses the shared exit-node layout; node mode switching is disabled to avoid affecting other main servers. Deploy it as dual if you need both direct and WARP ports.",
					"当前节点使用共享出口节点布局；为避免影响其他主服务器，已禁用 node mode 切换。如需同时使用 direct 和 WARP，请按 dual 模式重新部署。",
				))
			}
			if warpInstall == "" {
				warpInstall = config.WarpInstallAuto
			}
			if targetMode != config.ExitModeWarp && targetMode != config.ExitModeDual {
				warpInstall = config.WarpInstallAuto
			}
			if err := config.ValidateWarpInstall(warpInstall); err != nil {
				return err
			}
			if targetMode != config.ExitModeWarp && targetMode != config.ExitModeDual && cmd.Flags().Changed("warp-install") {
				return fmt.Errorf("--warp-install only applies when target mode is warp or dual")
			}
			if targetMode != config.ExitModeDirect && removeWarp {
				return fmt.Errorf("--remove-warp only applies when target mode is direct")
			}
			if targetMode == node.ExitMode && !removeWarp && warpInstall != config.WarpInstallReinstall {
				fmt.Fprintln(cmd.OutOrStdout(), tr(language, "node already uses target mode", "节点已经是目标出口模式"))
				return nil
			}

			method = strings.TrimSpace(method)
			if method == "" {
				if printCommand {
					method = "pull"
				} else {
					method, err = prompt.askMenu(tr(language, "Switch method", "切换方式"), method, "pull", []menuOption{
						{Label: tr(language, "pull - print command for the exit node", "pull - 输出子节点执行命令"), Value: "pull"},
						{Label: tr(language, "ssh - connect to the exit node automatically", "ssh - 自动 SSH 连接子节点"), Value: "ssh"},
					})
					if err != nil {
						return err
					}
				}
			}

			switch method {
			case "pull":
				return runNodeModePull(cmd, path, cfg, node, targetMode, nodeModePullOptions{
					Language:          language,
					PublicHost:        publicHost,
					RepoBaseURL:       repoBaseURL,
					TTL:               ttl,
					WarpInstall:       warpInstall,
					RemoveWarp:        removeWarp,
					PrintCommand:      printCommand,
					AutoStartListener: autoStartListener,
					Prompt:            prompt,
				})
			case "ssh":
				return runNodeModeSSH(cmd, path, cfg, node, targetMode, nodeModeSSHOptions{
					Language:    language,
					Prompt:      prompt,
					SSH:         ssh,
					RemoteDir:   remoteDir,
					AssetsDir:   assetsDir,
					WarpInstall: warpInstall,
					RemoveWarp:  removeWarp,
					DryRun:      dryRun,
					WarpPort:    warpPort,
				})
			default:
				return fmt.Errorf("unsupported method %q, expected pull or ssh", method)
			}
		},
	}

	cmd.Flags().StringVar(&method, "method", "", "switch method: pull or ssh")
	cmd.Flags().BoolVar(&printCommand, "print-command", false, "print exit-node command and return")
	cmd.Flags().StringVar(&repoBaseURL, "repo-base-url", "", "installer assets base URL")
	cmd.Flags().StringVar(&publicHost, "public-host", "", "public host/IP for generated exit-node command")
	cmd.Flags().DurationVar(&ttl, "ttl", config.DefaultNodeModeTokenTTL, "mode switch token TTL")
	cmd.Flags().StringVar(&warpInstall, "warp-install", config.WarpInstallAuto, "warp install policy: auto, reuse, or reinstall")
	cmd.Flags().BoolVar(&removeWarp, "remove-warp", false, "remove WARP when switching back to direct")
	cmd.Flags().BoolVar(&autoStartListener, "auto-start-listener", false, "start registration listener automatically without prompting")
	cmd.Flags().StringVar(&ssh.Host, "ssh-host", "", "SSH host")
	cmd.Flags().IntVar(&ssh.Port, "ssh-port", 0, "SSH port")
	cmd.Flags().StringVar(&ssh.User, "ssh-user", "", "SSH user")
	cmd.Flags().StringVar(&ssh.Password, "ssh-password", "", "SSH password, or use WARPOOL_SSH_PASSWORD")
	cmd.Flags().StringVar(&ssh.KeyPath, "ssh-key", "", "SSH private key path")
	cmd.Flags().StringVar(&ssh.KnownHostsPath, "known-hosts", "", "known_hosts file path")
	cmd.Flags().BoolVar(&ssh.InsecureIgnoreHostKey, "insecure-skip-host-key-check", false, "skip SSH host key verification")
	cmd.Flags().StringVar(&remoteDir, "remote-dir", "/tmp/warppool-mode", "remote mode-switch directory")
	cmd.Flags().StringVar(&assetsDir, "assets-dir", "assets", "local assets directory")
	cmd.Flags().IntVar(&warpPort, "warp-forward-port", 14000, "remote transparent TCP redirect port for warp mode")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "validate and show mode switch plan without SSH")
	return cmd
}

func newNodeModeTokenCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "mode-token",
		Hidden: true,
		Short:  "Manage node mode switch tokens",
	}
	cmd.AddCommand(newNodeModeTokenListCommand())
	cmd.AddCommand(newNodeModeTokenPruneCommand())
	return cmd
}

func newNodeModeTokenListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List node mode switch tokens",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(resolvedConfigPath())
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NODE\tMODE\tSTATUS\tEXPIRES_AT\tTOKEN")
			now := time.Now().UTC()
			for _, item := range cfg.ModeTokens {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
					item.NodeName,
					item.TargetMode,
					config.NodeModeTokenStatusOf(item, now),
					item.ExpiresAt,
					shortDeployToken(item.Token),
				)
			}
			return w.Flush()
		},
	}
	return cmd
}

func newNodeModeTokenPruneCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "prune",
		Short: "Remove expired unused node mode switch tokens",
		RunE: func(cmd *cobra.Command, args []string) error {
			path := resolvedConfigPath()
			cfg, err := config.Load(path)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			cfg, removed := config.PruneExpiredNodeModeTokens(cfg, time.Now().UTC())
			if err := config.SaveExisting(path, cfg); err != nil {
				return fmt.Errorf("save config: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "pruned node mode tokens: %d\n", removed)
			return nil
		},
	}
	return cmd
}

type nodeModePullOptions struct {
	Language          string
	PublicHost        string
	RepoBaseURL       string
	TTL               time.Duration
	WarpInstall       string
	RemoveWarp        bool
	PrintCommand      bool
	AutoStartListener bool
	Prompt            promptIO
}

func runNodeModePull(cmd *cobra.Command, path string, cfg config.Config, node config.Node, targetMode string, opts nodeModePullOptions) error {
	if opts.TTL <= 0 {
		opts.TTL = config.DefaultNodeModeTokenTTL
	}
	if err := ensureRegistrationListener(opts.Prompt, path, &cfg, opts.AutoStartListener, listenerHooks{}); err != nil {
		return err
	}
	if err := checkListenReachable(cfg.Listen.Host, cfg.Listen.Port); err != nil {
		return err
	}

	tokenValue, err := token.New()
	if err != nil {
		return err
	}
	expiresAt := time.Now().UTC().Add(opts.TTL)
	cfg, err = config.AddNodeModeToken(cfg, config.NodeModeToken{
		Token:       tokenValue,
		NodeName:    node.Name,
		TargetMode:  targetMode,
		Node:        node,
		ExpiresAt:   expiresAt.Format(time.RFC3339),
		WarpInstall: opts.WarpInstall,
		RemoveWarp:  opts.RemoveWarp,
		AutoStart:   true,
	})
	if err != nil {
		return err
	}
	if err := config.SaveExisting(path, cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	publicHost := opts.PublicHost
	if publicHost == "" {
		publicHost = cfg.Listen.PublicHost
	}
	if publicHost == "" {
		publicHost = cfg.Listen.Host
	}
	serverURL := listenURL(publicHost, cfg.Listen.Port)
	if opts.RepoBaseURL == "" {
		opts.RepoBaseURL = "https://raw.githubusercontent.com/murongruolan/warp-pool/main/assets"
	}
	command := fmt.Sprintf("curl -fsSL %s/node_mode.sh | sudo bash -s -- token=%s lang=%s", opts.RepoBaseURL, tokenValue, opts.Language)
	if opts.PrintCommand {
		fmt.Fprintln(cmd.OutOrStdout(), command)
		return nil
	}

	divider := "======================"
	fmt.Fprintln(cmd.OutOrStdout(), divider)
	fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n", tr(opts.Language, "Node:", "节点："), node.Name)
	fmt.Fprintf(cmd.OutOrStdout(), "%s %s -> %s\n", tr(opts.Language, "Mode:", "出口模式："), node.ExitMode, targetMode)
	fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n", tr(opts.Language, "expires at:", "过期时间："), expiresAt.Format(time.RFC3339))
	fmt.Fprintln(cmd.OutOrStdout(), tr(opts.Language,
		"IMPORTANT: this mode-switch token is time-limited and one-time use. If it expires, rerun `warppool node mode`.",
		"重要提示：此模式切换 token 有有效期且只能使用一次。过期后请重新执行 `warppool node mode`。",
	))
	fmt.Fprintln(cmd.OutOrStdout(), divider)
	fmt.Fprintln(cmd.OutOrStdout(), tr(opts.Language, "Run this command on the exit node:", "请在出口节点执行以下命令："))
	fmt.Fprintln(cmd.OutOrStdout(), command)
	fmt.Fprintln(cmd.OutOrStdout(), tr(opts.Language, "If the exit node has no saved server state, run:", "如果出口节点没有保存主服务器状态，请执行："))
	fmt.Fprintf(cmd.OutOrStdout(), "curl -fsSL %s/node_mode.sh | sudo bash -s -- token=%s server=%s lang=%s\n", opts.RepoBaseURL, tokenValue, serverURL, opts.Language)
	fmt.Fprintln(cmd.OutOrStdout(), divider)
	return nil
}

type nodeModeSSHOptions struct {
	Language    string
	Prompt      promptIO
	SSH         deploy.SSHOptions
	RemoteDir   string
	AssetsDir   string
	WarpInstall string
	RemoveWarp  bool
	DryRun      bool
	WarpPort    int
}

func runNodeModeSSH(cmd *cobra.Command, path string, cfg config.Config, node config.Node, targetMode string, opts nodeModeSSHOptions) error {
	var err error
	applyNodeSSHNonPromptDefaults(&opts.SSH, node)
	opts.SSH.Host, err = opts.Prompt.askRequiredWithDefault(tr(opts.Language, "SSH host/IP", "SSH 主机/IP"), opts.SSH.Host, nodeSSHHostDefault(node))
	if err != nil {
		return err
	}
	opts.SSH.Port, err = opts.Prompt.askInt(tr(opts.Language, "SSH port", "SSH 端口"), opts.SSH.Port, defaultInt(node.SSHPort, 22))
	if err != nil {
		return err
	}
	opts.SSH.User, err = opts.Prompt.askString(tr(opts.Language, "SSH user", "SSH 用户"), opts.SSH.User, defaultString(node.SSHUser, "root"))
	if err != nil {
		return err
	}
	if opts.SSH.KnownHostsPath == "" && !opts.SSH.InsecureIgnoreHostKey {
		if _, statErr := os.Stat(defaultKnownHostsPath()); statErr != nil && os.IsNotExist(statErr) {
			skip, askErr := opts.Prompt.askBool(
				tr(opts.Language, "known_hosts file is missing. Skip SSH host key verification for this mode switch?", "未找到 known_hosts 文件。本次切换是否跳过 SSH HostKey 校验？"),
				false,
				true,
			)
			if askErr != nil {
				return askErr
			}
			opts.SSH.InsecureIgnoreHostKey = skip
		}
	}
	if opts.SSH.Password == "" {
		opts.SSH.Password = os.Getenv("WARPOOL_SSH_PASSWORD")
	}
	if opts.SSH.Password == "" && opts.SSH.KeyPath == "" && !opts.DryRun {
		password, err := promptPassword(tr(opts.Language, "SSH password: ", "SSH 密码: "))
		if err != nil {
			return err
		}
		opts.SSH.Password = password
	}

	result, err := deploy.SwitchModeSSH(deploy.ModeSwitchOptions{
		SSH:            opts.SSH,
		Node:           node,
		TargetMode:     targetMode,
		RemoteDir:      opts.RemoteDir,
		AssetsDir:      resolveAssetsDir(opts.AssetsDir),
		WarpInstall:    opts.WarpInstall,
		RemoveWarp:     opts.RemoveWarp,
		DryRun:         opts.DryRun,
		WarpPort:       opts.WarpPort,
		AutoStartProxy: true,
		Language:       opts.Language,
	})
	if err != nil && deploy.IsSSHHostKeyVerificationError(err) && !opts.SSH.InsecureIgnoreHostKey {
		skip, askErr := opts.Prompt.askBool(
			tr(opts.Language, "SSH host key is not trusted by known_hosts. Skip SSH HostKey verification for this mode switch?", "SSH HostKey 不在 known_hosts 信任记录中。本次切换是否跳过 SSH HostKey 校验？"),
			false,
			true,
		)
		if askErr != nil {
			return askErr
		}
		if skip {
			opts.SSH.InsecureIgnoreHostKey = true
			result, err = deploy.SwitchModeSSH(deploy.ModeSwitchOptions{
				SSH:            opts.SSH,
				Node:           node,
				TargetMode:     targetMode,
				RemoteDir:      opts.RemoteDir,
				AssetsDir:      resolveAssetsDir(opts.AssetsDir),
				WarpInstall:    opts.WarpInstall,
				RemoveWarp:     opts.RemoveWarp,
				DryRun:         opts.DryRun,
				WarpPort:       opts.WarpPort,
				AutoStartProxy: true,
				Language:       opts.Language,
			})
		}
	}
	for _, item := range result.Logs {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		fmt.Fprintln(cmd.OutOrStdout(), item)
	}
	if err != nil {
		return err
	}
	if opts.DryRun {
		fmt.Fprintf(cmd.OutOrStdout(), "validated node mode switch: %s -> %s\n", node.Name, targetMode)
		return nil
	}

	node.ExitMode = targetMode
	node = deploy.ApplySSHMetadata(node, opts.SSH)
	next, err := config.UpdateNode(cfg, node)
	if err != nil {
		return err
	}
	if err := config.SaveExisting(path, next); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	if err := startProxyForNode(path, next, node); err != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "%s\n", tr(opts.Language, "warning: mode switched but failed to restart local proxy service: "+err.Error(), "警告：出口模式已切换，但重启本地代理服务失败："+err.Error()))
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "%s\n", tr(opts.Language, "local proxy service restarted", "本地代理服务已重启"))
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%s %s -> %s\n", tr(opts.Language, "switched node mode:", "节点出口模式已切换："), node.Name, targetMode)
	return nil
}

func applyNodeSSHNonPromptDefaults(ssh *deploy.SSHOptions, node config.Node) {
	if strings.TrimSpace(ssh.KeyPath) == "" {
		ssh.KeyPath = node.SSHKeyPath
	}
	if strings.TrimSpace(ssh.KnownHostsPath) == "" {
		ssh.KnownHostsPath = node.SSHKnownHostsPath
	}
	if !ssh.InsecureIgnoreHostKey {
		ssh.InsecureIgnoreHostKey = node.SSHInsecureHostKey
	}
}

func nodeSSHHostDefault(node config.Node) string {
	if strings.TrimSpace(node.SSHHost) != "" {
		return strings.TrimSpace(node.SSHHost)
	}
	if strings.TrimSpace(node.PublicIP) != "" {
		return strings.TrimSpace(node.PublicIP)
	}
	return endpointHost(node.Endpoint)
}

func endpointHost(endpoint string) string {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(endpoint); err == nil {
		return strings.Trim(host, "[]")
	}
	if strings.Count(endpoint, ":") == 1 {
		host, _, ok := strings.Cut(endpoint, ":")
		if ok {
			return strings.TrimSpace(host)
		}
	}
	return strings.Trim(endpoint, "[]")
}

func defaultInt(value int, fallback int) int {
	if value == 0 {
		return fallback
	}
	return value
}

func printNodeDetails(out interface{ Write([]byte) (int, error) }, language string, node config.Node, includeRuntime bool) error {
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	printNodeField(w, language, "name", "节点名称", node.Name)
	printNodeField(w, language, "exit_mode", "出口模式", node.ExitMode)
	printNodeField(w, language, "proxy", "本地代理协议", node.Proxy)
	if node.ExitMode == config.ExitModeDual {
		printNodeField(w, language, "direct_listen", "直连本地代理监听", fmt.Sprintf("%s:%d", node.BindHost, node.LocalPort))
		printNodeField(w, language, "warp_listen", "WARP 本地代理监听", fmt.Sprintf("%s:%d", node.BindHost, node.WarpLocalPort))
	} else {
		printNodeField(w, language, "listen", "本地代理监听", fmt.Sprintf("%s:%d", node.BindHost, node.LocalPort))
	}
	printNodeField(w, language, "public_ip", "公网IP", node.PublicIP)
	printNodeField(w, language, "country", "地区", node.Country)
	printNodeField(w, language, "wg_device", "远端 WireGuard 设备", node.WGDevice)
	if nodeUsesSystemWireGuard(node) {
		printNodeField(w, language, "wg_local_device", "本地 WireGuard 设备", defaultIfEmpty(node.WGLocalDevice, wgclient.DefaultLocalDeviceName(node.Name)))
	} else {
		printNodeField(w, language, "wg_local_endpoint", "本地 WireGuard endpoint", singbox.DefaultEndpointName(node))
	}
	printNodeField(w, language, "wg_server_address", "WireGuard 服务端地址", node.WGServerAddress)
	printNodeField(w, language, "wg_client_address", "WireGuard 客户端地址", node.WGClientAddress)
	printNodeField(w, language, "wg_server_ipv6_address", "WireGuard IPv6 服务端地址", node.WGServerIPv6Address)
	printNodeField(w, language, "wg_client_ipv6_address", "WireGuard IPv6 客户端地址", node.WGClientIPv6Address)
	if node.ExitMode == config.ExitModeDual {
		printNodeField(w, language, "wg_warp_client_address", "WireGuard WARP 客户端地址", node.WGWarpClientAddress)
		printNodeField(w, language, "wg_warp_client_ipv6_address", "WireGuard WARP IPv6 客户端地址", node.WGWarpClientIPv6Address)
	}
	printNodeField(w, language, "wg_listen_port", "WireGuard 监听端口", intString(node.WGListenPort))
	printNodeField(w, language, "endpoint", "WireGuard 公网端点", node.Endpoint)
	printNodeField(w, language, "created_at", "创建时间", node.CreatedAt)
	printNodeField(w, language, "last_updated", "更新时间", node.LastUpdated)

	if includeRuntime {
		if nodeUsesSystemWireGuard(node) {
			printNodeField(w, language, "wireguard_runtime", "WireGuard 运行方式", "system wg-quick")
			if status, err := wgclient.GetStatus(node, wgclient.Options{}); err == nil {
				printNodeField(w, language, "wireguard_active", "WireGuard 已启动", fmt.Sprintf("%t", status.Active))
				if strings.TrimSpace(status.Output) != "" {
					printNodeField(w, language, "wireguard_status", "WireGuard 状态", compactMultiline(status.Output))
				}
			} else {
				printNodeField(w, language, "wireguard_error", "WireGuard 错误", err.Error())
			}
		} else {
			printNodeField(w, language, "wireguard_runtime", "WireGuard 运行方式", tr(language, "sing-box embedded endpoint", "sing-box 内置 endpoint"))
			printNodeField(w, language, "wireguard_status", "WireGuard 状态", tr(language, "managed by sing-box; no system wg interface is expected", "由 sing-box 管理；不会创建系统 wg 设备"))
		}
		if status, err := singbox.Status(singbox.ManagerOptions{}); err == nil {
			printNodeField(w, language, "proxy_running", "本地代理已启动", fmt.Sprintf("%t", status.Running))
			printNodeField(w, language, "proxy_status", "本地代理状态", status.Message)
		} else {
			printNodeField(w, language, "proxy_error", "本地代理错误", err.Error())
		}
	}
	return w.Flush()
}

func nodeUsesSystemWireGuard(node config.Node) bool {
	return strings.TrimSpace(node.WGLocalConfigPath) != ""
}

func nodeListenSummary(node config.Node) string {
	if node.ExitMode == config.ExitModeDual {
		return fmt.Sprintf("direct=%s:%d,warp=%s:%d", node.BindHost, node.LocalPort, node.BindHost, node.WarpLocalPort)
	}
	return fmt.Sprintf("%s:%d", node.BindHost, node.LocalPort)
}

func printNodeField(w *tabwriter.Writer, language string, enKey string, zhKey string, value string) {
	if strings.TrimSpace(value) == "" {
		value = "-"
	}
	key := enKey
	if config.NormalizeLanguage(language) == "zh" {
		key = zhKey
	}
	fmt.Fprintf(w, "%s:\t%s\n", key, value)
}

func defaultIfEmpty(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func intString(value int) string {
	if value == 0 {
		return ""
	}
	return fmt.Sprintf("%d", value)
}

func compactMultiline(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}

func safeFilePart(value string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(value) {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			if b.Len() > 0 {
				b.WriteRune('-')
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "node"
	}
	return out
}

func newNodeRemoveCommand() *cobra.Command {
	cleanWG := true
	var yes bool
	var skipProxyRefresh bool

	cmd := &cobra.Command{
		Use:     "remove <name>",
		Aliases: []string{"rm"},
		Short:   "Remove a local node",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := resolvedConfigPath()
			cfg, err := config.Load(path)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			node, ok := config.FindNode(cfg, args[0])
			if !ok {
				return fmt.Errorf("node not found: %s", args[0])
			}

			language := cfgLanguage(cfg)
			if err := printNodeRemoveSummary(cmd.OutOrStdout(), language, node); err != nil {
				return err
			}
			prompt := newPromptIOWithLanguage(cmd.OutOrStdout(), language)
			confirmed, err := prompt.askConfirmDefaultNo(tr(language, "Confirm node removal?", "确认移除此节点？"), yes)
			if err != nil {
				return err
			}
			if !confirmed {
				fmt.Fprintln(cmd.OutOrStdout(), tr(language, "node removal cancelled", "已取消移除节点"))
				return nil
			}

			if cleanWG {
				result, err := removeLocalNodeWG(node)
				for _, log := range result.Logs {
					fmt.Fprintln(cmd.OutOrStdout(), log)
				}
				if err != nil {
					return err
				}
			}

			cfg, removed, err := config.RemoveNode(cfg, args[0])
			if err != nil {
				return err
			}

			if err := config.SaveExisting(path, cfg); err != nil {
				return fmt.Errorf("save config: %w", err)
			}

			if !skipProxyRefresh {
				logs, err := refreshLocalProxyAfterNodeRemove(path, cfg)
				for _, log := range logs {
					fmt.Fprintln(cmd.OutOrStdout(), log)
				}
				if err != nil {
					return err
				}
			}

			fmt.Fprintf(cmd.OutOrStdout(), "removed node: %s\n", removed.Name)
			return nil
		},
	}
	cmd.Flags().BoolVar(&cleanWG, "clean-wg", true, "stop and remove local WireGuard client config for this node")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "confirm node removal without prompting")
	cmd.Flags().BoolVar(&skipProxyRefresh, "skip-proxy-refresh", false, "skip refreshing local proxy after removal")
	_ = cmd.Flags().MarkHidden("skip-proxy-refresh")
	return cmd
}

func newRemoveCommand() *cobra.Command {
	cmd := newNodeRemoveCommand()
	cmd.Use = "remove <name>"
	cmd.Short = "Remove a local node"
	return cmd
}

func removeLocalNodeWG(node config.Node) (uninstallResult, error) {
	opts := uninstallDefaults(uninstallOptions{CleanWG: true, CleanWGSet: true, SkipInteractive: true})
	result := uninstallResult{}
	if strings.TrimSpace(node.WGLocalConfigPath) == "" && strings.TrimSpace(node.WGLocalDevice) == "" {
		result.append("no local WireGuard client config recorded for node: " + node.Name)
		return result, nil
	}
	if err := wgDownBestEffort(node, opts, &result); err != nil {
		return result, err
	}
	device := node.WGLocalDevice
	if device == "" {
		device = wgclient.DefaultLocalDeviceName(node.Name)
	}
	if opts.RuntimeOS == "linux" {
		_ = runBestEffort(opts, &result, "systemctl", "disable", "wg-quick@"+device)
	}
	if node.WGLocalConfigPath != "" {
		if err := removePath(opts, node.WGLocalConfigPath, &result, false); err != nil {
			return result, err
		}
	}
	return result, nil
}

func printNodeRemoveSummary(out interface{ Write([]byte) (int, error) }, language string, node config.Node) error {
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, tr(language, "Selected node to remove:", "将要移除的节点："))
	printNodeField(w, language, "name", "节点名称", node.Name)
	printNodeField(w, language, "exit_mode", "出口模式", node.ExitMode)
	if node.ExitMode == config.ExitModeDual {
		printNodeField(w, language, "direct_listen", "直连本地代理监听", fmt.Sprintf("%s:%d", node.BindHost, node.LocalPort))
		printNodeField(w, language, "warp_listen", "WARP 本地代理监听", fmt.Sprintf("%s:%d", node.BindHost, node.WarpLocalPort))
	} else {
		printNodeField(w, language, "listen", "本地代理监听", fmt.Sprintf("%s:%d", node.BindHost, node.LocalPort))
	}
	printNodeField(w, language, "public_ip", "公网IP", node.PublicIP)
	printNodeField(w, language, "endpoint", "WireGuard 公网端点", node.Endpoint)
	printNodeField(w, language, "wg_device", "远端 WireGuard 设备", node.WGDevice)
	return w.Flush()
}

func refreshLocalProxyAfterNodeRemove(configPath string, cfg config.Config) ([]string, error) {
	var logs []string
	proxyRunning := localProxyRuntimeRunning()

	if len(cfg.Nodes) == 0 {
		if proxyRunning {
			if err := stopLocalProxyRuntime(); err != nil {
				return logs, err
			}
			logs = append(logs, "stopped local proxy service; no nodes remain")
		} else {
			logs = append(logs, "local proxy service is not running")
		}
		if err := removeSingBoxConfigIfExists(); err != nil {
			return logs, err
		}
		logs = append(logs, "removed stale local proxy config")
		return logs, nil
	}

	data, err := buildProxyConfig(cfg, singbox.Options{}, proxyConfigRestart, nil)
	if err != nil {
		return logs, err
	}
	if err := singbox.WriteConfig(singbox.DefaultConfigPath(), data); err != nil {
		return logs, fmt.Errorf("write sing-box config: %w", err)
	}
	logs = append(logs, "updated local proxy config")

	if !proxyRunning {
		logs = append(logs, "local proxy service is not running; config updated only")
		return logs, nil
	}
	if err := restartLocalProxyRuntime(configPath); err != nil {
		return logs, err
	}
	logs = append(logs, "restarted local proxy service")
	return logs, nil
}

func localProxyRuntimeRunning() bool {
	status, err := singbox.Status(singbox.ManagerOptions{})
	if err == nil && status.Running {
		return true
	}
	if runtime.GOOS == "linux" {
		return runSystemctl("is-active", "--quiet", "warppool-proxy.service") == nil
	}
	return false
}

func stopLocalProxyRuntime() error {
	if runtime.GOOS == "linux" {
		serviceErr := stopProxyService()
		if status, err := singbox.Stop(singbox.ManagerOptions{}); err == nil {
			if serviceErr == nil || status.Message == "stopped sing-box" {
				return nil
			}
		}
		return serviceErr
	}
	_, err := singbox.Stop(singbox.ManagerOptions{})
	return err
}

func restartLocalProxyRuntime(configPath string) error {
	if runtime.GOOS == "linux" {
		if status, err := singbox.Stop(singbox.ManagerOptions{}); err != nil && status.Running {
			return err
		}
		return startProxyService(configPath, nil)
	}
	status, err := singbox.Stop(singbox.ManagerOptions{})
	if err != nil && status.Running {
		return err
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	data, err := buildAndValidateProxyConfig(cfg, singbox.Options{})
	if err != nil {
		return err
	}
	_, err = singbox.Start(data, singbox.ManagerOptions{})
	return err
}

func removeSingBoxConfigIfExists() error {
	path := singbox.DefaultConfigPath()
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove sing-box config %s: %w", path, err)
	}
	return nil
}

func resolvedConfigPath() string {
	if configPath != "" {
		return configPath
	}
	return config.DefaultPath()
}
