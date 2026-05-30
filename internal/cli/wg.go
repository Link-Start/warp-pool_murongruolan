package cli

import (
	"fmt"
	"strings"

	"github.com/murongruolan/warp-pool/internal/config"
	"github.com/murongruolan/warp-pool/internal/wgclient"
	"github.com/spf13/cobra"
)

func newWGCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "wg",
		Short: "Manage local WireGuard clients",
	}

	cmd.AddCommand(newWGConfigCommand())
	cmd.AddCommand(newWGUpCommand())
	cmd.AddCommand(newWGDownCommand())
	cmd.AddCommand(newWGStatusCommand())
	return cmd
}

func newWGConfigCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config <node>",
		Short: "Print WireGuard client config",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			node, err := loadNode(args[0])
			if err != nil {
				return err
			}
			if strings.TrimSpace(node.WGClientConfig) == "" {
				return fmt.Errorf("node %s has no WireGuard client config; deploy it first", node.Name)
			}
			fmt.Fprint(cmd.OutOrStdout(), node.WGClientConfig)
			return nil
		},
	}
	return cmd
}

func newWGUpCommand() *cobra.Command {
	var opts wgclient.Options

	cmd := &cobra.Command{
		Use:   "up <node>",
		Short: "Write and start a local WireGuard client",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := resolvedConfigPath()
			cfg, node, err := loadConfigAndNode(path, args[0])
			if err != nil {
				return err
			}

			result, err := wgclient.PrepareUp(node, opts)
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

			cfg, err = config.UpdateNode(cfg, result.Node)
			if err != nil {
				return err
			}
			if err := config.SaveExisting(path, cfg); err != nil {
				return fmt.Errorf("save config: %w", err)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&opts.ConfigDir, "config-dir", "", "WireGuard client config directory")
	cmd.Flags().BoolVar(&opts.SkipSystem, "skip-system", false, "write config only, do not run wg-quick")
	return cmd
}

func newWGDownCommand() *cobra.Command {
	var opts wgclient.Options

	cmd := &cobra.Command{
		Use:   "down <node>",
		Short: "Stop a local WireGuard client",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			node, err := loadNode(args[0])
			if err != nil {
				return err
			}

			result, err := wgclient.Down(node, opts)
			for _, item := range result.Logs {
				item = strings.TrimSpace(item)
				if item == "" {
					continue
				}
				fmt.Fprintln(cmd.OutOrStdout(), item)
			}
			return err
		},
	}

	cmd.Flags().BoolVar(&opts.SkipSystem, "skip-system", false, "skip wg-quick down")
	return cmd
}

func newWGStatusCommand() *cobra.Command {
	var opts wgclient.Options

	cmd := &cobra.Command{
		Use:   "status <node>",
		Short: "Show local WireGuard client status",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			node, err := loadNode(args[0])
			if err != nil {
				return err
			}

			status, err := wgclient.GetStatus(node, opts)
			if strings.TrimSpace(status.Output) != "" {
				fmt.Fprintln(cmd.OutOrStdout(), status.Output)
			}
			if status.Active {
				fmt.Fprintf(cmd.OutOrStdout(), "WireGuard client active: %s\n", status.Node.WGLocalDevice)
			}
			return err
		},
	}

	cmd.Flags().BoolVar(&opts.SkipSystem, "skip-system", false, "skip wg show")
	return cmd
}

func loadNode(name string) (config.Node, error) {
	_, node, err := loadConfigAndNode(resolvedConfigPath(), name)
	return node, err
}

func loadConfigAndNode(path string, name string) (config.Config, config.Node, error) {
	cfg, err := config.Load(path)
	if err != nil {
		return cfg, config.Node{}, fmt.Errorf("load config: %w", err)
	}
	node, ok := config.FindNode(cfg, name)
	if !ok {
		return cfg, config.Node{}, fmt.Errorf("node not found: %s", name)
	}
	return cfg, node, nil
}
