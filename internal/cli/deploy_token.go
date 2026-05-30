package cli

import (
	"fmt"
	"net"
	"time"

	"github.com/murongruolan/warp-pool/internal/config"
	"github.com/murongruolan/warp-pool/internal/token"
	"github.com/spf13/cobra"
)

func newDeployTokenCommand() *cobra.Command {
	var node config.Node
	var ttl time.Duration
	var publicHost string
	var repoBaseURL string

	cmd := &cobra.Command{
		Use:   "deploy-token",
		Short: "Generate one-time pull install token",
		RunE: func(cmd *cobra.Command, args []string) error {
			path := resolvedConfigPath()
			cfg, err := config.Load(path)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			if !cfg.Listen.Enabled {
				return fmt.Errorf("registration listener is not enabled, run: warppool listen start")
			}
			if err := checkListenReachable(cfg.Listen.Host, cfg.Listen.Port); err != nil {
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
				repoBaseURL = "https://raw.githubusercontent.com/murongruolan/warp-pool/developer/assets"
			}

			fmt.Fprintf(cmd.OutOrStdout(), "token expires at: %s\n", expiresAt.Format(time.RFC3339))
			fmt.Fprintln(cmd.OutOrStdout(), "install command:")
			fmt.Fprintf(cmd.OutOrStdout(), "curl -fsSL %s/install.sh | bash -s -- mode=%s token=%s server=%s\n", repoBaseURL, node.ExitMode, tokenValue, serverURL)
			return nil
		},
	}

	cmd.Flags().StringVar(&node.Name, "name", "", "node name")
	cmd.Flags().StringVar(&node.ExitMode, "exit-mode", config.ExitModeDirect, "exit mode: direct or warp")
	cmd.Flags().StringVar(&node.Proxy, "proxy", config.ProxyMixed, "local proxy protocol: socks5, http, or mixed")
	cmd.Flags().StringVar(&node.BindHost, "bind-host", "127.0.0.1", "local proxy bind host")
	cmd.Flags().IntVar(&node.LocalPort, "port", 0, "local proxy port")
	cmd.Flags().StringVar(&node.Country, "country", "", "node country or region")
	cmd.Flags().StringVar(&node.PublicIP, "public-ip", "", "node public IP")
	cmd.Flags().DurationVar(&ttl, "ttl", 30*time.Minute, "token TTL")
	cmd.Flags().StringVar(&publicHost, "public-host", "", "public host/IP for generated install command")
	cmd.Flags().StringVar(&repoBaseURL, "repo-base-url", "", "installer assets base URL")

	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("port")
	return cmd
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
