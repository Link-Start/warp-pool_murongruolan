package exporter

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/murongruolan/warp-pool/internal/config"
)

var unsafeProxyNameChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

type ClashOptions struct {
	ProxyType string
}

func Clash(cfg config.Config, opts ClashOptions) (string, error) {
	if opts.ProxyType != "" {
		if _, err := clashProxyType(config.ProxyMixed, opts.ProxyType); err != nil {
			return "", err
		}
	}

	var b strings.Builder
	b.WriteString("proxies:\n")

	for _, node := range cfg.Nodes {
		proxyType, err := clashProxyType(node.Proxy, opts.ProxyType)
		if err != nil {
			return "", err
		}

		writeClashProxy(&b, clashName(node, ""), proxyType, node.BindHost, node.LocalPort)
		if node.ExitMode == config.ExitModeDual {
			writeClashProxy(&b, clashName(node, "WARP"), proxyType, node.BindHost, node.WarpLocalPort)
		}
	}

	return b.String(), nil
}

func clashProxyType(nodeProxy string, override string) (string, error) {
	if override != "" {
		switch override {
		case config.ProxySocks5, config.ProxyHTTP:
			return override, nil
		default:
			return "", fmt.Errorf("unsupported clash proxy type %q, expected socks5 or http", override)
		}
	}

	switch nodeProxy {
	case config.ProxySocks5:
		return config.ProxySocks5, nil
	case config.ProxyHTTP:
		return config.ProxyHTTP, nil
	case config.ProxyMixed:
		return config.ProxySocks5, nil
	default:
		return "", fmt.Errorf("unsupported node proxy protocol %q", nodeProxy)
	}
}

func writeClashProxy(b *strings.Builder, name string, proxyType string, host string, port int) {
	fmt.Fprintf(b, "\n- name: %s\n", quoteYAML(name))
	fmt.Fprintf(b, "  type: %s\n", proxyType)
	fmt.Fprintf(b, "  server: %s\n", host)
	fmt.Fprintf(b, "  port: %d\n", port)
}

func clashName(node config.Node, variant string) string {
	prefix := "WarpPool"
	if node.ExitMode == config.ExitModeWarp {
		prefix = "WARP"
	}
	if variant != "" {
		prefix = variant
	}

	parts := []string{prefix}
	if node.Country != "" {
		parts = append(parts, node.Country)
	}
	parts = append(parts, node.Name)
	if node.ExitMode == config.ExitModeDual && variant == "" {
		parts = append(parts, "direct")
	}

	name := strings.Join(parts, "-")
	name = unsafeProxyNameChars.ReplaceAllString(name, "-")
	name = strings.Trim(name, "-")
	if name == "" {
		return "WarpPool-Node"
	}
	return name
}

func quoteYAML(value string) string {
	escaped := strings.ReplaceAll(value, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return `"` + escaped + `"`
}
