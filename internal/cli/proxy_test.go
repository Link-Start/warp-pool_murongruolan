package cli

import (
	"strings"
	"testing"
)

func TestRenderProxyService(t *testing.T) {
	service := renderProxyService("/usr/local/bin/warppool", "/etc/warppool/config.json", "/usr/local/lib/warppool/bin/sing-box")
	for _, want := range []string{
		"Description=WarpPool Local Proxy",
		"Environment='WARPOOL_SINGBOX_BIN=/usr/local/lib/warppool/bin/sing-box'",
		"ExecStart='/usr/local/bin/warppool' --config '/etc/warppool/config.json' proxy run",
		"Restart=on-failure",
		"WantedBy=multi-user.target",
	} {
		if !strings.Contains(service, want) {
			t.Fatalf("missing %q in service:\n%s", want, service)
		}
	}
}
