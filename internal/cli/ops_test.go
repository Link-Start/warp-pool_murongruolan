package cli

import (
	"testing"

	"github.com/murongruolan/warp-pool/internal/config"
)

func TestHostOnly(t *testing.T) {
	if got := hostOnly("10.200.0.1/30"); got != "10.200.0.1" {
		t.Fatalf("unexpected host: %s", got)
	}
}

func TestBuildDoctorChecksIncludesNodePort(t *testing.T) {
	cfg := config.Default()
	cfg.Nodes = append(cfg.Nodes, config.Node{
		Name:      "ops",
		ExitMode:  config.ExitModeDirect,
		Proxy:     config.ProxyMixed,
		BindHost:  "127.0.0.1",
		LocalPort: 1,
	})

	checks := BuildDoctorChecks(cfg, "config.json")
	if !hasDoctorCheck(checks, "config") || !hasDoctorCheck(checks, "port ops") {
		t.Fatalf("missing expected checks: %#v", checks)
	}
}

func TestValidateLocalProxyPortRejectsReservedPort(t *testing.T) {
	cfg := config.Default()
	cfg.Nodes = append(cfg.Nodes, config.Node{Name: "nat1", BindHost: "127.0.0.1", LocalPort: 10133})
	if err := validateLocalProxyPort(cfg, "127.0.0.1", 10133); err == nil {
		t.Fatal("expected reserved port error")
	}
}

func hasDoctorCheck(checks []DoctorCheck, name string) bool {
	for _, check := range checks {
		if check.Name == name {
			return true
		}
	}
	return false
}
