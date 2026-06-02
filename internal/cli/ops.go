package cli

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/murongruolan/warp-pool/internal/config"
	"github.com/murongruolan/warp-pool/internal/singbox"
	"github.com/spf13/cobra"
)

func newVersionCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Show WarpPool version",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "version: %s\ncommit: %s\nbuilt: %s\n", version, commit, date)
			return nil
		},
	}
	return cmd
}

func newDoctorCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check local WarpPool runtime prerequisites",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(resolvedConfigPath())
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			checks := BuildDoctorChecks(cfg, resolvedConfigPath())
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "CHECK\tSTATUS\tDETAIL")
			for _, check := range checks {
				printCheck(w, check.Name, check.OK, check.Detail)
			}
			return w.Flush()
		},
	}
	return cmd
}

func newPingCommand() *cobra.Command {
	return newPingCommandWithChecks(fetchText, runPing)
}

func newPingCommandWithHTTPCheck(httpCheck func(string, string, time.Duration) (string, error)) *cobra.Command {
	return newPingCommandWithChecks(httpCheck, func(string, int, time.Duration) (string, error) {
		return "rtt min/avg/max/mdev = 1.000/1.000/1.000/0.000 ms", nil
	})
}

func newPingCommandWithChecks(httpCheck func(string, string, time.Duration) (string, error), icmpCheck func(string, int, time.Duration) (string, error)) *cobra.Command {
	var count int
	var timeout time.Duration
	var checkURL string

	cmd := &cobra.Command{
		Use:   "ping [node]",
		Short: "Check node connectivity",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(resolvedConfigPath())
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			nodes := cfg.Nodes
			if len(args) == 1 {
				node, ok := config.FindNode(cfg, args[0])
				if !ok {
					return fmt.Errorf("node not found: %s", args[0])
				}
				nodes = []config.Node{node}
			}
			if len(nodes) == 0 {
				return fmt.Errorf("no nodes configured")
			}
			language := cfgLanguage(cfg)
			checkURLs := parseCheckURLs(checkURL)

			for _, node := range nodes {
				fmt.Fprintf(cmd.OutOrStdout(), "== %s ==\n", node.Name)
				printNodePublicLatency(cmd.OutOrStdout(), language, node, count, timeout, icmpCheck)
				printDirectHTTPCheck(cmd.OutOrStdout(), language, checkURLs, timeout, httpCheck)

				if nodeUsesSystemWireGuard(node) {
					target := hostOnly(node.WGServerAddress)
					if target == "" {
						fmt.Fprintf(cmd.OutOrStdout(), "%s\n", tr(language, node.Name+": missing wg_server_address", node.Name+"：缺少 wg_server_address"))
						continue
					}
					fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n", tr(language, "check mode:", "检测模式："), fmt.Sprintf("%s (%s)", tr(language, "system WireGuard ping", "系统 WireGuard ping"), target))
					out, err := icmpCheck(target, count, timeout)
					if strings.TrimSpace(out) != "" {
						fmt.Fprintln(cmd.OutOrStdout(), strings.TrimSpace(out))
					}
					if err != nil {
						fmt.Fprintf(cmd.OutOrStdout(), "%s %v\n", tr(language, "ping failed:", "ping 检测失败："), err)
					}
					continue
				}

				proxyURL := nodeProxyURL(node)
				if proxyURL == "" {
					fmt.Fprintln(cmd.OutOrStdout(), tr(language, "proxy check skipped: missing local proxy address", "代理检测已跳过：缺少本地代理地址"))
					continue
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n", tr(language, "check mode:", "检测模式："), fmt.Sprintf("%s (%s)", tr(language, "sing-box embedded WireGuard proxy check", "sing-box 内置 WireGuard 代理检测"), proxyURL))
				body, usedURL, elapsed, err := fetchTextWithFallback(checkURLs, proxyURL, timeout, httpCheck)
				if err != nil {
					fmt.Fprintf(cmd.OutOrStdout(), "%s %v\n", tr(language, "proxy check failed:", "代理检测失败："), err)
					continue
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n", tr(language, "proxy check url:", "代理检测地址："), usedURL)
				fmt.Fprintf(cmd.OutOrStdout(), "%s %s (%s %s)\n",
					tr(language, "proxy check ok:", "代理检测通过："),
					strings.TrimSpace(body),
					tr(language, "latency:", "延迟："),
					formatDuration(elapsed),
				)
			}
			return nil
		},
	}

	cmd.Flags().IntVarP(&count, "count", "c", 3, "ping count")
	cmd.Flags().DurationVar(&timeout, "timeout", 2*time.Second, "per-packet timeout")
	cmd.Flags().StringVar(&checkURL, "url", strings.Join(defaultPingCheckURLs(), ","), "HTTP URL(s), comma-separated, used for direct/proxy connectivity checks")
	return cmd
}

func defaultPingCheckURLs() []string {
	return []string{
		"https://api.ipify.org",
		"https://icanhazip.com",
		"https://ifconfig.me/ip",
	}
}

func parseCheckURLs(raw string) []string {
	items := strings.Split(raw, ",")
	urls := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item != "" {
			urls = append(urls, item)
		}
	}
	if len(urls) == 0 {
		return defaultPingCheckURLs()
	}
	return urls
}

