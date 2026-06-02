package server

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/murongruolan/warp-pool/internal/config"
	"github.com/murongruolan/warp-pool/internal/wireguard"
)

type RegisterRequest struct {
	Token string `json:"token"`
}

type PrepareRequest struct {
	Token            string `json:"token"`
	Endpoint         string `json:"endpoint"`
	EndpointPort     int    `json:"endpoint_port"`
	ServerPrivateKey string `json:"server_private_key"`
	ServerPublicKey  string `json:"server_public_key"`
	ListenPort       int    `json:"listen_port"`
	Mode             string `json:"mode,omitempty"`
}

type PrepareResponse struct {
	OK           bool        `json:"ok"`
	Message      string      `json:"message,omitempty"`
	Node         config.Node `json:"node,omitempty"`
	ServerConfig string      `json:"server_config,omitempty"`
}

type RegisterResponse struct {
	OK      bool        `json:"ok"`
	Message string      `json:"message,omitempty"`
	Node    config.Node `json:"node,omitempty"`
}

type NodeModeRequest struct {
	Token string `json:"token"`
}

type NodeModeResponse struct {
	OK          bool        `json:"ok"`
	Message     string      `json:"message,omitempty"`
	Node        config.Node `json:"node,omitempty"`
	TargetMode  string      `json:"target_mode,omitempty"`
	WarpInstall string      `json:"warp_install,omitempty"`
	RemoveWarp  bool        `json:"remove_warp,omitempty"`
}

