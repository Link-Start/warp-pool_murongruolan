package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/murongruolan/warp-pool/internal/config"
	"golang.org/x/term"
)

type progressReporter struct {
	mu          sync.Mutex
	out         io.Writer
	language    string
	interactive bool
	started     time.Time
	lines       int
	currentKey  string
	currentArgs []string
	stop        chan struct{}
	finished    bool
}

func newProgressReporter(out io.Writer, language string) *progressReporter {
	interactive := false
	if file, ok := out.(*os.File); ok {
		interactive = term.IsTerminal(int(file.Fd()))
	}
	reporter := &progressReporter{
		out:         out,
		language:    config.NormalizeLanguage(language),
		interactive: interactive,
		started:     time.Now(),
	}
	if reporter.interactive {
		reporter.stop = make(chan struct{})
		go reporter.refreshLoop(reporter.stop)
	}
	return reporter
}

func (p *progressReporter) Update(key string, args ...string) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.finished {
		return
	}
	msg := deployProgressText(p.language, key, args...)
	if msg == "" {
		return
	}
	if !p.interactive {
		fmt.Fprintln(p.out, msg)
		return
	}
	p.currentKey = key
	p.currentArgs = append([]string(nil), args...)
	p.renderLocked(key, args...)
}

func (p *progressReporter) Success(message string) {
	if p == nil {
		return
	}
	p.mu.Lock()
	if p.finished {
		p.mu.Unlock()
		return
	}
	p.finished = true
	p.stopRefreshLocked()
	p.clearLocked()
	p.mu.Unlock()
	if strings.TrimSpace(message) != "" {
		fmt.Fprintln(p.out, message)
	}
}

func (p *progressReporter) Fail(message string, logs []string, err error) {
	if p == nil {
		return
	}
	p.mu.Lock()
	if p.finished {
		p.mu.Unlock()
		return
	}
	p.finished = true
	p.stopRefreshLocked()
	p.clearLocked()
	p.mu.Unlock()
	if strings.TrimSpace(message) != "" {
		fmt.Fprintln(p.out, message)
	}
	if err != nil {
		fmt.Fprintf(p.out, "%s\n", err)
	}
	printDeployLogs(p.out, logs)
}

func (p *progressReporter) refreshLoop(stop <-chan struct{}) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			p.mu.Lock()
			if p.finished {
				p.mu.Unlock()
				return
			}
			if p.currentKey != "" {
				p.renderLocked(p.currentKey, p.currentArgs...)
			}
			p.mu.Unlock()
		case <-stop:
			return
		}
	}
}

func (p *progressReporter) stopRefreshLocked() {
	if p.stop != nil {
		close(p.stop)
		p.stop = nil
	}
}

func (p *progressReporter) renderLocked(key string, args ...string) {
	msg := deployProgressText(p.language, key, args...)
	if msg == "" {
		return
	}
	p.clearLocked()
	block := p.block(key, msg)
	fmt.Fprint(p.out, block)
	p.lines = strings.Count(block, "\n") + 1
}

func (p *progressReporter) clearLocked() {
	if !p.interactive || p.lines == 0 {
		return
	}
	fmt.Fprint(p.out, "\r\033[2K")
	for i := 1; i < p.lines; i++ {
		fmt.Fprint(p.out, "\033[F\r\033[2K")
	}
	p.lines = 0
}

func (p *progressReporter) block(key string, message string) string {
	elapsed := int(time.Since(p.started).Seconds())
	stage, total := deployProgressStage(key)
	bar := progressBar(stage, total)
	if config.NormalizeLanguage(p.language) == "zh" {
		return fmt.Sprintf("[WarpPool] %s %d/%d 部署节点 | 已用 %ds\n当前步骤：%s", bar, stage, total, elapsed, message)
	}
	return fmt.Sprintf("[WarpPool] %s %d/%d deploy node | %ds elapsed\nCurrent step: %s", bar, stage, total, elapsed, message)
}

