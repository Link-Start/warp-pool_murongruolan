package cli

import (
	"fmt"
	"net"
	"runtime"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/murongruolan/warp-pool/internal/config"
	"github.com/murongruolan/warp-pool/internal/singbox"
	"github.com/murongruolan/warp-pool/internal/token"
	"github.com/spf13/cobra"
)

func newDeployTokenCommand() *cobra.Command {
	cmd := newDeployTokenCreateCommand()
	cmd.AddCommand(newDeployTokenListCommand())
	cmd.AddCommand(newDeployTokenRemoveCommand())
	cmd.AddCommand(newDeployTokenPruneCommand())
	cmd.AddCommand(newDeployTokenWaitStartCommand())
	return cmd
}

func newDeployTokenCreateCommand() *cobra.Command {
	return newDeployTokenCreateCommandWithHooks(config.CheckPortAvailable, checkListenReachable, runSystemctl, ensureListenServiceInstalled)
}

func newDeployTokenCreateCommandWithHooks(checkPort func(string, int) error, checkReachable func(string, int) error, systemctl func(...string) error, ensureService func(string) error) *cobra.Command {
	var node config.Node
	var ttl time.Duration
	var publicHost string
	var repoBaseURL string
	var endpoint string
	var wgListenPort int
	var wgEndpointPort int
	var autoStartListener bool

	cmd := &cobra.Command{
		Use:   "deploy-token",
		Short: "Generate one-time pull install token",
		RunE: func(cmd *cobra.Command, args []string) error {
			path := resolvedConfigPath()
			cfg, err := config.Load(path)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			language := cfgLanguage(cfg)
			prompt := newPromptIOWithLanguage(cmd.OutOrStdout(), language)
			if err := promptDeployTokenOptions(prompt, cfg, &node); err != nil {
				return err
			}
			if wgListenPort == 0 && cmd.Flags().Lookup("wg-listen-port") != nil && !cmd.Flags().Changed("wg-listen-port") {
				wgListenPort, err = prompt.askInt(tr(language, "WireGuard listen port", "WireGuard 监听端口"), 0, 51820)
				if err != nil {
					return err
				}
			}
			if wgListenPort == 0 {
				wgListenPort = 51820
			}
			if wgEndpointPort == 0 && cmd.Flags().Lookup("wg-endpoint-port") != nil && !cmd.Flags().Changed("wg-endpoint-port") {
				wgEndpointPort, err = prompt.askInt(tr(language, "WireGuard public endpoint port", "WireGuard 公网端点端口"), 0, wgListenPort)
				if err != nil {
					return err
				}
			}
			if wgEndpointPort == 0 {
				wgEndpointPort = wgListenPort
			}
			if err := config.ValidatePort(wgListenPort); err != nil {
				return err
			}
			if err := config.ValidatePort(wgEndpointPort); err != nil {
				return err
			}
			if err := ensureRegistrationListener(prompt, path, &cfg, autoStartListener, listenerHooks{
				CheckPort:      checkPort,
				CheckReachable: checkReachable,
				Systemctl:      systemctl,
				EnsureService:  ensureService,
			}); err != nil {
				return err
			}
			if err := checkReachable(cfg.Listen.Host, cfg.Listen.Port); err != nil {
				return err
			}

			if node.ExitMode == "" {
				node.ExitMode = cfg.Defaults.ExitMode
			}
			if node.Proxy == "" {
				node.Proxy = cfg.Defaults.Proxy
			}
			if node.BindHost == "" {
				node.BindHost = cfg.Defaults.BindHost
			}

			tokenValue, err := token.New()
			if err != nil {
				return err
			}
			expiresAt := time.Now().UTC().Add(ttl)
			cfg, err = config.AddDeployToken(cfg, config.DeployToken{
				Token:     tokenValue,
				Node:      node,
				ExpiresAt: expiresAt.Format(time.RFC3339),
				AutoStart: true,
			})
			if err != nil {
				return err
			}
			if err := config.SaveExisting(path, cfg); err != nil {
				return fmt.Errorf("save config: %w", err)
			}

			if publicHost == "" {
				publicHost = cfg.Listen.PublicHost
			}
			if publicHost == "" {
				publicHost = cfg.Listen.Host
			}
			serverURL := listenURL(publicHost, cfg.Listen.Port)
			if repoBaseURL == "" {
				repoBaseURL = "https://raw.githubusercontent.com/murongruolan/warp-pool/main/assets"
			}
			divider := "======================"
			fmt.Fprintln(cmd.OutOrStdout(), divider)
			fmt.Fprintf(cmd.OutOrStdout(), "%s\n", tr(language, "Deploy Token:", "Deploy Token："))
			fmt.Fprintln(cmd.OutOrStdout(), tokenValue)
			fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n", tr(language, "expires at:", "过期时间："), expiresAt.Format(time.RFC3339))
			fmt.Fprintln(cmd.OutOrStdout(), divider)
			fmt.Fprintln(cmd.OutOrStdout(), tr(language, "Install command:", "安装命令："))
			installArgs := fmt.Sprintf("mode=%s token=%s server=%s", node.ExitMode, tokenValue, serverURL)
			if endpoint != "" {
				installArgs += fmt.Sprintf(" endpoint=%s", endpoint)
			}
			installArgs += fmt.Sprintf(" wg_listen_port=%d", wgListenPort)
			installArgs += fmt.Sprintf(" wg_endpoint_port=%d", wgEndpointPort)
			fmt.Fprintf(cmd.OutOrStdout(), "wget -qO- %s/install.sh | sudo bash -s -- %s\n", repoBaseURL, installArgs)
			fmt.Fprintln(cmd.OutOrStdout(), tr(language, "or:", "或："))
			fmt.Fprintf(cmd.OutOrStdout(), "curl -fsSL %s/install.sh | sudo bash -s -- %s\n", repoBaseURL, installArgs)
			fmt.Fprintln(cmd.OutOrStdout(), divider)
			fmt.Fprintln(cmd.OutOrStdout(), cloudSecurityGroupReminder(language, fmt.Sprintf("%d/tcp", cfg.Listen.Port), fmt.Sprintf("%d/udp", wgEndpointPort)))
			return nil
		},
	}

	cmd.Flags().StringVar(&node.Name, "name", "", "node name")
	cmd.Flags().StringVar(&node.ExitMode, "exit-mode", "", "exit mode: direct or warp")
	cmd.Flags().StringVar(&node.Proxy, "proxy", "", "local proxy protocol: socks5, http, or mixed")
	cmd.Flags().StringVar(&node.BindHost, "bind-host", "127.0.0.1", "local proxy bind host")
	cmd.Flags().IntVar(&node.LocalPort, "port", 0, "local proxy port")
	cmd.Flags().StringVar(&node.Country, "country", "", "node country or region")
	cmd.Flags().StringVar(&node.PublicIP, "public-ip", "", "node public IP")
	cmd.Flags().DurationVar(&ttl, "ttl", config.DefaultDeployTokenTTL, "token TTL")
	cmd.Flags().StringVar(&publicHost, "public-host", "", "public host/IP for generated install command")
	cmd.Flags().StringVar(&repoBaseURL, "repo-base-url", "", "installer assets base URL")
	cmd.Flags().StringVar(&endpoint, "wg-endpoint", "", "WireGuard public endpoint host/IP for the main server to connect, node installer auto-detects when empty")
	cmd.Flags().IntVar(&wgListenPort, "wg-listen-port", 0, "WireGuard listen port on the node")
	cmd.Flags().IntVar(&wgEndpointPort, "wg-endpoint-port", 0, "WireGuard public endpoint port, useful for NAT port forwarding")
	cmd.Flags().BoolVar(&autoStartListener, "auto-start-listener", false, "start registration listener automatically without prompting")

	return cmd
}

