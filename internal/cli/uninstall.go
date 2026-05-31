package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/murongruolan/warp-pool/internal/config"
	"github.com/murongruolan/warp-pool/internal/singbox"
	"github.com/murongruolan/warp-pool/internal/wgclient"
	"github.com/spf13/cobra"
)

type uninstallOptions struct {
	KeepConfig      bool
	KeepBinary      bool
	DryRun          bool
	Force           bool
	CleanWG         bool
	CleanProxy      bool
	CleanWGSet      bool
	CleanProxySet   bool
	BinaryPath      string
	ConfigPath      string
	StateDir        string
	InstallDir      string
	ListenUnit      string
	ProxyUnit       string
	SingBoxUnit     string
	RuntimeOS       string
	Runner          uninstallRunner
	RemoveFile      func(string) error
	RemoveAll       func(string) error
	Stat            func(string) (fs.FileInfo, error)
	Executable      func() (string, error)
	Out             func(string)
	Prompt          promptIO
	SkipInteractive bool
}

type uninstallRunner interface {
	Run(name string, args ...string) ([]byte, error)
}

type uninstallExecRunner struct{}

func (uninstallExecRunner) Run(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}

type uninstallResult struct {
	Logs []string
}

func newUninstallCommand() *cobra.Command {
	var opts uninstallOptions
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Uninstall WarpPool from the main server",
		Long: "Uninstall WarpPool local runtime.\n\n" +
			"Use `warppool node remove <name>` or `warppool remove <name>` to remove a node record.\n" +
			"`uninstall` is only for removing the main server program and local runtime files.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return fmt.Errorf("uninstall does not remove nodes; use `warppool remove %s` or `warppool node remove %s`", args[0], args[0])
			}
			opts.ConfigPath = resolvedConfigPath()
			opts.Out = func(line string) { fmt.Fprintln(cmd.OutOrStdout(), line) }
			language := "en"
			if cfg, err := config.Load(opts.ConfigPath); err == nil {
				language = cfgLanguage(cfg)
			}
			opts.Prompt = newPromptIOWithLanguage(cmd.OutOrStdout(), language)

			if !opts.Force && !opts.DryRun {
				return fmt.Errorf("refusing to uninstall without --force; use --dry-run to preview")
			}

			result, err := uninstallAll(opts)
			for _, log := range result.Logs {
				fmt.Fprintln(cmd.OutOrStdout(), log)
			}
			return err
		},
	}

	cmd.Flags().BoolVar(&opts.KeepConfig, "keep-config", false, "keep config file")
	cmd.Flags().BoolVar(&opts.KeepBinary, "keep-binary", false, "keep warppool binary")
	cmd.Flags().BoolVar(&opts.DryRun, "dry-run", false, "print actions without changing files or services")
	cmd.Flags().BoolVar(&opts.Force, "force", false, "confirm destructive uninstall")
	cmd.Flags().BoolVar(&opts.CleanWG, "clean-wg", false, "also stop and remove local WireGuard client configs")
	cmd.Flags().BoolVar(&opts.CleanProxy, "clean-proxy", false, "also stop and remove local proxy/listener services and runtime state")
	cmd.Flags().StringVar(&opts.BinaryPath, "binary", "", "warppool binary path")
	cmd.Flags().StringVar(&opts.StateDir, "state-dir", "", "WarpPool runtime state directory")
	cmd.Flags().StringVar(&opts.InstallDir, "install-dir", "", "WarpPool installation directory")
	cmd.Flags().StringVar(&opts.ListenUnit, "listen-unit", "", "Deploy Token listener systemd unit path")
	cmd.Flags().StringVar(&opts.ProxyUnit, "proxy-unit", "", "local proxy systemd unit path")
	cmd.Flags().StringVar(&opts.SingBoxUnit, "singbox-unit", "", "sing-box systemd unit path")
	cmd.PreRun = func(cmd *cobra.Command, args []string) {
		opts.CleanWGSet = cmd.Flags().Changed("clean-wg")
		opts.CleanProxySet = cmd.Flags().Changed("clean-proxy")
	}
	return cmd
}

func uninstallAll(opts uninstallOptions) (uninstallResult, error) {
	opts = uninstallDefaults(opts)
	result := uninstallResult{}

	if !opts.CleanWGSet {
		cleanWG, err := opts.Prompt.askBool(tr(opts.Prompt.language, "Remove local WireGuard client configs and disable wg-quick services?", "删除本地 WireGuard 客户端配置并禁用 wg-quick 服务？"), false, true)
		if err != nil {
			return result, err
		}
		opts.CleanWG = cleanWG
	}
	if !opts.CleanProxySet {
		cleanProxy, err := opts.Prompt.askBool(tr(opts.Prompt.language, "Remove local proxy/listener services and runtime state?", "删除本地代理/监听服务和运行状态？"), false, true)
		if err != nil {
			return result, err
		}
		opts.CleanProxy = cleanProxy
	}

	cfg, err := config.Load(opts.ConfigPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return result, fmt.Errorf("load config: %w", err)
	}

	if opts.CleanWG && cfg.Version != 0 {
		for _, node := range cfg.Nodes {
			if err := wgDownBestEffort(node, opts, &result); err != nil {
				return result, err
			}
			device := node.WGLocalDevice
			if device == "" {
				device = wgclient.DefaultLocalDeviceName(node.Name)
			}
			if opts.RuntimeOS == "linux" {
				_ = runBestEffort(opts, &result, "systemctl", "disable", "wg-quick@"+device)
			}
			if node.WGLocalConfigPath != "" {
				if err := removePath(opts, node.WGLocalConfigPath, &result, false); err != nil {
					return result, err
				}
			}
		}
	}

	if opts.CleanProxy {
		if opts.RuntimeOS == "linux" {
			_ = runBestEffort(opts, &result, "systemctl", "disable", "--now", "warppool-proxy.service")
			if err := removePath(opts, opts.ProxyUnit, &result, false); err != nil {
				return result, err
			}
		}
		if opts.DryRun {
			result.append("dry-run: stop sing-box")
		} else if status, stopErr := singbox.Stop(singbox.ManagerOptions{}); stopErr == nil {
			result.append(status.Message)
		} else {
			result.append("skip sing-box stop: " + stopErr.Error())
		}

		if opts.RuntimeOS == "linux" {
			_ = runBestEffort(opts, &result, "systemctl", "disable", "--now", "warppool-listen.service")
			if err := removePath(opts, opts.ListenUnit, &result, false); err != nil {
				return result, err
			}
			if err := removePath(opts, opts.SingBoxUnit, &result, false); err != nil {
				return result, err
			}
			_ = runBestEffort(opts, &result, "systemctl", "daemon-reload")
		}

		if err := removePath(opts, opts.StateDir, &result, true); err != nil {
			return result, err
		}
	}

	if err := removePath(opts, opts.InstallDir, &result, true); err != nil {
		return result, err
	}
	if !opts.KeepConfig {
		if err := removePath(opts, opts.ConfigPath, &result, false); err != nil {
			return result, err
		}
	}
	if !opts.KeepBinary {
		if err := removePath(opts, opts.BinaryPath, &result, false); err != nil {
			return result, err
		}
	}

	result.append("uninstall completed")
	return result, nil
}

