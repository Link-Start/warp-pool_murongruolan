package wgclient

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/murongruolan/warp-pool/internal/config"
	"github.com/murongruolan/warp-pool/internal/wireguard"
)

type RuntimeOS string

const (
	RuntimeLinux   RuntimeOS = "linux"
	RuntimeWindows RuntimeOS = "windows"
)

type Options struct {
	Runtime    RuntimeOS
	ConfigDir  string
	Runner     CommandRunner
	SkipSystem bool
	EnableBoot bool
}

type Result struct {
	Node config.Node
	Logs []string
}

type Status struct {
	Node    config.Node
	Output  string
	Runtime RuntimeOS
	Active  bool
}

type CommandRunner interface {
	Run(name string, args ...string) ([]byte, error)
}

type execRunner struct{}

func (execRunner) Run(name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	return cmd.CombinedOutput()
}

func PrepareUp(node config.Node, opts Options) (Result, error) {
	if err := validateNodeForClient(node); err != nil {
		return Result{}, err
	}

	opts = withDefaults(opts)
	device := node.WGLocalDevice
	if device == "" {
		device = DefaultLocalDeviceName(node.Name)
	}

	if err := os.MkdirAll(opts.ConfigDir, 0o755); err != nil {
		return Result{}, fmt.Errorf("create WireGuard config dir: %w", err)
	}

	path := filepath.Join(opts.ConfigDir, device+".conf")
	if err := os.WriteFile(path, []byte(node.WGClientConfig), 0o600); err != nil {
		return Result{}, fmt.Errorf("write WireGuard client config: %w", err)
	}

	node.WGLocalDevice = device
	node.WGLocalConfigPath = path
	result := Result{
		Node: node,
		Logs: []string{"wrote WireGuard client config: " + path},
	}

	if opts.Runtime != RuntimeLinux || opts.SkipSystem {
		result.Logs = append(result.Logs, fmt.Sprintf("%s runtime: skip wg-quick up, import/start config manually if needed", opts.Runtime))
		return result, nil
	}

	output, err := opts.Runner.Run("wg-quick", "up", path)
	if len(output) > 0 {
		result.Logs = append(result.Logs, strings.TrimSpace(string(output)))
	}
	if err != nil {
		return result, fmt.Errorf("wg-quick up %s: %w", path, err)
	}
	result.Logs = append(result.Logs, "WireGuard client started: "+device)

	if opts.EnableBoot {
		output, err = opts.Runner.Run("systemctl", "enable", "wg-quick@"+device)
		if len(output) > 0 {
			result.Logs = append(result.Logs, strings.TrimSpace(string(output)))
		}
		if err != nil {
			return result, fmt.Errorf("systemctl enable wg-quick@%s: %w", device, err)
		}
		result.Logs = append(result.Logs, "WireGuard client enabled on boot: "+device)
	}
	return result, nil
}

func Down(node config.Node, opts Options) (Result, error) {
	if node.Name == "" {
		return Result{}, fmt.Errorf("node name cannot be empty")
	}

	opts = withDefaults(opts)
	device := node.WGLocalDevice
	if device == "" {
		device = DefaultLocalDeviceName(node.Name)
	}
	target := node.WGLocalConfigPath
	if target == "" {
		target = device
	}

	result := Result{Node: node}
	if opts.Runtime != RuntimeLinux || opts.SkipSystem {
		result.Logs = append(result.Logs, fmt.Sprintf("%s runtime: skip wg-quick down", opts.Runtime))
		return result, nil
	}

	output, err := opts.Runner.Run("wg-quick", "down", target)
	if len(output) > 0 {
		result.Logs = append(result.Logs, strings.TrimSpace(string(output)))
	}
	if err != nil {
		return result, fmt.Errorf("wg-quick down %s: %w", target, err)
	}
	result.Logs = append(result.Logs, "WireGuard client stopped: "+device)
	return result, nil
}

func GetStatus(node config.Node, opts Options) (Status, error) {
	if node.Name == "" {
		return Status{}, fmt.Errorf("node name cannot be empty")
	}

	opts = withDefaults(opts)
	device := node.WGLocalDevice
	if device == "" {
		device = DefaultLocalDeviceName(node.Name)
	}
	node.WGLocalDevice = device

	status := Status{
		Node:    node,
		Runtime: opts.Runtime,
	}
	if opts.Runtime != RuntimeLinux || opts.SkipSystem {
		status.Output = fmt.Sprintf("%s runtime: wg show is not executed", opts.Runtime)
		return status, nil
	}

	output, err := opts.Runner.Run("wg", "show", device)
	status.Output = strings.TrimSpace(string(output))
	if err != nil {
		return status, fmt.Errorf("wg show %s: %w", device, err)
	}
	status.Active = true
	return status, nil
}

func validateNodeForClient(node config.Node) error {
	if node.Name == "" {
		return fmt.Errorf("node name cannot be empty")
	}
	if strings.TrimSpace(node.WGClientConfig) == "" {
		return fmt.Errorf("node %s has no WireGuard client config; deploy it first", node.Name)
	}
	return nil
}

func withDefaults(opts Options) Options {
	if opts.Runtime == "" {
		opts.Runtime = RuntimeOS(runtime.GOOS)
	}
	if opts.ConfigDir == "" {
		opts.ConfigDir = DefaultConfigDir()
	}
	if opts.Runner == nil {
		opts.Runner = execRunner{}
	}
	return opts
}

func DefaultLocalDeviceName(name string) string {
	safe := wireguard.SafeDeviceName(name)
	safe = strings.TrimPrefix(safe, "wp")
	out := "wpc" + safe
	if out == "wpc" {
		out = "wpc-node"
	}
	if len(out) > 15 {
		out = strings.TrimRight(out[:15], "-")
	}
	if out == "" || out == "wpc" {
		return "wpc-node"
	}
	return out
}

func DefaultConfigDir() string {
	if runtime.GOOS == "windows" {
		base := os.Getenv("ProgramData")
		if base == "" {
			base = "."
		}
		return filepath.Join(base, "warppool", "wireguard")
	}
	return "/etc/wireguard"
}
