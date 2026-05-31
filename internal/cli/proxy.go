package cli

import (
	"fmt"
	"strings"

	"github.com/murongruolan/warp-pool/internal/config"
	"github.com/murongruolan/warp-pool/internal/singbox"
	"github.com/spf13/cobra"
)

func newProxyCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "proxy",
		Short: "Manage local sing-box proxy",
	}

	cmd.AddCommand(newProxyConfigCommand())
	cmd.AddCommand(newProxyStartCommand())
	cmd.AddCommand(newProxyStopCommand())
	cmd.AddCommand(newProxyStatusCommand())
	return cmd
}

func newProxyConfigCommand() *cobra.Command {
	var outPath string
	var opts singbox.Options

	cmd := &cobra.Command{
		Use:   "config",
		Short: "Generate sing-box config",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(resolvedConfigPath())
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			data, err := singbox.BuildJSON(cfg, opts)
			if err != nil {
				return err
			}
			if outPath != "" {
				if err := singbox.WriteConfig(outPath, data); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "wrote sing-box config: %s\n", outPath)
				return nil
			}
			fmt.Fprint(cmd.OutOrStdout(), string(data))
			return nil
		},
	}

	cmd.Flags().StringVarP(&outPath, "output", "o", "", "write config to file")
	cmd.Flags().StringVar(&opts.LogLevel, "log-level", "info", "sing-box log level")
	cmd.Flags().IntVar(&opts.MTU, "mtu", 1420, "WireGuard endpoint MTU")
	return cmd
}

func newProxyStartCommand() *cobra.Command {
	var sbOpts singbox.Options
	var mgrOpts singbox.ManagerOptions

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start local sing-box proxy",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(resolvedConfigPath())
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			data, err := singbox.BuildJSON(cfg, sbOpts)
			if err != nil {
				return err
			}
			result, err := singbox.Start(data, mgrOpts)
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

	addProxyManagerFlags(cmd, &mgrOpts)
	cmd.Flags().StringVar(&sbOpts.LogLevel, "log-level", "info", "sing-box log level")
	cmd.Flags().IntVar(&sbOpts.MTU, "mtu", 1420, "WireGuard endpoint MTU")
	return cmd
}

func newProxyStopCommand() *cobra.Command {
	var mgrOpts singbox.ManagerOptions

	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop local sing-box proxy",
		RunE: func(cmd *cobra.Command, args []string) error {
			status, err := singbox.Stop(mgrOpts)
			if status.Message != "" {
				fmt.Fprintln(cmd.OutOrStdout(), status.Message)
			}
			return err
		},
	}

	addProxyManagerFlags(cmd, &mgrOpts)
	return cmd
}

func newProxyStatusCommand() *cobra.Command {
	var mgrOpts singbox.ManagerOptions

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show local sing-box proxy status",
		RunE: func(cmd *cobra.Command, args []string) error {
			status, err := singbox.Status(mgrOpts)
			if status.Message != "" {
				fmt.Fprintln(cmd.OutOrStdout(), status.Message)
			}
			return err
		},
	}

	addProxyManagerFlags(cmd, &mgrOpts)
	return cmd
}

func addProxyManagerFlags(cmd *cobra.Command, opts *singbox.ManagerOptions) {
	cmd.Flags().StringVar(&opts.ConfigPath, "singbox-config", "", "sing-box config path")
	cmd.Flags().StringVar(&opts.PIDPath, "pid-file", "", "sing-box pid file path")
	cmd.Flags().StringVar(&opts.Binary, "singbox-bin", "", "sing-box binary path; defaults to bundled bin/sing-box or /usr/local/lib/warppool/bin/sing-box")
	cmd.Flags().StringVar(&opts.BundleDir, "singbox-bundle-dir", "", "directory containing bundled sing-box binary")
}
