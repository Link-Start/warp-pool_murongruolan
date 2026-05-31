package cli

import (
	"encoding/json"
	"fmt"
	"text/tabwriter"

	"github.com/murongruolan/warp-pool/internal/config"
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

			data, err := json.MarshalIndent(node, "", "  ")
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return nil
		},
	}
	return cmd
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
