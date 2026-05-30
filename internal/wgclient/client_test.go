package wgclient

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/murongruolan/warp-pool/internal/config"
)

type fakeRunner struct {
	calls []string
	out   string
	err   error
}

func (r *fakeRunner) Run(name string, args ...string) ([]byte, error) {
	r.calls = append(r.calls, name+" "+strings.Join(args, " "))
	return []byte(r.out), r.err
}

func TestPrepareUpWritesConfigAndSkipsSystemOnWindows(t *testing.T) {
	dir := t.TempDir()
	result, err := PrepareUp(testNode(), Options{
		Runtime:   RuntimeWindows,
		ConfigDir: dir,
	})
	if err != nil {
		t.Fatalf("prepare up: %v", err)
	}

	if result.Node.WGLocalDevice != "wpcnat1" {
		t.Fatalf("unexpected device: %s", result.Node.WGLocalDevice)
	}
	data, err := os.ReadFile(filepath.Join(dir, "wpcnat1.conf"))
	if err != nil {
		t.Fatalf("read generated config: %v", err)
	}
	if string(data) != testNode().WGClientConfig {
		t.Fatalf("unexpected config:\n%s", data)
	}
	if !containsLog(result.Logs, "skip wg-quick up") {
		t.Fatalf("expected skip log, got %#v", result.Logs)
	}
}

func TestPrepareUpRunsWGQuickOnLinux(t *testing.T) {
	dir := t.TempDir()
	runner := &fakeRunner{out: "started"}

	result, err := PrepareUp(testNode(), Options{
		Runtime:   RuntimeLinux,
		ConfigDir: dir,
		Runner:    runner,
	})
	if err != nil {
		t.Fatalf("prepare up: %v", err)
	}

	if len(runner.calls) != 1 {
		t.Fatalf("expected one command, got %#v", runner.calls)
	}
	wantPath := filepath.Join(dir, "wpcnat1.conf")
	if runner.calls[0] != "wg-quick up "+wantPath {
		t.Fatalf("unexpected command: %s", runner.calls[0])
	}
	if !containsLog(result.Logs, "WireGuard client started: wpcnat1") {
		t.Fatalf("expected started log, got %#v", result.Logs)
	}
}

func TestDownRunsWGQuickOnLinux(t *testing.T) {
	runner := &fakeRunner{}

	_, err := Down(testNode(), Options{
		Runtime: RuntimeLinux,
		Runner:  runner,
	})
	if err != nil {
		t.Fatalf("down: %v", err)
	}
	if len(runner.calls) != 1 || runner.calls[0] != "wg-quick down wpcnat1" {
		t.Fatalf("unexpected calls: %#v", runner.calls)
	}
}

func TestStatusRunsWGShowOnLinux(t *testing.T) {
	runner := &fakeRunner{out: "interface: wpcnat1\n  latest handshake: now"}

	status, err := GetStatus(testNode(), Options{
		Runtime: RuntimeLinux,
		Runner:  runner,
	})
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !status.Active {
		t.Fatal("expected active status")
	}
	if !strings.Contains(status.Output, "latest handshake") {
		t.Fatalf("unexpected output: %s", status.Output)
	}
}

func TestDefaultLocalDeviceNameDiffersFromRemoteDeviceName(t *testing.T) {
	if got := DefaultLocalDeviceName("nat-direct-8"); got != "wpcnat-direct-8" {
		t.Fatalf("unexpected local device name: %s", got)
	}
}

func testNode() config.Node {
	return config.Node{
		Name:           "nat1",
		WGClientConfig: "[Interface]\nPrivateKey = test\nAddress = 10.200.0.2/30\n",
	}
}

func containsLog(logs []string, want string) bool {
	for _, item := range logs {
		if strings.Contains(item, want) {
			return true
		}
	}
	return false
}