func printNodePublicLatency(out io.Writer, language string, node config.Node, count int, timeout time.Duration, icmpCheck func(string, int, time.Duration) (string, error)) {
	target := nodeLatencyTarget(node)
	if target == "" {
		fmt.Fprintln(out, tr(language, "node latency skipped: missing public endpoint", "节点延迟检测已跳过：缺少公网端点"))
		return
	}
	fmt.Fprintf(out, "%s %s\n", tr(language, "node latency target:", "节点延迟检测地址："), target)
	pingOutput, err := icmpCheck(target, count, timeout)
	avg := pingAverageLatency(pingOutput)
	if err != nil {
		if strings.TrimSpace(pingOutput) != "" {
			fmt.Fprintf(out, "%s %s\n", tr(language, "node latency output:", "节点延迟输出："), compactMultiline(pingOutput))
		}
		fmt.Fprintf(out, "%s %v\n", tr(language, "node latency failed:", "节点延迟检测失败："), err)
		return
	}
	if avg != "" {
		fmt.Fprintf(out, "%s %s\n", tr(language, "node latency avg:", "节点平均延迟："), avg)
		return
	}
	fmt.Fprintln(out, tr(language, "node latency ok", "节点延迟检测通过"))
}

func printDirectHTTPCheck(out io.Writer, language string, urls []string, timeout time.Duration, httpCheck func(string, string, time.Duration) (string, error)) {
	body, usedURL, elapsed, err := fetchTextWithFallback(urls, "", timeout, httpCheck)
	if err != nil {
		fmt.Fprintf(out, "%s %v\n", tr(language, "direct HTTP check failed:", "主服务器直连 HTTP 检测失败："), err)
		return
	}
	fmt.Fprintf(out, "%s %s\n", tr(language, "direct HTTP check url:", "主服务器直连 HTTP 检测地址："), usedURL)
	fmt.Fprintf(out, "%s %s (%s %s)\n",
		tr(language, "direct HTTP check ok:", "主服务器直连 HTTP 检测通过："),
		strings.TrimSpace(body),
		tr(language, "latency:", "延迟："),
		formatDuration(elapsed),
	)
}

func fetchTextWithFallback(urls []string, proxyURL string, timeout time.Duration, httpCheck func(string, string, time.Duration) (string, error)) (string, string, time.Duration, error) {
	var failures []string
	for _, rawURL := range urls {
		start := time.Now()
		body, err := httpCheck(rawURL, proxyURL, timeout)
		elapsed := time.Since(start)
		if err == nil {
			return body, rawURL, elapsed, nil
		}
		failures = append(failures, fmt.Sprintf("%s: %v", rawURL, err))
	}
	return "", "", 0, fmt.Errorf("all HTTP checks failed: %s", strings.Join(failures, "; "))
}

