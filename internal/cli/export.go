package cli

import (
	"fmt"
	"os"

	"github.com/murongruolan/warp-pool/internal/config"
	"github.com/murongruolan/warp-pool/internal/exporter"
	"github.com/spf13/cobra"
)

func newExportCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export proxy client configs",
	}

	cmd.AddCommand(newExportClashCommand())
	return cmd
}

func newExportClashCommand() *cobra.Command {
	var output string
	var proxyType string

	cmd := &cobra.Command{
		Use:   "clash",
		Short: "Export Clash compatible YAML",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(resolvedConfigPath())
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			data, err := exporter.Clash(cfg, exporter.ClashOptions{ProxyType: proxyType})
			if err != nil {
				return err
			}

			if output == "" {
				fmt.Fprint(cmd.OutOrStdout(), data)
				return nil
			}

			if err := os.WriteFile(output, []byte(data), 0o644); err != nil {
				return fmt.Errorf("write clash config: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "exported clash config: %s\n", output)
			return nil
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "", "output file path, default stdout")
	cmd.Flags().StringVar(&proxyType, "proxy-type", "", "force clash proxy type: socks5 or http")
	return cmd
}
