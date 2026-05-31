package cli

import (
	"fmt"

	"github.com/murongruolan/warp-pool/internal/config"
	"github.com/spf13/cobra"
)

func newConfigCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage WarpPool config",
	}

	cmd.AddCommand(newConfigInitCommand())
	return cmd
}

func newConfigInitCommand() *cobra.Command {
	var force bool
	var language string

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create a default config file",
		RunE: func(cmd *cobra.Command, args []string) error {
			path := configPath
			if path == "" {
				path = config.DefaultPath()
			}

			cfg := config.Default()
			if language != "" {
				if err := config.ValidateLanguage(language); err != nil {
					return err
				}
				cfg.Language = config.NormalizeLanguage(language)
			}
			if err := config.Save(path, cfg, force); err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "created config: %s\n", path)
			return nil
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "overwrite existing config")
	cmd.Flags().StringVar(&language, "language", "", "interactive language: zh or en")
	return cmd
}
