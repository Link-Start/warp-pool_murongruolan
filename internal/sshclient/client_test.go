package sshclient

import (
	"os"
	"path/filepath"
	"testing"
)

func TestShellQuote(t *testing.T) {
	got := shellQuote("a'b")
	want := `'a'"'"'b'`
	if got != want {
		t.Fatalf("expected %s, got %s", want, got)
	}
}

func TestHostKeyCallbackRequiresKnownHostsByDefault(t *testing.T) {
	_, err := hostKeyCallback(Config{KnownHostsPath: filepath.Join(t.TempDir(), "missing")})
	if err == nil {
		t.Fatal("expected missing known_hosts error")
	}
}

func TestHostKeyCallbackAllowsExplicitInsecureMode(t *testing.T) {
	callback, err := hostKeyCallback(Config{InsecureIgnoreHostKey: true})
	if err != nil {
		t.Fatalf("expected insecure callback: %v", err)
	}
	if callback == nil {
		t.Fatal("expected callback")
	}
}

func TestHostKeyCallbackParsesKnownHostsFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known_hosts")
	if err := os.WriteFile(path, []byte{}, 0o600); err != nil {
		t.Fatal(err)
	}
	callback, err := hostKeyCallback(Config{KnownHostsPath: path})
	if err != nil {
		t.Fatalf("expected callback: %v", err)
	}
	if callback == nil {
		t.Fatal("expected callback")
	}
}
