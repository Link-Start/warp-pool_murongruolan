package singbox

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

type CommandRunner interface {
	Run(name string, args ...string) ([]byte, error)
	StartBackground(name string, args ...string) (int, error)
}

type execRunner struct{}

func (execRunner) Run(name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	return cmd.CombinedOutput()
}

func (execRunner) StartBackground(name string, args ...string) (int, error) {
	cmd := exec.Command(name, args...)
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	pid := cmd.Process.Pid
	go func() {
		_ = cmd.Wait()
	}()
	return pid, nil
}

type ManagerOptions struct {
	ConfigPath string
	PIDPath    string
	Binary     string
	BundleDir  string
	Runner     CommandRunner
	Runtime    string
}

func Run(data []byte, opts ManagerOptions) error {
	opts = managerDefaults(opts)
	if err := WriteConfig(opts.ConfigPath, data); err != nil {
		return fmt.Errorf("write sing-box config: %w", err)
	}
	cmd := exec.Command(opts.Binary, "run", "-c", opts.ConfigPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("run sing-box using %q: %w", opts.Binary, err)
	}
	if err := os.MkdirAll(filepath.Dir(opts.PIDPath), 0o755); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return err
	}
	if err := os.WriteFile(opts.PIDPath, []byte(fmt.Sprintf("%d\n", cmd.Process.Pid)), 0o600); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return fmt.Errorf("write pid file: %w", err)
	}
	defer os.Remove(opts.PIDPath)
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("run sing-box using %q: %w", opts.Binary, err)
	}
	return nil
}

type StartResult struct {
	ConfigPath string
	PIDPath    string
	Logs       []string
}

type StatusResult struct {
	Running bool
	PID     int
	Message string
}