type listenerHooks struct {
	CheckPort      func(string, int) error
	CheckReachable func(string, int) error
	Systemctl      func(...string) error
	EnsureService  func(string) error
}

func (h listenerHooks) withDefaults() listenerHooks {
	if h.CheckPort == nil {
		h.CheckPort = config.CheckPortAvailable
	}
	if h.CheckReachable == nil {
		h.CheckReachable = checkListenReachable
	}
	if h.Systemctl == nil {
		h.Systemctl = runSystemctl
	}
	if h.EnsureService == nil {
		h.EnsureService = ensureListenServiceInstalled
	}
	return h
}

func ensureRegistrationListener(prompt promptIO, path string, cfg *config.Config, autoStart bool, hooks listenerHooks) error {
	hooks = hooks.withDefaults()
	language := prompt.language
	if cfg.Listen.Enabled {
		if err := hooks.CheckReachable(cfg.Listen.Host, cfg.Listen.Port); err == nil {
			return nil
		}
		*cfg = config.SetListenEnabled(*cfg, false)
	}
	if !autoStart {
		start, err := prompt.askBool(
			tr(language, "Registration listener is not running. Start it now?", "注册监听未启动。是否现在自动启动？"),
			false,
			true,
		)
		if err != nil {
			return err
		}
		if !start {
			return fmt.Errorf("%s", tr(language, "registration listener is required for Deploy Token. Run: warppool listen start", "Deploy Token 安装方式必须启动注册监听。请执行：warppool listen start"))
		}
	}
	if err := hooks.CheckPort(cfg.Listen.Host, cfg.Listen.Port); err != nil {
		if cfg.Listen.Host == "0.0.0.0" || cfg.Listen.Host == "::" {
			if reachableErr := hooks.CheckReachable(cfg.Listen.Host, cfg.Listen.Port); reachableErr == nil {
				*cfg = config.SetListenEnabled(*cfg, true)
				if err := config.SaveExisting(path, *cfg); err != nil {
					return fmt.Errorf("save config: %w", err)
				}
				return nil
			}
		}
		return fmt.Errorf("%w; %s", err, tr(language, "listener port is occupied by another process. Stop it or run `warppool listen config --port <free-port>` first", "监听端口已被其他进程占用。请先停止占用进程，或执行 `warppool listen config --port <空闲端口>` 修改端口"))
	}
	if runtime.GOOS == "linux" {
		if err := hooks.EnsureService(path); err != nil {
			return err
		}
		if err := hooks.Systemctl("enable", "--now", "warppool-listen.service"); err != nil {
			return err
		}
	} else {
		return fmt.Errorf("%s", tr(language, "registration listener is not enabled", "注册监听未启动"))
	}
	*cfg = config.SetListenEnabled(*cfg, true)
	if err := config.SaveExisting(path, *cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	fmt.Fprintf(prompt.out, "%s\n", tr(language, "registration listener started", "注册监听已启动"))
	return nil
}

func newDeployTokenListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List Deploy Tokens",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(resolvedConfigPath())
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NODE\tPORT\tMODE\tSTATUS\tEXPIRES_AT\tTOKEN")
			now := time.Now().UTC()
			for _, item := range cfg.Tokens {
				fmt.Fprintf(w, "%s\t%d\t%s\t%s\t%s\t%s\n",
					item.Node.Name,
					item.Node.LocalPort,
					item.Node.ExitMode,
					deployTokenStatus(item, now),
					item.ExpiresAt,
					shortDeployToken(item.Token),
				)
			}
			return w.Flush()
		},
	}
	return cmd
}

