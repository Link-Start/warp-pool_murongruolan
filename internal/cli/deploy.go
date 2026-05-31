package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/murongruolan/warp-pool/internal/config"
	"github.com/murongruolan/warp-pool/internal/deploy"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func newDeployCommand() *cobra.Command {
	var opts deploy.PushOptions

	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Push install a node over SSH",
		RunE: func(cmd *cobra.Command, args []string) error {
			path := resolvedConfigPath()
			cfg, err := config.Load(path)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			prompt := newPromptIO(cmd.OutOrStdout())
			if err := promptDeployOptions(prompt, cfg, &opts); err != nil {
				return err
			}

			if opts.SSH.Password == "" {
				opts.SSH.Password = os.Getenv("WARPOOL_SSH_PASSWORD")
			}
			if opts.SSH.Password == "" && opts.SSH.KeyPath == "" {
				password, err := promptPassword("SSH password: ")
				if err != nil {
					return err
				}
				opts.SSH.Password = password
			}

			next, result, err := deploy.Push(cfg, opts)
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

			if err := config.SaveExisting(path, next); err != nil {
				return fmt.Errorf("save config: %w", err)
			}

			if opts.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "validated deploy plan: %s\n", result.Node.Name)
				return nil
			}

			fmt.Fprintf(cmd.OutOrStdout(), "deployed node: %s\n", result.Node.Name)
			return nil
		},
	}

	cmd.Flags().StringVar(&opts.Node.Name, "name", "", "node name")
	cmd.Flags().StringVar(&opts.Node.ExitMode, "exit-mode", "", "exit mode: direct or warp")
	cmd.Flags().StringVar(&opts.Node.Proxy, "proxy", "", "local proxy protocol: socks5, http, or mixed")
	cmd.Flags().StringVar(&opts.Node.BindHost, "bind-host", "127.0.0.1", "local proxy bind host")
	cmd.Flags().IntVar(&opts.Node.LocalPort, "port", 0, "local proxy port")
	cmd.Flags().StringVar(&opts.Node.Country, "country", "", "node country or region")
	cmd.Flags().StringVar(&opts.Node.PublicIP, "public-ip", "", "node public IP")
	cmd.Flags().StringVar(&opts.SSH.Host, "ssh-host", "", "SSH host")
	cmd.Flags().IntVar(&opts.SSH.Port, "ssh-port", 0, "SSH port")
	cmd.Flags().StringVar(&opts.SSH.User, "ssh-user", "", "SSH user")
	cmd.Flags().StringVar(&opts.SSH.Password, "ssh-password", "", "SSH password, or use WARPOOL_SSH_PASSWORD")
	cmd.Flags().StringVar(&opts.SSH.KeyPath, "ssh-key", "", "SSH private key path")
	cmd.Flags().StringVar(&opts.SSH.KnownHostsPath, "known-hosts", "", "known_hosts file path")
	cmd.Flags().BoolVar(&opts.SSH.InsecureIgnoreHostKey, "insecure-skip-host-key-check", false, "skip SSH host key verification")
	cmd.Flags().StringVar(&opts.RemoteDir, "remote-dir", "/tmp/warppool-install", "remote installer directory")
	cmd.Flags().StringVar(&opts.AssetsDir, "assets-dir", "assets", "local assets directory")
	cmd.Flags().StringVar(&opts.WGEndpoint, "wg-endpoint", "", "WireGuard public endpoint host/IP for the main server to connect")
	cmd.Flags().IntVar(&opts.WGEndpointPort, "wg-endpoint-port", 0, "WireGuard public endpoint port, useful for NAT port forwarding")
	cmd.Flags().IntVar(&opts.WGListenPort, "wg-listen-port", 0, "WireGuard listen port on the node")
	cmd.Flags().IntVar(&opts.WarpPort, "warp-forward-port", 14000, "remote transparent TCP redirect port for warp mode")
	cmd.Flags().BoolVar(&opts.SkipWGUp, "skip-wg-up", false, "write WireGuard config but do not start it")
	cmd.Flags().BoolVar(&opts.SkipForwarding, "skip-forwarding", false, "skip direct-mode IPv4 forwarding and NAT rules")
	cmd.Flags().BoolVar(&opts.DryRun, "dry-run", false, "validate and show deploy plan without SSH")
	cmd.Flags().BoolVar(&opts.SkipPortCheck, "skip-port-check", false, "skip system port availability check")

	return cmd
}

