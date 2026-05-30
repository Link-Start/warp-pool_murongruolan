package cli

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
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
	cmd.AddCommand(newListenStartCommand())
	cmd.AddCommand(newListenStopCommand())
	cmd.AddCommand(newListenStatusCommand())
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
	var publicHost string

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start registration listener",
		RunE: func(cmd *cobra.Command, args []string) error {
			path := resolvedConfigPath()
			cfg, err := config.Load(path)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
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
			fmt.Fprintf(cmd.OutOrStdout(), "registration listener started: %s\n", listenURL(publicHost, cfg.Listen.Port))
			fmt.Fprintln(cmd.OutOrStdout(), "press Ctrl+C to stop")

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
				fmt.Fprintf(cmd.OutOrStdout(), "received signal: %s\n", sig)
			case err := <-errCh:
				if err != nil {
					return err
				}
			case <-watchListenStop(path, 1*time.Second):
				fmt.Fprintln(cmd.OutOrStdout(), "listener stop requested")
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
			fmt.Fprintln(cmd.OutOrStdout(), "registration listener stopped")
			return nil
		},
	}

	cmd.Flags().StringVar(&publicHost, "public-host", "", "public host/IP used in generated install command")
	return cmd
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
		Short: "Mark registration listener as stopped",
		RunE: func(cmd *cobra.Command, args []string) error {
			path := resolvedConfigPath()
			cfg, err := config.Load(path)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			cfg = config.SetListenEnabled(cfg, false)
			if err := config.SaveExisting(path, cfg); err != nil {
				return fmt.Errorf("save config: %w", err)
			}

			fmt.Fprintln(cmd.OutOrStdout(), "registration listener marked stopped")
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