func newDeployTokenRemoveCommand() *cobra.Command {
	var includeUsed bool
	cmd := &cobra.Command{
		Use:   "remove <token-or-node>",
		Short: "Remove unused Deploy Token by token value or node name",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := resolvedConfigPath()
			cfg, err := config.Load(path)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			cfg, removed := config.RemoveDeployTokens(cfg, args[0], includeUsed)
			if removed == 0 {
				return fmt.Errorf("deploy token not found or already used: %s", args[0])
			}
			if err := config.SaveExisting(path, cfg); err != nil {
				return fmt.Errorf("save config: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "removed deploy tokens: %d\n", removed)
			return nil
		},
	}
	cmd.Flags().BoolVar(&includeUsed, "include-used", false, "also remove used token history")
	return cmd
}

func newDeployTokenPruneCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "prune",
		Short: "Remove expired unused Deploy Tokens",
		RunE: func(cmd *cobra.Command, args []string) error {
			path := resolvedConfigPath()
			cfg, err := config.Load(path)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			cfg, removed := config.PruneExpiredDeployTokens(cfg, time.Now().UTC())
			if err := config.SaveExisting(path, cfg); err != nil {
				return fmt.Errorf("save config: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "pruned deploy tokens: %d\n", removed)
			return nil
		},
	}
	return cmd
}

func newDeployTokenWaitStartCommand() *cobra.Command {
	var timeout time.Duration
	cmd := &cobra.Command{
		Use:    "wait-start <node>",
		Hidden: true,
		Short:  "Wait for Deploy Token registration and start proxy",
		Args:   cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return autoStartProxyAfterRegistration(resolvedConfigPath(), args[0], timeout)
		},
	}
	cmd.Flags().DurationVar(&timeout, "timeout", 2*time.Minute, "wait timeout")
	return cmd
}