func RegisterHandler(configPath string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, RegisterResponse{OK: true, Message: "ok"})
	})
	mux.HandleFunc("/register/info", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writePrepareJSON(w, http.StatusMethodNotAllowed, PrepareResponse{OK: false, Message: "method not allowed"})
			return
		}

		var req RegisterRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writePrepareJSON(w, http.StatusBadRequest, PrepareResponse{OK: false, Message: "invalid json body"})
			return
		}
		if req.Token == "" {
			writePrepareJSON(w, http.StatusBadRequest, PrepareResponse{OK: false, Message: "token is required"})
			return
		}

		cfg, err := config.Load(configPath)
		if err != nil {
			writePrepareJSON(w, http.StatusInternalServerError, PrepareResponse{OK: false, Message: "load config failed"})
			fmt.Printf("[WarpPool][register][ERROR] load config: %v\n", err)
			return
		}
		_, token, err := config.FindDeployToken(cfg, req.Token, time.Now().UTC())
		if err != nil {
			writePrepareJSON(w, http.StatusBadRequest, PrepareResponse{OK: false, Message: err.Error()})
			return
		}

		node := token.Node
		if node.ExitMode == "" {
			node.ExitMode = cfg.Defaults.ExitMode
		}
		if node.Proxy == "" {
			node.Proxy = cfg.Defaults.Proxy
		}
		if node.BindHost == "" {
			node.BindHost = cfg.Defaults.BindHost
		}

		response := PrepareResponse{OK: true, Message: "ok", Node: node}
		if r.URL.Query().Get("format") == "sh" {
			writePrepareShell(w, http.StatusOK, response)
			return
		}
		writePrepareJSON(w, http.StatusOK, response)
	})
	mux.HandleFunc("/register/prepare", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writePrepareJSON(w, http.StatusMethodNotAllowed, PrepareResponse{OK: false, Message: "method not allowed"})
			return
		}

		var req PrepareRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writePrepareJSON(w, http.StatusBadRequest, PrepareResponse{OK: false, Message: "invalid json body"})
			return
		}
		if req.Token == "" {
			writePrepareJSON(w, http.StatusBadRequest, PrepareResponse{OK: false, Message: "token is required"})
			return
		}
		if strings.TrimSpace(req.Endpoint) == "" {
			writePrepareJSON(w, http.StatusBadRequest, PrepareResponse{OK: false, Message: "endpoint is required"})
			return
		}

		cfg, err := config.Load(configPath)
		if err != nil {
			writePrepareJSON(w, http.StatusInternalServerError, PrepareResponse{OK: false, Message: "load config failed"})
			fmt.Printf("[WarpPool][register][ERROR] load config: %v\n", err)
			return
		}
		_, token, err := config.FindDeployToken(cfg, req.Token, time.Now().UTC())
		if err != nil {
			writePrepareJSON(w, http.StatusBadRequest, PrepareResponse{OK: false, Message: err.Error()})
			return
		}

		node := token.Node
		if node.ExitMode == "" {
			node.ExitMode = cfg.Defaults.ExitMode
		}
		if node.Proxy == "" {
			node.Proxy = cfg.Defaults.Proxy
		}
		if node.BindHost == "" {
			node.BindHost = cfg.Defaults.BindHost
		}
		if node.PublicIP == "" {
			node.PublicIP = req.Endpoint
		}

		listenPort := req.ListenPort
		if listenPort == 0 {
			listenPort = wireguard.DefaultListenPort
		}
		endpointPort := req.EndpointPort
		if endpointPort == 0 {
			endpointPort = listenPort
		}
		plan, err := wireguard.BuildPlan(cfg, wireguard.Options{
			Node:             node,
			Endpoint:         req.Endpoint,
			EndpointPort:     endpointPort,
			ListenPort:       listenPort,
			EnableForwarding: node.ExitMode == config.ExitModeDirect || node.ExitMode == config.ExitModeDual,
			ServerPrivateKey: req.ServerPrivateKey,
			ServerPublicKey:  req.ServerPublicKey,
		})
		if err != nil {
			writePrepareJSON(w, http.StatusBadRequest, PrepareResponse{OK: false, Message: err.Error()})
			return
		}
		node = wireguard.ApplyPlan(node, plan)
		cfg, err = config.PrepareDeployToken(cfg, req.Token, node, time.Now().UTC())
		if err != nil {
			writePrepareJSON(w, http.StatusBadRequest, PrepareResponse{OK: false, Message: err.Error()})
			return
		}
		if err := config.SaveExisting(configPath, cfg); err != nil {
			writePrepareJSON(w, http.StatusInternalServerError, PrepareResponse{OK: false, Message: "save config failed"})
			fmt.Printf("[WarpPool][register][ERROR] save config: %v\n", err)
			return
		}

		response := PrepareResponse{OK: true, Message: "prepared", Node: node, ServerConfig: plan.ServerConfig}
		if r.URL.Query().Get("format") == "sh" {
			writePrepareShell(w, http.StatusOK, response)
			return
		}
		writePrepareJSON(w, http.StatusOK, response)
	})
	mux.HandleFunc("/register/complete", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, RegisterResponse{OK: false, Message: "method not allowed"})
			return
		}

		var req RegisterRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, RegisterResponse{OK: false, Message: "invalid json body"})
			return
		}
		if req.Token == "" {
			writeJSON(w, http.StatusBadRequest, RegisterResponse{OK: false, Message: "token is required"})
			return
		}
		autostartDisabled := r.URL.Query().Get("autostart") == "0"

		cfg, err := config.Load(configPath)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, RegisterResponse{OK: false, Message: "load config failed"})
			fmt.Printf("[WarpPool][register][ERROR] load config: %v\n", err)
			return
		}
		cfg, node, err := config.CompleteDeployToken(cfg, req.Token, time.Now().UTC())
		if err != nil {
			writeJSON(w, http.StatusBadRequest, RegisterResponse{OK: false, Message: err.Error()})
			return
		}
		var completedToken config.DeployToken
		for _, item := range cfg.Tokens {
			if item.Token == req.Token {
				completedToken = item
				break
			}
		}
		if err := config.SaveExisting(configPath, cfg); err != nil {
			writeJSON(w, http.StatusInternalServerError, RegisterResponse{OK: false, Message: "save config failed"})
			fmt.Printf("[WarpPool][register][ERROR] save config: %v\n", err)
			return
		}
		if completedToken.AutoStart && !autostartDisabled {
			if err := spawnProxyAutostart(configPath, node.Name); err != nil {
				fmt.Printf("[WarpPool][register][WARN] auto start proxy watcher for %s failed: %v\n", node.Name, err)
			}
		}

		writeJSON(w, http.StatusOK, RegisterResponse{OK: true, Message: "registered", Node: node})
	})
	mux.HandleFunc("/node-mode/prepare", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeNodeModeJSON(w, http.StatusMethodNotAllowed, NodeModeResponse{OK: false, Message: "method not allowed"})
			return
		}

		var req NodeModeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeNodeModeJSON(w, http.StatusBadRequest, NodeModeResponse{OK: false, Message: "invalid json body"})
			return
		}
		if req.Token == "" {
			writeNodeModeJSON(w, http.StatusBadRequest, NodeModeResponse{OK: false, Message: "token is required"})
			return
		}

		cfg, err := config.Load(configPath)
		if err != nil {
			writeNodeModeJSON(w, http.StatusInternalServerError, NodeModeResponse{OK: false, Message: "load config failed"})
			fmt.Printf("[WarpPool][node-mode][ERROR] load config: %v\n", err)
			return
		}
		_, modeToken, err := config.FindNodeModeToken(cfg, req.Token, time.Now().UTC())
		if err != nil {
			writeNodeModeJSON(w, http.StatusBadRequest, NodeModeResponse{OK: false, Message: err.Error()})
			return
		}

		response := NodeModeResponse{
			OK:          true,
			Message:     "prepared",
			Node:        modeToken.Node,
			TargetMode:  modeToken.TargetMode,
			WarpInstall: modeToken.WarpInstall,
			RemoveWarp:  modeToken.RemoveWarp,
		}
		if response.WarpInstall == "" {
			response.WarpInstall = config.WarpInstallAuto
		}
		if r.URL.Query().Get("format") == "sh" {
			writeNodeModeShell(w, http.StatusOK, response)
			return
		}
		writeNodeModeJSON(w, http.StatusOK, response)
	})
	mux.HandleFunc("/node-mode/complete", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeNodeModeJSON(w, http.StatusMethodNotAllowed, NodeModeResponse{OK: false, Message: "method not allowed"})
			return
		}

		var req NodeModeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeNodeModeJSON(w, http.StatusBadRequest, NodeModeResponse{OK: false, Message: "invalid json body"})
			return
		}
		if req.Token == "" {
			writeNodeModeJSON(w, http.StatusBadRequest, NodeModeResponse{OK: false, Message: "token is required"})
			return
		}
		autostartDisabled := r.URL.Query().Get("autostart") == "0"

		cfg, err := config.Load(configPath)
		if err != nil {
			writeNodeModeJSON(w, http.StatusInternalServerError, NodeModeResponse{OK: false, Message: "load config failed"})
			fmt.Printf("[WarpPool][node-mode][ERROR] load config: %v\n", err)
			return
		}
		var completedToken config.NodeModeToken
		for _, item := range cfg.ModeTokens {
			if item.Token == req.Token {
				completedToken = item
				break
			}
		}
		cfg, node, err := config.CompleteNodeModeToken(cfg, req.Token, time.Now().UTC())
		if err != nil {
			writeNodeModeJSON(w, http.StatusBadRequest, NodeModeResponse{OK: false, Message: err.Error()})
			return
		}
		if err := config.SaveExisting(configPath, cfg); err != nil {
			writeNodeModeJSON(w, http.StatusInternalServerError, NodeModeResponse{OK: false, Message: "save config failed"})
			fmt.Printf("[WarpPool][node-mode][ERROR] save config: %v\n", err)
			return
		}
		if completedToken.AutoStart && !autostartDisabled {
			if err := spawnProxyAutostart(configPath, node.Name); err != nil {
				fmt.Printf("[WarpPool][node-mode][WARN] auto restart proxy watcher for %s failed: %v\n", node.Name, err)
			}
		}

		writeNodeModeJSON(w, http.StatusOK, NodeModeResponse{
			OK:          true,
			Message:     "completed",
			Node:        node,
			TargetMode:  node.ExitMode,
			WarpInstall: completedToken.WarpInstall,
			RemoveWarp:  completedToken.RemoveWarp,
		})
	})
	mux.HandleFunc("/register", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, RegisterResponse{OK: false, Message: "method not allowed"})
			return
		}

		var req RegisterRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, RegisterResponse{OK: false, Message: "invalid json body"})
			return
		}
		if req.Token == "" {
			writeJSON(w, http.StatusBadRequest, RegisterResponse{OK: false, Message: "token is required"})
			return
		}

		cfg, err := config.Load(configPath)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, RegisterResponse{OK: false, Message: "load config failed"})
			fmt.Printf("[WarpPool][register][ERROR] load config: %v\n", err)
			return
		}

		cfg, node, err := config.UseDeployToken(cfg, req.Token, time.Now().UTC())
		if err != nil {
			writeJSON(w, http.StatusBadRequest, RegisterResponse{OK: false, Message: err.Error()})
			return
		}

		if err := config.SaveExisting(configPath, cfg); err != nil {
			writeJSON(w, http.StatusInternalServerError, RegisterResponse{OK: false, Message: "save config failed"})
			fmt.Printf("[WarpPool][register][ERROR] save config: %v\n", err)
			return
		}

		writeJSON(w, http.StatusOK, RegisterResponse{OK: true, Message: "registered", Node: node})
	})
	return mux
}

