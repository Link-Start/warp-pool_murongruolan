package cli

import (
	"io"
	"os"

	"github.com/spf13/cobra"
)

var configPath string
var inputReader io.Reader = defaultInputReader()

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func Execute() error {
	root := NewRootCommand()
	return root.Execute()
}

func defaultInputReader() io.Reader {
	return os.Stdin
}

func NewRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:           "warppool",
		Short:         "WarpPool manages WireGuard based exit nodes",
		Long:          "WarpPool manages WireGuard based exit nodes with optional Cloudflare WARP egress.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.PersistentFlags().StringVar(&configPath, "config", "", "config file path")
	root.AddCommand(newConfigCommand())
	root.AddCommand(newDeployCommand())
	root.AddCommand(newDeployTokenCommand())
	root.AddCommand(newDoctorCommand())
	root.AddCommand(newExportCommand())
	root.AddCommand(newListenCommand())
	root.AddCommand(newNodeCommand())
	root.AddCommand(newPingCommand())
	root.AddCommand(newProxyCommand())
	root.AddCommand(newRemoveCommand())
	root.AddCommand(newShowCommand())
	root.AddCommand(newSpeedtestCommand())
	root.AddCommand(newUninstallCommand())
	root.AddCommand(newUpgradeCommand())
	root.AddCommand(newVersionCommand())
	root.AddCommand(newWGCommand())

	return root
}
