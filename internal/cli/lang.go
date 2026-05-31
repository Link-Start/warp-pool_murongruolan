package cli

import (
	"strings"

	"github.com/murongruolan/warp-pool/internal/config"
)

func cfgLanguage(cfg config.Config) string {
	if config.NormalizeLanguage(cfg.Language) == "zh" {
		return "zh"
	}
	return "en"
}

func tr(language string, en string, zh string) string {
	if config.NormalizeLanguage(language) == "zh" {
		return zh
	}
	return en
}

func cloudSecurityGroupReminder(language string, ports ...string) string {
	filtered := make([]string, 0, len(ports))
	seen := map[string]bool{}
	for _, port := range ports {
		if port == "" || seen[port] {
			continue
		}
		seen[port] = true
		filtered = append(filtered, port)
	}
	if len(filtered) == 0 {
		return tr(language,
			"Reminder: if this server is behind a cloud provider security group, open the required inbound ports there. Local firewall rules have been handled when supported.",
			"提醒：如果服务器位于云服务商安全组后，请在服务商控制台放行需要入站的端口；本机防火墙已在支持时自动处理。",
		)
	}
	joined := strings.Join(filtered, ", ")
	return tr(language,
		"Reminder: if this server is behind a cloud provider security group, open these inbound ports there: "+joined+". Local firewall rules have been handled when supported.",
		"提醒：如果服务器位于云服务商安全组后，请在服务商控制台放行这些入站端口："+joined+"。本机防火墙已在支持时自动处理。",
	)
}