func writeJSON(w http.ResponseWriter, status int, value RegisterResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writePrepareJSON(w http.ResponseWriter, status int, value PrepareResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeNodeModeJSON(w http.ResponseWriter, status int, value NodeModeResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writePrepareShell(w http.ResponseWriter, status int, value PrepareResponse) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	ok := "0"
	if value.OK {
		ok = "1"
	}
	fmt.Fprintf(w, "OK=%s\n", ok)
	fmt.Fprintf(w, "MESSAGE_B64=%s\n", shellB64(value.Message))
	fmt.Fprintf(w, "NODE_NAME_B64=%s\n", shellB64(value.Node.Name))
	fmt.Fprintf(w, "WG_DEVICE_B64=%s\n", shellB64(value.Node.WGDevice))
	fmt.Fprintf(w, "WG_SERVER_ADDR_B64=%s\n", shellB64(value.Node.WGServerAddress))
	fmt.Fprintf(w, "WG_CLIENT_ADDR_B64=%s\n", shellB64(value.Node.WGClientAddress))
	fmt.Fprintf(w, "WG_WARP_CLIENT_ADDR_B64=%s\n", shellB64(value.Node.WGWarpClientAddress))
	fmt.Fprintf(w, "NODE_EXIT_MODE_B64=%s\n", shellB64(value.Node.ExitMode))
	fmt.Fprintf(w, "SERVER_CONFIG_B64=%s\n", shellB64(value.ServerConfig))
}

func writeNodeModeShell(w http.ResponseWriter, status int, value NodeModeResponse) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	ok := "0"
	if value.OK {
		ok = "1"
	}
	removeWarp := "0"
	if value.RemoveWarp {
		removeWarp = "1"
	}
	fmt.Fprintf(w, "OK=%s\n", ok)
	fmt.Fprintf(w, "MESSAGE_B64=%s\n", shellB64(value.Message))
	fmt.Fprintf(w, "NODE_NAME_B64=%s\n", shellB64(value.Node.Name))
	fmt.Fprintf(w, "TARGET_MODE_B64=%s\n", shellB64(value.TargetMode))
	fmt.Fprintf(w, "WG_DEVICE_B64=%s\n", shellB64(value.Node.WGDevice))
	fmt.Fprintf(w, "WG_SERVER_ADDR_B64=%s\n", shellB64(value.Node.WGServerAddress))
	fmt.Fprintf(w, "WG_CLIENT_ADDR_B64=%s\n", shellB64(value.Node.WGClientAddress))
	fmt.Fprintf(w, "WG_WARP_CLIENT_ADDR_B64=%s\n", shellB64(value.Node.WGWarpClientAddress))
	fmt.Fprintf(w, "WARP_INSTALL_B64=%s\n", shellB64(value.WarpInstall))
	fmt.Fprintf(w, "REMOVE_WARP=%s\n", removeWarp)
}

func shellB64(value string) string {
	return base64.StdEncoding.EncodeToString([]byte(value))
}

var execCommand = exec.Command

func spawnProxyAutostart(configPath string, nodeName string) error {
	if execCommand == nil {
		return nil
	}
	if !filepath.IsAbs(configPath) {
		if abs, err := filepath.Abs(configPath); err == nil {
			configPath = abs
		}
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("detect warppool executable: %w", err)
	}
	stateDir := "/var/lib/warppool"
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return err
	}
	logPath := filepath.Join(stateDir, "autostart-"+safeFilePart(nodeName)+".log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer logFile.Close()

	cmd := execCommand(exe, "--config", configPath, "deploy-token", "wait-start", nodeName)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Stdin = nil
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

func safeFilePart(value string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(value) {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			if b.Len() > 0 {
				b.WriteRune('-')
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "node"
	}
	return out
}