func nodeLatencyTarget(node config.Node) string {
	if host := endpointHost(node.Endpoint); host != "" {
		return host
	}
	if strings.TrimSpace(node.PublicIP) != "" {
		return strings.TrimSpace(node.PublicIP)
	}
	if strings.TrimSpace(node.SSHHost) != "" {
		return strings.TrimSpace(node.SSHHost)
	}
	return ""
}

func pingAverageLatency(output string) string {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		lower := strings.ToLower(line)
		if strings.Contains(lower, "min/avg") && strings.Contains(line, "=") {
			_, after, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			fields := strings.Fields(strings.TrimSpace(after))
			if len(fields) == 0 {
				continue
			}
			values := strings.Split(fields[0], "/")
			if len(values) >= 2 && strings.TrimSpace(values[1]) != "" {
				return strings.TrimSpace(values[1]) + " ms"
			}
		}
		if idx := strings.LastIndex(line, "Average = "); idx >= 0 {
			value := strings.TrimSpace(line[idx+len("Average = "):])
			if value != "" {
				return value
			}
		}
	}
	return ""
}

func formatDuration(value time.Duration) string {
	if value <= 0 {
		return "0s"
	}
	if value < time.Millisecond {
		return value.String()
	}
	return value.Round(time.Millisecond).String()
}

func newSpeedtestCommand() *cobra.Command {
	var proxy string
	var url string
	var timeout time.Duration

	cmd := &cobra.Command{
		Use:   "speedtest",
		Short: "Run a lightweight HTTP download speed test",
		RunE: func(cmd *cobra.Command, args []string) error {
			start := time.Now()
			n, err := timedDownload(url, proxy, timeout)
			elapsed := time.Since(start)
			if err != nil {
				return err
			}
			mbps := float64(n*8) / elapsed.Seconds() / 1000 / 1000
			fmt.Fprintf(cmd.OutOrStdout(), "downloaded: %d bytes\nelapsed: %s\nspeed: %.2f Mbps\n", n, elapsed.Round(time.Millisecond), mbps)
			return nil
		},
	}
	cmd.Flags().StringVar(&proxy, "proxy", "", "proxy URL, for example socks5h://127.0.0.1:10134")
	cmd.Flags().StringVar(&url, "url", "https://speed.cloudflare.com/__down?bytes=1000000", "download URL")
	cmd.Flags().DurationVar(&timeout, "timeout", 30*time.Second, "HTTP timeout")
	return cmd
}

func newShowCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "show <node>",
		Short:  "Show node details with local runtime status",
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, node, err := loadConfigAndNode(resolvedConfigPath(), args[0])
			if err != nil {
				return err
			}
			return printNodeDetails(cmd.OutOrStdout(), cfgLanguage(cfg), node, true)
		},
	}
	return cmd
}

func redactNode(node config.Node) config.Node {
	if node.WGClientPrivateKey != "" {
		node.WGClientPrivateKey = "<redacted>"
	}
	if node.WGClientConfig != "" {
		node.WGClientConfig = redactWireGuardConfig(node.WGClientConfig)
	}
	return node
}

func redactWireGuardConfig(value string) string {
	lines := strings.Split(value, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "PrivateKey") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				lines[i] = strings.TrimRight(parts[0], " ") + " = <redacted>"
			}
		}
	}
	return strings.Join(lines, "\n")
}

