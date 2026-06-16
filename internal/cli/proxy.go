package cli

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/murongruolan/warp-pool/internal/config"
	"github.com/murongruolan/warp-pool/internal/singbox"
	"github.com/spf13/cobra"
)

type proxyConfigMode int

const (
	proxyConfigStrict proxyConfigMode = iota
	proxyConfigRestart
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
	cmd.AddCommand(newProxyRunCommand())
	cmd.AddCommand(newProxyServiceCommand())
	return cmd
}

func buildAndValidateProxyConfig(cfg config.Config, opts singbox.Options) ([]byte, error) {
	return buildProxyConfig(cfg, opts, proxyConfigStrict, nil)
}

func buildProxyConfig(cfg config.Config, opts singbox.Options, mode proxyConfigMode, restartingNode *config.Node) ([]byte, error) {
	data, err := singbox.BuildJSON(cfg, opts)
	if err != nil {
		return nil, err
	}
	ignored := map[string]bool(nil)
	if mode == proxyConfigRestart {
		ignored = map[string]bool{}
		for _, node := range cfg.Nodes {
			ignored[singbox.InboundTag(node.Name)] = true
			if node.ExitMode == config.ExitModeDual {
				ignored[singbox.WarpInboundTag(node.Name)] = true
			}
		}
	}
	if err := singbox.CheckInboundPortsExcept(data, ignored); err != nil {
		status, statusErr := singbox.Status(singbox.ManagerOptions{})
		if mode == proxyConfigStrict && statusErr == nil && status.Running {
			return data, nil
		}
		return nil, err
	}
	return data, nil
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
			data, err := buildAndValidateProxyConfig(cfg, sbOpts)
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

func newProxyRunCommand() *cobra.Command {
	var sbOpts singbox.Options
	var mgrOpts singbox.ManagerOptions

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run local sing-box proxy in foreground",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(resolvedConfigPath())
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			data, err := buildAndValidateProxyConfig(cfg, sbOpts)
			if err != nil {
				return err
			}
			return singbox.Run(data, mgrOpts)
		},
	}

	addProxyManagerFlags(cmd, &mgrOpts)
	cmd.Flags().StringVar(&sbOpts.LogLevel, "log-level", "info", "sing-box log level")
	cmd.Flags().IntVar(&sbOpts.MTU, "mtu", 1420, "WireGuard endpoint MTU")
	return cmd
}

func newProxyServiceCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "service",
		Short: "Manage sing-box proxy systemd service",
	}
	cmd.AddCommand(newProxyServiceInstallCommand())
	cmd.AddCommand(newProxyServiceEnableCommand())
	cmd.AddCommand(newProxyServiceDisableCommand())
	return cmd
}

func newProxyServiceInstallCommand() *cobra.Command {
	var unitPath string
	var warppoolBin string
	var singboxBin string

	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install systemd service for local proxy",
		RunE: func(cmd *cobra.Command, args []string) error {
			if runtime.GOOS != "linux" {
				return fmt.Errorf("systemd service installation is only supported on Linux")
			}
			if unitPath != "" && unitPath != "/etc/systemd/system/warppool-proxy.service" {
				return fmt.Errorf("custom proxy unit path is not supported by start/stop yet: %s", unitPath)
			}
			unitPath = "/etc/systemd/system/warppool-proxy.service"
			if warppoolBin == "" {
				bin, err := os.Executable()
				if err != nil {
					return fmt.Errorf("detect warppool executable: %w", err)
				}
				warppoolBin = bin
			}
			service := renderProxyService(warppoolBin, resolvedConfigPath(), singboxBin)
			if err := os.WriteFile(unitPath, []byte(service), 0o644); err != nil {
				return fmt.Errorf("write systemd service %s: %w", unitPath, err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "installed service: %s\n", unitPath)
			return runSystemctl("daemon-reload")
		},
	}
	cmd.Flags().StringVar(&unitPath, "unit-path", "", "systemd unit path")
	cmd.Flags().StringVar(&warppoolBin, "warppool-bin", "", "warppool binary path")
	cmd.Flags().StringVar(&singboxBin, "singbox-bin", "", "sing-box binary path")
	return cmd
}

func ensureProxyServiceInstalled(configPath string) error {
	if runtime.GOOS != "linux" {
		return nil
	}
	unitPath := "/etc/systemd/system/warppool-proxy.service"
	bin, err := os.Executable()
	if err != nil {
		return fmt.Errorf("detect warppool executable: %w", err)
	}
	service := renderProxyService(bin, configPath, singbox.ResolveBinary("", runtime.GOOS))
	if err := os.WriteFile(unitPath, []byte(service), 0o644); err != nil {
		return fmt.Errorf("write systemd service %s: %w", unitPath, err)
	}
	return runSystemctl("daemon-reload")
}