func wgDownBestEffort(node config.Node, opts uninstallOptions, result *uninstallResult) error {
	if node.Name == "" {
		return nil
	}
	if opts.DryRun {
		result.append("dry-run: stop WireGuard client for node " + node.Name)
		return nil
	}
	downResult, err := wgclient.Down(node, wgclient.Options{})
	for _, log := range downResult.Logs {
		result.append(log)
	}
	if err != nil {
		result.append("skip WireGuard down for " + node.Name + ": " + err.Error())
	}
	return nil
}

func runBestEffort(opts uninstallOptions, result *uninstallResult, name string, args ...string) error {
	if opts.DryRun {
		result.append("dry-run: " + name + " " + strings.Join(args, " "))
		return nil
	}
	out, err := opts.Runner.Run(name, args...)
	text := strings.TrimSpace(string(out))
	if text != "" {
		result.append(text)
	}
	if err != nil {
		result.append(fmt.Sprintf("skip %s %s: %v", name, strings.Join(args, " "), err))
	}
	return nil
}

func removePath(opts uninstallOptions, path string, result *uninstallResult, recursive bool) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if opts.DryRun {
		result.append("dry-run: remove " + path)
		return nil
	}
	if _, err := opts.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			result.append("not found: " + path)
			return nil
		}
		return err
	}
	if recursive {
		if err := opts.RemoveAll(path); err != nil {
			return fmt.Errorf("remove %s: %w", path, err)
		}
	} else {
		if err := opts.RemoveFile(path); err != nil {
			return fmt.Errorf("remove %s: %w", path, err)
		}
	}
	result.append("removed: " + path)
	return nil
}

func uninstallDefaults(opts uninstallOptions) uninstallOptions {
	if opts.RuntimeOS == "" {
		opts.RuntimeOS = runtime.GOOS
	}
	if opts.Runner == nil {
		opts.Runner = uninstallExecRunner{}
	}
	if opts.RemoveFile == nil {
		opts.RemoveFile = os.Remove
	}
	if opts.RemoveAll == nil {
		opts.RemoveAll = os.RemoveAll
	}
	if opts.Stat == nil {
		opts.Stat = os.Stat
	}
	if opts.Executable == nil {
		opts.Executable = os.Executable
	}
	if opts.ConfigPath == "" {
		opts.ConfigPath = resolvedConfigPath()
	}
	if opts.Prompt.in == nil || opts.Prompt.out == nil {
		opts.Prompt = newPromptIO(os.Stdout)
	}
	if opts.SkipInteractive || opts.DryRun {
		if !opts.CleanWGSet {
			opts.CleanWG = true
			opts.CleanWGSet = true
		}
		if !opts.CleanProxySet {
			opts.CleanProxy = true
			opts.CleanProxySet = true
		}
	}
	if opts.StateDir == "" {
		opts.StateDir = defaultUninstallStateDir(opts.RuntimeOS)
	}
	if opts.InstallDir == "" {
		opts.InstallDir = defaultUninstallInstallDir(opts.RuntimeOS)
	}
	if opts.ListenUnit == "" {
		opts.ListenUnit = "/etc/systemd/system/warppool-listen.service"
	}
	if opts.ProxyUnit == "" {
		opts.ProxyUnit = "/etc/systemd/system/warppool-proxy.service"
	}
	if opts.SingBoxUnit == "" {
		opts.SingBoxUnit = ""
	}
	if opts.BinaryPath == "" {
		if exe, err := opts.Executable(); err == nil {
			opts.BinaryPath = exe
		}
	}
	return opts
}

func defaultUninstallStateDir(runtimeOS string) string {
	if runtimeOS == "windows" {
		base := os.Getenv("ProgramData")
		if base == "" {
			base = "."
		}
		return filepath.Join(base, "warppool")
	}
	return "/var/lib/warppool"
}

func defaultUninstallInstallDir(runtimeOS string) string {
	if runtimeOS == "windows" {
		base := os.Getenv("ProgramFiles")
		if base == "" {
			base = "."
		}
		return filepath.Join(base, "WarpPool")
	}
	return "/usr/local/lib/warppool"
}

func (r *uninstallResult) append(line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	r.Logs = append(r.Logs, line)
}
