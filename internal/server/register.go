package server

import (
	"encoding/json"
	"fmt"
	"net/http"
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

func RegisterHandler(configPath string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, RegisterResponse{OK: true, Message: "ok"})
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
			EnableForwarding: node.ExitMode == config.ExitModeDirect,
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

		writePrepareJSON(w, http.StatusOK, PrepareResponse{OK: true, Message: "prepared", Node: node, ServerConfig: plan.ServerConfig})
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
		if err := config.SaveExisting(configPath, cfg); err != nil {
			writeJSON(w, http.StatusInternalServerError, RegisterResponse{OK: false, Message: "save config failed"})
			fmt.Printf("[WarpPool][register][ERROR] save config: %v\n", err)
			return
		}

		writeJSON(w, http.StatusOK, RegisterResponse{OK: true, Message: "registered", Node: node})
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