func newUpgradeCommand() *cobra.Command {
	var scriptPath string
	var versionArg string
	var localFile string
	var yes bool
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Upgrade WarpPool binary and bundled assets",
		RunE: func(cmd *cobra.Command, args []string) error {
			if runtime.GOOS != "linux" {
				return fmt.Errorf("upgrade is currently supported only on Linux")
			}
			if scriptPath == "" {
				scriptPath = resolveUpgradeScript()
			}
			cleanupScript := func() {}
			if localFile != "" && !cmd.Flags().Changed("script") {
				if extracted, cleanup, err := extractUpgradeScriptFromPackage(localFile); err == nil {
					scriptPath = extracted
					cleanupScript = cleanup
				}
			}
			defer cleanupScript()
			if _, err := os.Stat(scriptPath); err != nil {
				return fmt.Errorf("upgrade helper not found: %s: %w", scriptPath, err)
			}
			argv := []string{scriptPath}
			if versionArg != "" {
				argv = append(argv, "--version", versionArg)
			}
			if localFile != "" {
				argv = append(argv, "--file", localFile)
			}
			language := "en"
			if envLanguage := os.Getenv("WARPPOOL_LANGUAGE"); envLanguage != "" {
				language = cfgLanguage(config.Config{Language: envLanguage})
			} else if cfg, err := config.Load(resolvedConfigPath()); err == nil {
				language = cfgLanguage(cfg)
			}
			argv = append(argv, "--language", language)
			if yes {
				argv = append(argv, "--yes")
			}
			if dryRun {
				argv = append(argv, "--dry-run")
			}
			c := exec.Command("bash", argv...)
			c.Stdout = cmd.OutOrStdout()
			c.Stderr = cmd.ErrOrStderr()
			c.Stdin = os.Stdin
			return c.Run()
		},
	}
	cmd.Flags().StringVar(&scriptPath, "script", "", "upgrade helper script path")
	cmd.Flags().StringVar(&versionArg, "version", "", "release version: latest or vX.Y.Z")
	cmd.Flags().StringVar(&localFile, "file", "", "local release package path, for example /tmp/warppool-linux-amd64.tar.gz")
	cmd.Flags().BoolVar(&yes, "yes", false, "run without confirmation")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print upgrade actions without changing files")
	return cmd
}

func resolveUpgradeScript() string {
	if candidate := filepath.Join(resolveAssetsDir(""), "upgrade.sh"); fileExistsLocal(candidate) {
		return candidate
	}
	return filepath.Join("/usr/local/lib/warppool/assets", "upgrade.sh")
}

func extractUpgradeScriptFromPackage(packagePath string) (string, func(), error) {
	file, err := os.Open(packagePath)
	if err != nil {
		return "", func() {}, err
	}
	defer file.Close()

	gz, err := gzip.NewReader(file)
	if err != nil {
		return "", func() {}, err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", func() {}, err
		}
		name := filepath.ToSlash(header.Name)
		if !strings.HasSuffix(name, "/assets/upgrade.sh") {
			continue
		}
		dir, err := os.MkdirTemp("", "warppool-upgrade-*")
		if err != nil {
			return "", func() {}, err
		}
		cleanup := func() { _ = os.RemoveAll(dir) }
		path := filepath.Join(dir, "upgrade.sh")
		out, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o700)
		if err != nil {
			cleanup()
			return "", func() {}, err
		}
		if _, err := io.Copy(out, tr); err != nil {
			_ = out.Close()
			cleanup()
			return "", func() {}, err
		}
		if err := out.Close(); err != nil {
			cleanup()
			return "", func() {}, err
		}
		return path, cleanup, nil
	}
	return "", func() {}, fmt.Errorf("assets/upgrade.sh not found in local package")
}

