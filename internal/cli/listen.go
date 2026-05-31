package cli

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/murongruolan/warp-pool/internal/config"
	"github.com/murongruolan/warp-pool/internal/server"
	"github.com/spf13/cobra"
)

func newListenCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "listen",
		Short: "Manage deploy-token registration listener",
	}

	cmd.AddCommand(newListenConfigCommand())
	cmd.AddCommand(newListenRunCommand())
	cmd.AddCommand(newListenStartCommand())
	cmd.AddCommand(newListenStopCommand())
	cmd.AddCommand(newListenStatusCommand())
	cmd.AddCommand(newListenServiceCommand())
	return cmd
}

func newListenConfigCommand() *cobra.Command {
	var host string
	var publicHost string
	var port int

	cmd := &cobra.Command{
		Use:   "config",
		Short: "Configure registration listener",
		RunE: func(cmd *cobra.Command, args []string) error {
			path := resolvedConfigPath()
			cfg, err := config.Load(path)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			next, err := config.SetListenConfig(cfg, config.ListenConfig{
				Host:       host,
				PublicHost: publicHost,
				Port:       port,
				Enabled:    cfg.Listen.Enabled,
			})
			if err != nil {
				return err
			}
			if err := config.SaveExisting(path, next); err != nil {
				return fmt.Errorf("save config: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "listen configured: %s:%d\n", host, port)
			if publicHost != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "public host: %s\n", publicHost)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&host, "host", "0.0.0.0", "listener host")
	cmd.Flags().StringVar(&publicHost, "public-host", "", "public host/IP used in generated install command")
	cmd.Flags().IntVar(&port, "port", 18080, "listener TCP port")
	return cmd
}

func newListenStartCommand() *cobra.Command {
	return newListenStartCommandWithHooks(ensureListenServiceInstalled, runSystemctl, runtime.GOOS)
}

func newListenStartCommandWithHooks(ensureService func(string) error, systemctl func(...string) error, runtimeOS string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start registration listener service",
		RunE: func(cmd *cobra.Command, args []string) error {
			path := resolvedConfigPath()
			cfg, err := config.Load(path)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			language := cfgLanguage(cfg)
			if runtimeOS != "linux" {
				return runListenForeground(cmd, "")
			}
			if err := ensureService(path); err != nil {
				return err
			}
			if err := systemctl("enable", "--now", "warppool-listen.service"); err != nil {
				return err
			}
			cfg = config.SetListenEnabled(cfg, true)
			if err := config.SaveExisting(path, cfg); err != nil {
				return fmt.Errorf("save config: %w", err)
			}
			publicHost := cfg.Listen.PublicHost
			if publicHost == "" {
				publicHost = cfg.Listen.Host
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n", tr(language, "registration listener service started:", "注册监听服务已启动："), listenURL(publicHost, cfg.Listen.Port))
			fmt.Fprintln(cmd.OutOrStdout(), tr(language, "stop it with: warppool listen stop", "停止监听命令：warppool listen stop"))
			fmt.Fprintln(cmd.OutOrStdout(), cloudSecurityGroupReminder(language, fmt.Sprintf("%d/tcp", cfg.Listen.Port)))
			return nil
		},
	}

	return cmd
}

func newListenRunCommand() *cobra.Command {
	var publicHost string
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run registration listener in foreground",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runListenForeground(cmd, publicHost)
		},
	}
	cmd.Flags().StringVar(&publicHost, "public-host", "", "public host/IP used in generated install command")
	return cmd
}

func runListenForeground(cmd *cobra.Command, publicHost string) error {
	path := resolvedConfigPath()
	cfg, err := config.Load(path)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	language := cfgLanguage(cfg)
	if err := config.CheckPortAvailable(cfg.Listen.Host, cfg.Listen.Port); err != nil {
		return err
	}

	cfg = config.SetListenEnabled(cfg, true)
	if err := config.SaveExisting(path, cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	addr := net.JoinHostPort(cfg.Listen.Host, fmt.Sprintf("%d", cfg.Listen.Port))
	srv := &http.Server{
		Addr:              addr,
		Handler:           server.RegisterHandler(path),
		ReadHeaderTimeout: 10 * time.Second,
	}

	if publicHost == "" {
		publicHost = cfg.Listen.PublicHost
	}
	if publicHost == "" {
		publicHost = cfg.Listen.Host
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n", tr(language, "registration listener started:", "注册监听已启动："), listenURL(publicHost, cfg.Listen.Port))
	fmt.Fprintln(cmd.OutOrStdout(), tr(language, "press Ctrl+C to stop", "按 Ctrl+C 停止"))
	fmt.Fprintln(cmd.OutOrStdout(), cloudSecurityGroupReminder(language, fmt.Sprintf("%d/tcp", cfg.Listen.Port)))

	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n", tr(language, "received signal:", "收到信号："), sig)
	case err := <-errCh:
		if err != nil {
			return err
		}
	case <-watchListenStop(path, 1*time.Second):
		fmt.Fprintln(cmd.OutOrStdout(), tr(language, "listener stop requested", "收到监听停止请求"))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown listener: %w", err)
	}

	cfg, err = config.Load(path)
	if err == nil {
		cfg = config.SetListenEnabled(cfg, false)
		_ = config.SaveExisting(path, cfg)
	}
	fmt.Fprintln(cmd.OutOrStdout(), tr(language, "registration listener stopped", "注册监听已停止"))
	return nil
}

func watchListenStop(configPath string, interval time.Duration) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		defer close(done)

		for range ticker.C {
			cfg, err := config.Load(configPath)
			if err != nil {
				continue
			}
			if !cfg.Listen.Enabled {
				return
			}
		}
	}()
	return done
}

func newListenStopCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop registration listener service",
		RunE: func(cmd *cobra.Command, args []string) error {
			path := resolvedConfigPath()
			cfg, err := config.Load(path)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			if runtime.GOOS == "linux" {
				_ = runSystemctl("disable", "--now", "warppool-listen.service")
			}

			cfg = config.SetListenEnabled(cfg, false)
			if err := config.SaveExisting(path, cfg); err != nil {
				return fmt.Errorf("save config: %w", err)
			}

			fmt.Fprintln(cmd.OutOrStdout(), tr(cfgLanguage(cfg), "registration listener stopped", "注册监听已停止"))
			return nil
		},
	}
	return cmd
}

func newListenStatusCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show registration listener status",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(resolvedConfigPath())
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			status := "stopped"
			if cfg.Listen.Enabled {
				status = "enabled"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "status: %s\n", status)
			fmt.Fprintf(cmd.OutOrStdout(), "listen: %s:%d\n", cfg.Listen.Host, cfg.Listen.Port)
			if cfg.Listen.PublicHost != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "public host: %s\n", cfg.Listen.PublicHost)
			}
			return nil
		},
	}
	return cmd
}

func listenURL(host string, port int) string {
	if host == "0.0.0.0" || host == "::" {
		host = "<主服务器IP>"
	}
	return fmt.Sprintf("http://%s:%d", host, port)
}

func newListenServiceCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "service",
		Short: "Manage Deploy Token listener systemd service",
	}

	cmd.AddCommand(newListenServiceInstallCommand())
	cmd.AddCommand(newListenServiceEnableCommand())
	cmd.AddCommand(newListenServiceDisableCommand())
	return cmd
}

func newListenServiceInstallCommand() *cobra.Command {
	var unitPath string
	var warppoolBin string

	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install systemd service for Deploy Token listener",
		RunE: func(cmd *cobra.Command, args []string) error {
			if runtime.GOOS != "linux" {
				return fmt.Errorf("systemd service installation is only supported on Linux")
			}
			if unitPath != "" && unitPath != "/etc/systemd/system/warppool-listen.service" {
				return fmt.Errorf("custom listener unit path is not supported by start/stop yet: %s", unitPath)
			}
			if warppoolBin == "" {
				bin, err := os.Executable()
				if err != nil {
					return fmt.Errorf("detect warppool executable: %w", err)
				}
				warppoolBin = bin
			}

			unitPath = "/etc/systemd/system/warppool-listen.service"
			service := renderListenService(warppoolBin, resolvedConfigPath())
			if err := os.WriteFile(unitPath, []byte(service), 0o644); err != nil {
				return fmt.Errorf("write systemd service %s: %w", unitPath, err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "installed service: %s\n", unitPath)
			if err := runSystemctl("daemon-reload"); err != nil {
				return err
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&unitPath, "unit-path", "", "systemd unit path")
	cmd.Flags().StringVar(&warppoolBin, "warppool-bin", "", "warppool binary path")
	return cmd
}

func newListenServiceEnableCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "enable",
		Short: "Enable and start Deploy Token listener",
		RunE: func(cmd *cobra.Command, args []string) error {
			if runtime.GOOS != "linux" {
				return fmt.Errorf("systemd service is only supported on Linux")
			}
			path := resolvedConfigPath()
			cfg, err := config.Load(path)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			if err := ensureListenServiceInstalled(path); err != nil {
				return err
			}
			if err := runSystemctl("enable", "--now", "warppool-listen.service"); err != nil {
				return err
			}
			cfg = config.SetListenEnabled(cfg, true)
			if err := config.SaveExisting(path, cfg); err != nil {
				return fmt.Errorf("save config: %w", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "enabled service: warppool-listen.service")
			return nil
		},
	}
	return cmd
}

func newListenServiceDisableCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "disable",
		Short: "Disable and stop Deploy Token listener",
		RunE: func(cmd *cobra.Command, args []string) error {
			if runtime.GOOS != "linux" {
				return fmt.Errorf("systemd service is only supported on Linux")
			}
			path := resolvedConfigPath()
			cfg, err := config.Load(path)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			if err := runSystemctl("disable", "--now", "warppool-listen.service"); err != nil {
				return err
			}
			cfg = config.SetListenEnabled(cfg, false)
			if err := config.SaveExisting(path, cfg); err != nil {
				return fmt.Errorf("save config: %w", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "disabled service: warppool-listen.service")
			return nil
		},
	}
	return cmd
}

func renderListenService(warppoolBin string, configPath string) string {
	return fmt.Sprintf(`[Unit]
Description=WarpPool Deploy Token Listener
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=%s --config %s listen run
Restart=on-failure
RestartSec=3s

[Install]
WantedBy=multi-user.target
`, systemdEscapeArg(warppoolBin), systemdEscapeArg(configPath))
}

func ensureListenServiceInstalled(configPath string) error {
	if runtime.GOOS != "linux" {
		return nil
	}
	unitPath := "/etc/systemd/system/warppool-listen.service"
	bin, err := os.Executable()
	if err != nil {
		return fmt.Errorf("detect warppool executable: %w", err)
	}
	service := renderListenService(bin, configPath)
	if err := os.WriteFile(unitPath, []byte(service), 0o644); err != nil {
		return fmt.Errorf("write systemd service %s: %w", unitPath, err)
	}
	return runSystemctl("daemon-reload")
}

func systemdEscapeArg(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func runSystemctl(args ...string) error {
	out, err := exec.Command("systemctl", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl %v failed: %w: %s", args, err, strings.TrimSpace(string(out)))
	}
	return nil
}
