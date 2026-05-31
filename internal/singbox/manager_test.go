package singbox

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeRunner struct {
	starts []string
	runs   []string
	pid    int
}

func (r *fakeRunner) Run(name string, args ...string) ([]byte, error) {
	call := name + " " + strings.Join(args, " ")
	r.runs = append(r.runs, call)
	if name == "kill" && len(args) >= 2 && args[0] == "-0" {
		return []byte{}, nil
	}
	return []byte{}, nil
}

func (r *fakeRunner) StartBackground(name string, args ...string) (int, error) {
	r.starts = append(r.starts, name+" "+strings.Join(args, " "))
	if r.pid == 0 {
		r.pid = 12345
	}
	return r.pid, nil
}

func TestStartWritesConfigAndPID(t *testing.T) {
	dir := t.TempDir()
	runner := &fakeRunner{pid: 23456}
	configPath := filepath.Join(dir, "sing-box.json")
	pidPath := filepath.Join(dir, "sing-box.pid")

	result, err := Start([]byte(`{"inbounds":[{"type":"mixed","tag":"in-test","listen":"127.0.0.1","listen_port":0}]}`+"\n"), ManagerOptions{
		ConfigPath: configPath,
		PIDPath:    pidPath,
		Binary:     "sing-box-test",
		Runner:     runner,
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if result.ConfigPath != configPath || result.PIDPath != pidPath {
		t.Fatalf("unexpected result: %#v", result)
	}
	if len(runner.starts) != 1 || runner.starts[0] != "sing-box-test run -c "+configPath {
		t.Fatalf("unexpected starts: %#v", runner.starts)
	}
	data, err := os.ReadFile(pidPath)
	if err != nil {
		t.Fatalf("read pid file: %v", err)
	}
	if strings.TrimSpace(string(data)) != "23456" {
		t.Fatalf("unexpected pid file: %s", data)
	}
}

func TestStartRejectsBusyPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	_, portText, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatalf("split addr: %v", err)
	}
	data := []byte(fmt.Sprintf(`{"inbounds":[{"type":"mixed","tag":"in-test","listen":"127.0.0.1","listen_port":%s}]}`, portText))
	_, err = Start(data, ManagerOptions{
		ConfigPath: filepath.Join(t.TempDir(), "sing-box.json"),
		PIDPath:    filepath.Join(t.TempDir(), "sing-box.pid"),
		Runner:     &fakeRunner{},
	})
	if err == nil || !strings.Contains(err.Error(), "local proxy port is not available") {
		t.Fatalf("expected busy port error, got %v", err)
	}
}

func TestCheckInboundPortsExceptIgnoresRestartingInbound(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	_, portText, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatalf("split addr: %v", err)
	}
	data := []byte(fmt.Sprintf(`{"inbounds":[{"type":"mixed","tag":"in-us1","listen":"127.0.0.1","listen_port":%s}]}`, portText))
	if err := CheckInboundPortsExcept(data, map[string]bool{"in-us1": true}); err != nil {
		t.Fatalf("expected ignored busy port to pass, got %v", err)
	}
	if err := CheckInboundPortsExcept(data, map[string]bool{"in-other": true}); err == nil {
		t.Fatal("expected non-ignored busy port to fail")
	}
}

func TestStatusReadsPID(t *testing.T) {
	dir := t.TempDir()
	pidPath := filepath.Join(dir, "sing-box.pid")
	if err := os.WriteFile(pidPath, []byte("34567\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{}

	status, err := Status(ManagerOptions{PIDPath: pidPath, Runtime: "linux", Runner: runner})
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !status.Running || status.PID != 34567 {
		t.Fatalf("unexpected status: %#v", status)
	}
}

func TestStopKillsPID(t *testing.T) {
	dir := t.TempDir()
	pidPath := filepath.Join(dir, "sing-box.pid")
	if err := os.WriteFile(pidPath, []byte("45678\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{}

	status, err := Stop(ManagerOptions{PIDPath: pidPath, Runtime: "linux", Runner: runner})
	if err != nil {
		t.Fatalf("stop: %v", err)
	}
	if status.Running {
		t.Fatalf("expected stopped status: %#v", status)
	}
	joined := fmt.Sprint(runner.runs)
	if !strings.Contains(joined, "kill -0 45678") || !strings.Contains(joined, "kill 45678") {
		t.Fatalf("unexpected runs: %#v", runner.runs)
	}
}

func TestResolveBinaryPrefersBundleDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sing-box")
	if err := os.WriteFile(path, []byte("fake"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := ResolveBinary(dir, "linux"); got != path {
		t.Fatalf("expected bundle binary %s, got %s", path, got)
	}
}

func TestResolveBinaryFallsBackToPath(t *testing.T) {
	if got := ResolveBinary(filepath.Join(t.TempDir(), "missing"), "darwin"); got != "sing-box" {
		t.Fatalf("expected PATH fallback, got %s", got)
	}
}

func TestSystemBinaryDirsIncludesWarpPoolInstallDir(t *testing.T) {
	dirs := systemBinaryDirs("linux")
	if len(dirs) != 1 || dirs[0] != "/usr/local/lib/warppool/bin" {
		t.Fatalf("unexpected linux system dirs: %#v", dirs)
	}
}