func WriteConfig(path string, data []byte) error {
	if path == "" {
		path = DefaultConfigPath()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func Start(data []byte, opts ManagerOptions) (StartResult, error) {
	opts = managerDefaults(opts)
	if err := CheckInboundPorts(data); err != nil {
		return StartResult{}, err
	}
	if err := WriteConfig(opts.ConfigPath, data); err != nil {
		return StartResult{}, fmt.Errorf("write sing-box config: %w", err)
	}
	if status, _ := Status(opts); status.Running {
		return StartResult{}, fmt.Errorf("sing-box already running with pid %d", status.PID)
	}

	pid, err := opts.Runner.StartBackground(opts.Binary, "run", "-c", opts.ConfigPath)
	if err != nil {
		return StartResult{}, fmt.Errorf("start sing-box using %q: %w; install sing-box to /usr/local/lib/warppool/bin, put it in bundled bin/ directory, or pass --singbox-bin", opts.Binary, err)
	}
	if err := os.MkdirAll(filepath.Dir(opts.PIDPath), 0o755); err != nil {
		return StartResult{}, err
	}
	if err := os.WriteFile(opts.PIDPath, []byte(fmt.Sprintf("%d\n", pid)), 0o600); err != nil {
		return StartResult{}, fmt.Errorf("write pid file: %w", err)
	}

	result := StartResult{
		ConfigPath: opts.ConfigPath,
		PIDPath:    opts.PIDPath,
		Logs: []string{
			"wrote sing-box config: " + opts.ConfigPath,
			"started sing-box with config: " + opts.ConfigPath,
		},
	}
	return result, nil
}

func CheckInboundPorts(data []byte) error {
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("parse sing-box config for port check: %w", err)
	}
	for _, inbound := range cfg.Inbounds {
		ln, err := net.Listen("tcp", net.JoinHostPort(inbound.Listen, strconv.Itoa(inbound.ListenPort)))
		if err != nil {
			return fmt.Errorf("local proxy port is not available for inbound %s: %s:%d: %w", inbound.Tag, inbound.Listen, inbound.ListenPort, err)
		}
		_ = ln.Close()
	}
	return nil
}

func Stop(opts ManagerOptions) (StatusResult, error) {
	opts = managerDefaults(opts)
	status, err := Status(opts)
	if err != nil {
		return status, err
	}
	if !status.Running {
		return status, nil
	}

	if opts.Runtime == "windows" {
		if _, err := opts.Runner.Run("taskkill", "/PID", strconv.Itoa(status.PID), "/T", "/F"); err != nil {
			return status, fmt.Errorf("stop sing-box pid %d: %w", status.PID, err)
		}
	} else {
		if _, err := opts.Runner.Run("kill", strconv.Itoa(status.PID)); err != nil {
			return status, fmt.Errorf("stop sing-box pid %d: %w", status.PID, err)
		}
	}
	_ = os.Remove(opts.PIDPath)
	status.Running = false
	status.Message = "stopped sing-box"
	return status, nil
}

func Status(opts ManagerOptions) (StatusResult, error) {
	opts = managerDefaults(opts)
	data, err := os.ReadFile(opts.PIDPath)
	if err != nil {
		if os.IsNotExist(err) {
			return StatusResult{Message: "sing-box is not running"}, nil
		}
		return StatusResult{}, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return StatusResult{}, fmt.Errorf("invalid pid file %s: %w", opts.PIDPath, err)
	}

	if processRunning(pid, opts.Runtime, opts.Runner) {
		return StatusResult{Running: true, PID: pid, Message: fmt.Sprintf("sing-box running with pid %d", pid)}, nil
	}
	return StatusResult{PID: pid, Message: fmt.Sprintf("stale pid file: %d", pid)}, nil
}

func DefaultConfigPath() string {
	return filepath.Join(defaultStateDir(), "sing-box.json")
}

func DefaultStateDir() string {
	return defaultStateDir()
}

func DefaultPIDPath() string {
	return filepath.Join(defaultStateDir(), "sing-box.pid")
}

func managerDefaults(opts ManagerOptions) ManagerOptions {
	if opts.ConfigPath == "" {
		opts.ConfigPath = DefaultConfigPath()
	}
	if opts.PIDPath == "" {
		opts.PIDPath = DefaultPIDPath()
	}
	if opts.Runner == nil {
		opts.Runner = execRunner{}
	}
	if opts.Runtime == "" {
		opts.Runtime = runtime.GOOS
	}
	if opts.Binary == "" {
		opts.Binary = ResolveBinary(opts.BundleDir, opts.Runtime)
	}
	return opts
}

func ResolveBinary(bundleDir string, runtimeOS string) string {
	name := "sing-box"
	if runtimeOS == "windows" {
		name += ".exe"
	}

	if bundleDir != "" {
		candidate := filepath.Join(bundleDir, name)
		if fileExists(candidate) {
			return candidate
		}
	}

	if exe, err := os.Executable(); err == nil {
		base := filepath.Dir(exe)
		for _, candidate := range []string{
			filepath.Join(base, "bin", name),
			filepath.Join(base, name),
		} {
			if fileExists(candidate) {
				return candidate
			}
		}
	}

	for _, dir := range systemBinaryDirs(runtimeOS) {
		candidate := filepath.Join(dir, name)
		if fileExists(candidate) {
			return candidate
		}
	}

	return "sing-box"
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func systemBinaryDirs(runtimeOS string) []string {
	switch runtimeOS {
	case "windows":
		base := os.Getenv("ProgramFiles")
		if base == "" {
			return nil
		}
		return []string{filepath.Join(base, "WarpPool", "bin")}
	case "linux":
		return []string{"/usr/local/lib/warppool/bin"}
	default:
		return nil
	}
}

func defaultStateDir() string {
	if runtime.GOOS == "windows" {
		base := os.Getenv("ProgramData")
		if base == "" {
			base = "."
		}
		return filepath.Join(base, "warppool")
	}
	return "/var/lib/warppool"
}

func processRunning(pid int, runtimeOS string, runner CommandRunner) bool {
	if pid <= 0 {
		return false
	}
	if runtimeOS == "windows" {
		out, err := runner.Run("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid))
		return err == nil && strings.Contains(string(out), strconv.Itoa(pid))
	}
	_, err := runner.Run("kill", "-0", strconv.Itoa(pid))
	return err == nil
}
