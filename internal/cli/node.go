package cli

import (
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"text/tabwriter"

	"github.com/murongruolan/warp-pool/internal/config"
	"github.com/murongruolan/warp-pool/internal/singbox"
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
	cmd.Flags().StringVar(&node.ExitMode, "exit-mode", config.ExitModeDirect, "exit mode: direct or warp")
	cmd.Flags().StringVar(&node.Proxy, "proxy", config.ProxyMixed, "local proxy protocol: socks5, http, or mixed")
	cmd.Flags().StringVar(&node.BindHost, "bind-host", "127.0.0.1", "local proxy bind host")
	cmd.Flags().IntVar(&node.LocalPort, "port", 0, "local proxy port")
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
				listen := fmt.Sprintf("%s:%d", node.BindHost, node.LocalPort)
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
			fmt.Fprintf(cmd.OutOrStdout(), "started local proxy service for node: %s\n", args[0])
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

func printNodeDetails(out interface{ Write([]byte) (int, error) }, language string, node config.Node, includeRuntime bool) error {
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	printNodeField(w, language, "name", "节点名称", node.Name)
	printNodeField(w, language, "exit_mode", "出口模式", node.ExitMode)
	printNodeField(w, language, "proxy", "本地代理协议", node.Proxy)
	printNodeField(w, language, "listen", "本地代理监听", fmt.Sprintf("%s:%d", node.BindHost, node.LocalPort))
	printNodeField(w, language, "public_ip", "公网IP", node.PublicIP)
	printNodeField(w, language, "country", "地区", node.Country)
	printNodeField(w, language, "wg_device", "远端 WireGuard 设备", node.WGDevice)
	printNodeField(w, language, "wg_local_device", "本地 WireGuard 设备", defaultIfEmpty(node.WGLocalDevice, wgclient.DefaultLocalDeviceName(node.Name)))
	printNodeField(w, language, "wg_server_address", "WireGuard 服务端地址", node.WGServerAddress)
	printNodeField(w, language, "wg_client_address", "WireGuard 客户端地址", node.WGClientAddress)
	printNodeField(w, language, "wg_listen_port", "WireGuard 监听端口", intString(node.WGListenPort))
	printNodeField(w, language, "endpoint", "WireGuard 公网端点", node.Endpoint)
	printNodeField(w, language, "created_at", "创建时间", node.CreatedAt)
	printNodeField(w, language, "last_updated", "更新时间", node.LastUpdated)

	if includeRuntime {
		if status, err := wgclient.GetStatus(node, wgclient.Options{}); err == nil {
			printNodeField(w, language, "wireguard_active", "WireGuard 已启动", fmt.Sprintf("%t", status.Active))
			if strings.TrimSpace(status.Output) != "" {
				printNodeField(w, language, "wireguard_status", "WireGuard 状态", compactMultiline(status.Output))
			}
		} else {
			printNodeField(w, language, "wireguard_error", "WireGuard 错误", err.Error())
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
	var cleanWG bool

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

			fmt.Fprintf(cmd.OutOrStdout(), "removed node: %s\n", removed.Name)
			return nil
		},
	}
	cmd.Flags().BoolVar(&cleanWG, "clean-wg", false, "also stop and remove local WireGuard client config for this node")
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

func resolvedConfigPath() string {
	if configPath != "" {
		return configPath
	}
	return config.DefaultPath()
}
