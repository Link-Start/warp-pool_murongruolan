package cli

import (
	"fmt"
	"net"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/murongruolan/warp-pool/internal/config"
	"github.com/murongruolan/warp-pool/internal/token"
	"github.com/spf13/cobra"
)

func newDeployTokenCommand() *cobra.Command {
	cmd := newDeployTokenCreateCommand()
	cmd.AddCommand(newDeployTokenListCommand())
	cmd.AddCommand(newDeployTokenRemoveCommand())
	cmd.AddCommand(newDeployTokenPruneCommand())
	return cmd
}

func newDeployTokenCreateCommand() *cobra.Command {
	var node config.Node
	var ttl time.Duration
	var publicHost string
	var repoBaseURL string
	var endpoint string
	var wgListenPort int
	var wgEndpointPort int

	cmd := &cobra.Command{
		Use:   "deploy-token",
		Short: "Generate one-time pull install token",
		RunE: func(cmd *cobra.Command, args []string) error {
			path := resolvedConfigPath()
			cfg, err := config.Load(path)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			prompt := newPromptIO(cmd.OutOrStdout())
			if err := promptDeployTokenOptions(prompt, cfg, &node); err != nil {
				return err
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
				repoBaseURL = "https://raw.githubusercontent.com/murongruolan/warp-pool/main/assets"
			}

			fmt.Fprintf(cmd.OutOrStdout(), "token expires at: %s\n", expiresAt.Format(time.RFC3339))
			fmt.Fprintln(cmd.OutOrStdout(), "install command:")
			installArgs := fmt.Sprintf("mode=%s token=%s server=%s", node.ExitMode, tokenValue, serverURL)
			if endpoint != "" {
				installArgs += fmt.Sprintf(" endpoint=%s", endpoint)
			}
			if wgListenPort != 0 {
				installArgs += fmt.Sprintf(" wg_listen_port=%d", wgListenPort)
			}
			if wgEndpointPort != 0 {
				installArgs += fmt.Sprintf(" wg_endpoint_port=%d", wgEndpointPort)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "wget -qO- %s/install.sh | sudo bash -s -- %s\n", repoBaseURL, installArgs)
			fmt.Fprintln(cmd.OutOrStdout(), "or:")
			fmt.Fprintf(cmd.OutOrStdout(), "curl -fsSL %s/install.sh | sudo bash -s -- %s\n", repoBaseURL, installArgs)
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

	return cmd
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
	node.Name, err = prompt.askRequired("Node name", node.Name)
	if err != nil {
		return err
	}
	node.ExitMode, err = prompt.askMenu("Exit mode", node.ExitMode, defaultString(cfg.Defaults.ExitMode, config.ExitModeDirect), []menuOption{
		{Label: "direct", Value: config.ExitModeDirect},
		{Label: "warp", Value: config.ExitModeWarp},
	})
	if err != nil {
		return err
	}
	node.Proxy, err = prompt.askMenu("Local proxy protocol", node.Proxy, defaultString(cfg.Defaults.Proxy, config.ProxyMixed), []menuOption{
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