func deployTokenStatus(item config.DeployToken, now time.Time) string {
	if item.Used {
		if item.Registered {
			return "registered"
		}
		return "used"
	}
	expiresAt, err := time.Parse(time.RFC3339, item.ExpiresAt)
	if err != nil || now.After(expiresAt) {
		return "expired"
	}
	if item.Prepared {
		return "prepared"
	}
	return "unused"
}

func shortDeployToken(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 12 {
		return value
	}
	return value[:6] + "..." + value[len(value)-6:]
}

func promptDeployTokenOptions(prompt promptIO, cfg config.Config, node *config.Node) error {
	var err error
	language := prompt.language
	node.Name, err = prompt.askRequired(tr(language, "Node name", "节点名称"), node.Name)
	if err != nil {
		return err
	}
	node.ExitMode, err = prompt.askMenu(tr(language, "Exit mode", "出口模式"), node.ExitMode, defaultString(cfg.Defaults.ExitMode, config.ExitModeDirect), []menuOption{
		{Label: "direct", Value: config.ExitModeDirect},
		{Label: "warp", Value: config.ExitModeWarp},
	})
	if err != nil {
		return err
	}
	node.Proxy, err = prompt.askMenu(tr(language, "Local proxy protocol", "本地代理协议"), node.Proxy, defaultString(cfg.Defaults.Proxy, config.ProxyMixed), []menuOption{
		{Label: "mixed", Value: config.ProxyMixed},
		{Label: "socks5", Value: config.ProxySocks5},
		{Label: "http", Value: config.ProxyHTTP},
	})
	if err != nil {
		return err
	}
	bindHost := node.BindHost
	if bindHost == "" {
		bindHost = cfg.Defaults.BindHost
	}
	node.LocalPort, err = promptAvailableLocalPort(prompt, cfg, bindHost, node.LocalPort)
	return err
}

func checkListenReachable(host string, port int) error {
	targetHost := host
	if targetHost == "0.0.0.0" || targetHost == "::" {
		targetHost = "127.0.0.1"
	}

	conn, err := net.DialTimeout("tcp", net.JoinHostPort(targetHost, fmt.Sprintf("%d", port)), 2*time.Second)
	if err != nil {
		return fmt.Errorf("registration listener is not reachable at %s:%d, run listen start first: %w", targetHost, port, err)
	}
	return conn.Close()
}

func autoStartProxyAfterRegistration(configPath string, nodeName string, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		cfg, err := config.Load(configPath)
		if err != nil {
			return err
		}
		node, ok := config.FindNode(cfg, nodeName)
		if ok {
			return startProxyForNode(configPath, cfg, node)
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("timed out waiting for node registration: %s", nodeName)
}

func startProxyForNode(configPath string, cfg config.Config, node config.Node) error {
	if _, err := singbox.BuildJSON(config.Config{Nodes: []config.Node{node}}, singbox.Options{}); err != nil {
		return err
	}
	data, err := buildAndValidateProxyConfig(cfg, singbox.Options{})
	if err != nil {
		return err
	}
	if runtime.GOOS == "linux" {
		if err := singbox.WriteConfig(singbox.DefaultConfigPath(), data); err != nil {
			return fmt.Errorf("write sing-box config: %w", err)
		}
		return startProxyService(configPath, &node)
	}
	_, err = singbox.Start(data, singbox.ManagerOptions{})
	return err
}
