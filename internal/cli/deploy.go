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
	cmd.Flags().StringVar(&opts.Node.ExitMode, "exit-mode", config.ExitModeDirect, "exit mode: direct or warp")
	cmd.Flags().StringVar(&opts.Node.Proxy, "proxy", config.ProxyMixed, "local proxy protocol: socks5, http, or mixed")
	cmd.Flags().StringVar(&opts.Node.BindHost, "bind-host", "127.0.0.1", "local proxy bind host")
	cmd.Flags().IntVar(&opts.Node.LocalPort, "port", 0, "local proxy port")
	cmd.Flags().StringVar(&opts.Node.Country, "country", "", "node country or region")
	cmd.Flags().StringVar(&opts.Node.PublicIP, "public-ip", "", "node public IP")
	cmd.Flags().StringVar(&opts.SSH.Host, "ssh-host", "", "SSH host")
	cmd.Flags().IntVar(&opts.SSH.Port, "ssh-port", 22, "SSH port")
	cmd.Flags().StringVar(&opts.SSH.User, "ssh-user", "root", "SSH user")
	cmd.Flags().StringVar(&opts.SSH.Password, "ssh-password", "", "SSH password, or use WARPOOL_SSH_PASSWORD")
	cmd.Flags().StringVar(&opts.SSH.KeyPath, "ssh-key", "", "SSH private key path")
	cmd.Flags().StringVar(&opts.SSH.KnownHostsPath, "known-hosts", "", "known_hosts file path")
	cmd.Flags().BoolVar(&opts.SSH.InsecureIgnoreHostKey, "insecure-skip-host-key-check", false, "skip SSH host key verification")
	cmd.Flags().StringVar(&opts.RemoteDir, "remote-dir", "/tmp/warppool-install", "remote installer directory")
	cmd.Flags().StringVar(&opts.AssetsDir, "assets-dir", "assets", "local assets directory")
	cmd.Flags().StringVar(&opts.WGEndpoint, "wg-endpoint", "", "WireGuard endpoint host/IP, default SSH host")
	cmd.Flags().IntVar(&opts.WGListenPort, "wg-listen-port", 51820, "WireGuard listen port")
	cmd.Flags().IntVar(&opts.WarpPort, "warp-forward-port", 14000, "remote transparent TCP redirect port for warp mode")
	cmd.Flags().BoolVar(&opts.SkipWGUp, "skip-wg-up", false, "write WireGuard config but do not start it")
	cmd.Flags().BoolVar(&opts.SkipForwarding, "skip-forwarding", false, "skip direct-mode IPv4 forwarding and NAT rules")
	cmd.Flags().BoolVar(&opts.DryRun, "dry-run", false, "validate and show deploy plan without SSH")
	cmd.Flags().BoolVar(&opts.SkipPortCheck, "skip-port-check", false, "skip system port availability check")

	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("port")
	return cmd
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