func deployProgressStage(key string) (int, int) {
	total := 12
	switch key {
	case "checking_local_port":
		return 1, total
	case "ssh_connect":
		return 2, total
	case "ssh_connected", "prepare_remote_dir":
		return 3, total
	case "upload_asset":
		return 4, total
	case "detect_privilege", "using_sudo":
		return 5, total
	case "install_node":
		return 6, total
	case "detect_egress":
		return 7, total
	case "generate_wireguard":
		return 8, total
	case "configure_wireguard":
		return 9, total
	case "configure_warp":
		return 10, total
	case "save_config":
		return 11, total
	case "start_proxy":
		return 12, total
	default:
		return 1, total
	}
}

func progressBar(stage int, total int) string {
	if total <= 0 {
		total = 1
	}
	width := 12
	filled := stage * width / total
	if filled < 1 {
		filled = 1
	}
	if filled > width {
		filled = width
	}
	return "[" + strings.Repeat("=", filled) + strings.Repeat("-", width-filled) + "]"
}

func deployProgressText(language string, key string, args ...string) string {
	arg := ""
	if len(args) > 0 {
		arg = args[0]
	}
	switch key {
	case "checking_local_port":
		return tr(language, "Checking local proxy port...", "正在检查本地代理端口...")
	case "ssh_connect":
		return tr(language, "Connecting SSH "+arg+"...", "正在连接 SSH "+arg+"...")
	case "ssh_connected":
		return tr(language, "SSH connected.", "SSH 已连接。")
	case "prepare_remote_dir":
		return tr(language, "Preparing remote installer directory...", "正在准备节点服务器安装目录...")
	case "upload_asset":
		return tr(language, "Uploading installer script "+arg+"...", "正在上传安装脚本 "+arg+"...")
	case "detect_privilege":
		return tr(language, "Checking remote privileges...", "正在检查节点服务器权限...")
	case "using_sudo":
		return tr(language, "Remote user is not root; using sudo...", "节点用户不是 root，正在使用 sudo...")
	case "install_node":
		return tr(language, "Installing node dependencies on remote server...", "正在节点服务器安装依赖...")
	case "detect_egress":
		return tr(language, "Detecting remote egress interface...", "正在检测节点服务器出口网卡...")
	case "generate_wireguard":
		return tr(language, "Generating WireGuard configuration...", "正在生成 WireGuard 配置...")
	case "configure_wireguard":
		return tr(language, "Writing and starting remote WireGuard...", "正在写入并启动节点 WireGuard...")
	case "configure_warp":
		return tr(language, "Configuring remote WARP forwarding...", "正在配置节点 WARP 转发...")
	case "save_config":
		return tr(language, "Saving local configuration...", "正在保存本地配置...")
	case "start_proxy":
		return tr(language, "Starting local proxy service...", "正在启动本地代理服务...")
	default:
		return ""
	}
}

func printDeployLogs(out io.Writer, logs []string) {
	for _, item := range logs {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		fmt.Fprintln(out, item)
	}
}

func printDeploySummary(out io.Writer, language string, node config.Node, proxyStarted bool) {
	if config.NormalizeLanguage(language) == "zh" {
		fmt.Fprintf(out, "节点：%s\n", node.Name)
		fmt.Fprintf(out, "- 出口模式：%s\n", node.ExitMode)
		fmt.Fprintf(out, "- 本地代理：%s://%s:%d\n", node.Proxy, node.BindHost, node.LocalPort)
		if node.Endpoint != "" {
			fmt.Fprintf(out, "- WireGuard 公网端点：%s\n", node.Endpoint)
		}
		if node.WGDevice != "" {
			fmt.Fprintf(out, "- 节点 WireGuard 设备：%s\n", node.WGDevice)
		}
		if proxyStarted {
			fmt.Fprintln(out, "- 本地代理服务：已启动")
		}
		return
	}
	fmt.Fprintf(out, "Node: %s\n", node.Name)
	fmt.Fprintf(out, "- Exit mode: %s\n", node.ExitMode)
	fmt.Fprintf(out, "- Local proxy: %s://%s:%d\n", node.Proxy, node.BindHost, node.LocalPort)
	if node.Endpoint != "" {
		fmt.Fprintf(out, "- WireGuard endpoint: %s\n", node.Endpoint)
	}
	if node.WGDevice != "" {
		fmt.Fprintf(out, "- Remote WireGuard device: %s\n", node.WGDevice)
	}
	if proxyStarted {
		fmt.Fprintln(out, "- Local proxy service: started")
	}
}
