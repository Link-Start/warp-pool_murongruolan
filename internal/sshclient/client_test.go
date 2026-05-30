package sshclient

import "testing"

func TestShellQuote(t *testing.T) {
	got := shellQuote("a'b")
	want := `'a'"'"'b'`
	if got != want {
		t.Fatalf("expected %s, got %s", want, got)
	}
}