func fileExistsLocal(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func printCheck(w *tabwriter.Writer, name string, ok bool, detail string) {
	status := "FAIL"
	if ok {
		status = "OK"
	}
	fmt.Fprintf(w, "%s\t%s\t%s\n", name, status, detail)
}

type DoctorCheck struct {
	Name   string
	OK     bool
	Detail string
}

func BuildDoctorChecks(cfg config.Config, cfgPath string) []DoctorCheck {
	checks := []DoctorCheck{
		{Name: "config", OK: true, Detail: cfgPath},
		{Name: "wireguard", OK: commandExists("wg"), Detail: pathOrMissing("wg")},
		{Name: "wg-quick", OK: runtime.GOOS == "windows" || commandExists("wg-quick"), Detail: pathOrMissing("wg-quick")},
	}
	sb := singbox.ResolveBinary("", runtime.GOOS)
	checks = append(checks, DoctorCheck{Name: "sing-box", OK: binaryRunnable(sb, "version"), Detail: sb})
	proxyStatus, proxyErr := singbox.Status(singbox.ManagerOptions{})
	proxyRunning := proxyErr == nil && proxyStatus.Running

	for _, node := range cfg.Nodes {
		_, err := net.Listen("tcp", net.JoinHostPort(node.BindHost, fmt.Sprintf("%d", node.LocalPort)))
		if err != nil {
			if proxyRunning {
				checks = append(checks, DoctorCheck{Name: "port " + node.Name, OK: true, Detail: fmt.Sprintf("%s:%d in use by local proxy", node.BindHost, node.LocalPort)})
				continue
			}
			checks = append(checks, DoctorCheck{Name: "port " + node.Name, OK: false, Detail: err.Error()})
			continue
		}
		checks = append(checks, DoctorCheck{Name: "port " + node.Name, OK: true, Detail: fmt.Sprintf("%s:%d", node.BindHost, node.LocalPort)})
	}

	if proxyErr != nil {
		checks = append(checks, DoctorCheck{Name: "proxy", OK: false, Detail: proxyErr.Error()})
	} else {
		checks = append(checks, DoctorCheck{Name: "proxy", OK: proxyStatus.Running, Detail: proxyStatus.Message})
	}
	return checks
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func pathOrMissing(name string) string {
	path, err := exec.LookPath(name)
	if err != nil {
		return "not found"
	}
	return path
}

func binaryRunnable(name string, args ...string) bool {
	cmd := exec.Command(name, args...)
	return cmd.Run() == nil
}

func hostOnly(cidr string) string {
	if cidr == "" {
		return ""
	}
	return strings.Split(cidr, "/")[0]
}

func runPing(target string, count int, timeout time.Duration) (string, error) {
	if count < 1 {
		count = 1
	}
	if runtime.GOOS == "windows" {
		out, err := exec.Command("ping", "-n", fmt.Sprintf("%d", count), "-w", fmt.Sprintf("%d", timeout.Milliseconds()), target).CombinedOutput()
		return string(out), err
	}
	seconds := int(timeout.Seconds())
	if seconds < 1 {
		seconds = 1
	}
	out, err := exec.Command("ping", "-c", fmt.Sprintf("%d", count), "-W", fmt.Sprintf("%d", seconds), target).CombinedOutput()
	return string(out), err
}

func nodeProxyURL(node config.Node) string {
	if strings.TrimSpace(node.BindHost) == "" || node.LocalPort == 0 {
		return ""
	}
	host := node.BindHost
	if host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	scheme := "socks5"
	if node.Proxy == config.ProxyHTTP {
		scheme = "http"
	}
	return fmt.Sprintf("%s://%s", scheme, net.JoinHostPort(host, fmt.Sprintf("%d", node.LocalPort)))
}

func fetchText(rawURL string, proxyURL string, timeout time.Duration) (string, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if proxyURL != "" {
		parsedProxy, err := url.Parse(proxyURL)
		if err != nil {
			return "", fmt.Errorf("parse proxy URL: %w", err)
		}
		transport.Proxy = http.ProxyURL(parsedProxy)
	}
	client := &http.Client{Transport: transport, Timeout: timeout}
	resp, err := client.Get(rawURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("HTTP status: %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func timedDownload(rawURL string, proxyURL string, timeout time.Duration) (int64, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if proxyURL != "" {
		parsedProxy, err := url.Parse(proxyURL)
		if err != nil {
			return 0, fmt.Errorf("parse proxy URL: %w", err)
		}
		transport.Proxy = func(*http.Request) (*url.URL, error) {
			return parsedProxy, nil
		}
	}
	client := &http.Client{Transport: transport, Timeout: timeout}
	resp, err := client.Get(rawURL)
	if err != nil {
		return 0, fmt.Errorf("download speedtest URL: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, fmt.Errorf("speedtest HTTP status: %s", resp.Status)
	}
	n, err := io.Copy(io.Discard, resp.Body)
	if err != nil {
		return 0, fmt.Errorf("read speedtest body: %w", err)
	}
	return n, nil
}
