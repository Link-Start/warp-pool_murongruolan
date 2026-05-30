package deploy

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/murongruolan/warp-pool/internal/config"
	"github.com/murongruolan/warp-pool/internal/sshclient"
)

type SSHOptions struct {
	Host     string
	Port     int
	User     string
	Password string
	KeyPath  string
}

type PushOptions struct {
	SSH           SSHOptions
	Node          config.Node
	DryRun        bool
	RemoteDir     string
	AssetsDir     string
	SkipPortCheck bool
}

type PushResult struct {
	Node config.Node
	Logs []string
}

func Push(cfg config.Config, opts PushOptions) (config.Config, PushResult, error) {
	if opts.RemoteDir == "" {
		opts.RemoteDir = "/tmp/warppool-install"
	}
	if opts.AssetsDir == "" {
		opts.AssetsDir = "assets"
	}
	if opts.Node.ExitMode == "" {
		opts.Node.ExitMode = cfg.Defaults.ExitMode
	}
	if opts.Node.Proxy == "" {
		opts.Node.Proxy = cfg.Defaults.Proxy
	}
	if opts.Node.BindHost == "" {
		opts.Node.BindHost = cfg.Defaults.BindHost
	}

	if err := config.ValidateNode(cfg, opts.Node); err != nil {
		return cfg, PushResult{}, err
	}
	if !opts.SkipPortCheck {
		if err := config.CheckPortAvailable(opts.Node.BindHost, opts.Node.LocalPort); err != nil {
			return cfg, PushResult{}, err
		}
	}

	result := PushResult{Node: opts.Node}
	if opts.DryRun {
		result.Logs = append(result.Logs, "dry-run: skip ssh connect")
		result.Logs = append(result.Logs, fmt.Sprintf("dry-run: upload assets to %s", opts.RemoteDir))
		result.Logs = append(result.Logs, fmt.Sprintf("dry-run: run bash %s/install.sh --dry-run mode=%s", opts.RemoteDir, opts.Node.ExitMode))
		return cfg, result, nil
	}

	client, err := sshclient.Dial(sshclient.Config{
		Host: opts.SSH.Host,
		Port: opts.SSH.Port,
		User: opts.SSH.User,
		Auth: sshclient.Auth{
			Password: opts.SSH.Password,
			KeyPath:  opts.SSH.KeyPath,
		},
		Timeout: 20 * time.Second,
	})
	if err != nil {
		return cfg, result, err
	}
	defer client.Close()

	if err := uploadAssets(client, opts.AssetsDir, opts.RemoteDir, &result); err != nil {
		return cfg, result, err
	}

	command := fmt.Sprintf("bash %s mode=%s", shellPath(filepath.ToSlash(filepath.Join(opts.RemoteDir, "install.sh"))), opts.Node.ExitMode)
	remoteResult, err := client.Run(command)
	result.Logs = append(result.Logs, remoteResult.Stdout)
	if remoteResult.Stderr != "" {
		result.Logs = append(result.Logs, remoteResult.Stderr)
	}
	if err != nil {
		return cfg, result, err
	}

	next, err := config.AddNode(cfg, opts.Node)
	return next, result, err
}

func uploadAssets(client *sshclient.Client, assetsDir string, remoteDir string, result *PushResult) error {
	if _, err := client.Run("mkdir -p " + shellPath(remoteDir)); err != nil {
		return err
	}

	entries, err := os.ReadDir(assetsDir)
	if err != nil {
		return fmt.Errorf("read assets dir: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sh") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(assetsDir, entry.Name()))
		if err != nil {
			return err
		}
		remotePath := filepath.ToSlash(filepath.Join(remoteDir, entry.Name()))
		if err := client.Upload(remotePath, data, "0755"); err != nil {
			return err
		}
		result.Logs = append(result.Logs, "uploaded: "+remotePath)
	}
	return nil
}

func shellPath(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
