package cli

import (
	"strings"
	"testing"
)

func TestRenderListenService(t *testing.T) {
	service := renderListenService("/usr/local/bin/warppool", "/etc/warppool/config.json")
	for _, want := range []string{
		"Description=WarpPool Deploy Token Listener",
		"ExecStart='/usr/local/bin/warppool' --config '/etc/warppool/config.json' listen start",
		"Restart=on-failure",
		"WantedBy=multi-user.target",
	} {
		if !strings.Contains(service, want) {
			t.Fatalf("missing %q in service:\n%s", want, service)
		}
	}
}
