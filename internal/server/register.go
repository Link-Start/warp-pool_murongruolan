package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/murongruolan/warp-pool/internal/config"
)

type RegisterRequest struct {
	Token string `json:"token"`
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