func promptDeployOptions(prompt promptIO, cfg config.Config, opts *deploy.PushOptions) error {
	var err error
	opts.Node.Name, err = prompt.askRequired("Node name", opts.Node.Name)
	if err != nil {
		return err
	}
	opts.Node.ExitMode, err = prompt.askMenu("Exit mode", opts.Node.ExitMode, defaultString(cfg.Defaults.ExitMode, config.ExitModeDirect), []menuOption{
		{Label: "direct", Value: config.ExitModeDirect},
		{Label: "warp", Value: config.ExitModeWarp},
	})
	if err != nil {
		return err
	}
	opts.Node.Proxy, err = prompt.askMenu("Local proxy protocol", opts.Node.Proxy, defaultString(cfg.Defaults.Proxy, config.ProxyMixed), []menuOption{
		{Label: "mixed", Value: config.ProxyMixed},
		{Label: "socks5", Value: config.ProxySocks5},
		{Label: "http", Value: config.ProxyHTTP},
	})
	if err != nil {
		return err
	}
	bindHost := opts.Node.BindHost
	if bindHost == "" {
		bindHost = cfg.Defaults.BindHost
	}
	opts.Node.LocalPort, err = promptAvailableLocalPort(prompt, cfg, bindHost, opts.Node.LocalPort)
	if err != nil {
		return err
	}
	opts.SSH.Host, err = prompt.askRequired("SSH host/IP", opts.SSH.Host)
	if err != nil {
		return err
	}
	opts.SSH.Port, err = prompt.askInt("SSH port", opts.SSH.Port, 22)
	if err != nil {
		return err
	}
	opts.SSH.User, err = prompt.askString("SSH user", opts.SSH.User, "root")
	if err != nil {
		return err
	}
	opts.WGListenPort, err = prompt.askInt("WireGuard listen port", opts.WGListenPort, 51820)
	if err != nil {
		return err
	}
	opts.WGEndpoint, err = prompt.askOptionalString("WireGuard public endpoint host/IP, Enter uses SSH host/IP", opts.WGEndpoint)
	if err != nil {
		return err
	}
	if opts.WGEndpoint == "" {
		opts.WGEndpoint = opts.SSH.Host
	}
	opts.WGEndpointPort, err = prompt.askInt("WireGuard public endpoint port", opts.WGEndpointPort, opts.WGListenPort)
	return err
}

func defaultString(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func promptAvailableLocalPort(prompt promptIO, cfg config.Config, bindHost string, current int) (int, error) {
	for {
		port, err := prompt.askRequiredInt("Local proxy port", current)
		if err != nil {
			return 0, err
		}
		if err := validateLocalProxyPort(cfg, bindHost, port); err != nil {
			if current != 0 {
				return 0, err
			}
			fmt.Fprintf(prompt.out, "%v\n", err)
			continue
		}
		return port, nil
	}
}

func validateLocalProxyPort(cfg config.Config, bindHost string, port int) error {
	if bindHost == "" {
		bindHost = cfg.Defaults.BindHost
	}
	for _, node := range cfg.Nodes {
		if node.BindHost == bindHost && node.LocalPort == port {
			return fmt.Errorf("local proxy port already used by node %s: %s:%d", node.Name, bindHost, port)
		}
	}
	return config.CheckPortAvailable(bindHost, port)
}

func promptPassword(prompt string) (string, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return "", fmt.Errorf("ssh password or key path is required; set WARPOOL_SSH_PASSWORD, pass --ssh-key, or run in an interactive terminal")
	}
	fmt.Fprint(os.Stderr, prompt)
	data, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", fmt.Errorf("read ssh password: %w", err)
	}
	password := strings.TrimSpace(string(data))
	if password == "" {
		return "", fmt.Errorf("ssh password cannot be empty")
	}
	return password, nil
}
