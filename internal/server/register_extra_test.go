package server

import "testing"

func TestServerSafeFilePart(t *testing.T) {
	if got := safeFilePart("美国NAT01"); got != "nat01" {
		t.Fatalf("unexpected safe file part: %s", got)
	}
	if got := safeFilePart("美国节点"); got != "node" {
		t.Fatalf("unexpected fallback safe file part: %s", got)
	}
}

func TestSpawnProxyAutostartSkipsWhenExecDisabled(t *testing.T) {
	oldExec := execCommand
	execCommand = nil
	t.Cleanup(func() { execCommand = oldExec })

	if err := spawnProxyAutostart("/tmp/config.json", "nat01"); err != nil {
		t.Fatalf("expected disabled autostart to be nil, got %v", err)
	}
}
