package cli

import (
	"github.com/spf13/cobra"
)

var configPath string

func Execute() error {
	root := NewRootCommand()
	return root.Execute()
}

func NewRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "warppool",
		Short: "WarpPool manages WireGuard based exit nodes",
		Long:  "WarpPool manages WireGuard based exit nodes with optional Cloudflare WARP egress.",
	}

	root.PersistentFlags().StringVar(&configPath, "config", "", "config file path")
	root.AddCommand(newConfigCommand())
	root.AddCommand(newDeployCommand())
	root.AddCommand(newDeployTokenCommand())
	root.AddCommand(newExportCommand())
	root.AddCommand(newListenCommand())
	root.AddCommand(newNodeCommand())
	root.AddCommand(newProxyCommand())
	root.AddCommand(newWGCommand())

	return root
}