func startProxyService(configPath string, restartingNode *config.Node) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("systemd service is only supported on Linux")
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	data, err := buildProxyConfig(cfg, singbox.Options{}, proxyConfigRestart, restartingNode)
	if err != nil {
		return err
	}
	if err := singbox.WriteConfig(singbox.DefaultConfigPath(), data); err != nil {
		return fmt.Errorf("write sing-box config: %w", err)
	}
	if err := ensureProxyServiceInstalled(configPath); err != nil {
		return err
	}
	if err := runSystemctl("enable", "warppool-proxy.service"); err != nil {
		return err
	}
	if err := runSystemctl("restart", "warppool-proxy.service"); err != nil {
		return err
	}
	return waitProxyServiceRunning(5 * time.Second)
}

func stopProxyService() error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("systemd service is only supported on Linux")
	}
	return runSystemctl("disable", "--now", "warppool-proxy.service")
}

func newProxyServiceEnableCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "enable",
		Short: "Enable local proxy on boot",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := startProxyService(resolvedConfigPath(), nil); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "enabled service: warppool-proxy.service")
			return nil
		},
	}
	return cmd
}

func newProxyServiceDisableCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "disable",
		Short: "Disable local proxy service",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := stopProxyService(); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "disabled service: warppool-proxy.service")
			return nil
		},
	}
	return cmd
}

func renderProxyService(warppoolBin string, configPath string, singboxBin string) string {
	var envLine string
	if singboxBin != "" {
		envLine = fmt.Sprintf("Environment=%s\n", systemdEscapeArg("WARPOOL_SINGBOX_BIN="+singboxBin))
	}
	return fmt.Sprintf(`[Unit]
Description=WarpPool Local Proxy
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
%sExecStart=%s --config %s proxy run
Restart=on-failure
RestartSec=3s

[Install]
WantedBy=multi-user.target
`, envLine, systemdEscapeArg(warppoolBin), systemdEscapeArg(configPath))
}

func waitProxyServiceRunning(timeout time.Duration) error {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	deadline := time.Now().Add(timeout)
	var lastStatus singbox.StatusResult
	var lastErr error
	for time.Now().Before(deadline) {
		status, err := singbox.Status(singbox.ManagerOptions{})
		if err == nil && status.Running {
			return nil
		}
		lastStatus = status
		lastErr = err
		time.Sleep(300 * time.Millisecond)
	}

	base := "local proxy service failed to start"
	if lastErr != nil {
		base = fmt.Sprintf("%s: %v", base, lastErr)
	} else if strings.TrimSpace(lastStatus.Message) != "" {
		base = fmt.Sprintf("%s: %s", base, lastStatus.Message)
	}
	details := proxyServiceDiagnostics()
	if strings.TrimSpace(details) == "" {
		return fmt.Errorf("%s", base)
	}
	return fmt.Errorf("%s\n%s", base, details)
}

func proxyServiceDiagnostics() string {
	sections := []string{}
	for _, item := range []struct {
		title   string
		command string
		args    []string
	}{
		{
			title:   "systemctl status warppool-proxy.service",
			command: "systemctl",
			args:    []string{"status", "warppool-proxy.service", "--no-pager", "-l"},
		},
		{
			title:   "journalctl -u warppool-proxy.service",
			command: "journalctl",
			args:    []string{"-u", "warppool-proxy.service", "-n", "120", "--no-pager"},
		},
	} {
		out, _ := exec.Command(item.command, item.args...).CombinedOutput()
		text := strings.TrimSpace(string(out))
		if text == "" {
			continue
		}
		sections = append(sections, "== "+item.title+" ==\n"+text)
	}
	return strings.Join(sections, "\n\n")
}

func addProxyManagerFlags(cmd *cobra.Command, opts *singbox.ManagerOptions) {
	cmd.Flags().StringVar(&opts.ConfigPath, "singbox-config", "", "sing-box config path")
	cmd.Flags().StringVar(&opts.PIDPath, "pid-file", "", "sing-box pid file path")
	cmd.Flags().StringVar(&opts.Binary, "singbox-bin", "", "sing-box binary path; defaults to bundled bin/sing-box or /usr/local/lib/warppool/bin/sing-box")
	cmd.Flags().StringVar(&opts.BundleDir, "singbox-bundle-dir", "", "directory containing bundled sing-box binary")
	cmd.PreRun = func(cmd *cobra.Command, args []string) {
		if opts.Binary == "" {
			opts.Binary = os.Getenv("WARPOOL_SINGBOX_BIN")
		}
	}
}
