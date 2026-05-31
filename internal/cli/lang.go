package cli

import "github.com/murongruolan/warp-pool/internal/config"

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
