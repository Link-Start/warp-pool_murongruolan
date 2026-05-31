package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/murongruolan/warp-pool/internal/config"
)

func TestRegisterPrepareAndComplete(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := config.Default()
	var err error
	cfg, err = config.AddDeployToken(cfg, config.DeployToken{
		Token:     "token-1",
		ExpiresAt: time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
		Node: config.Node{
			Name:      "nat1",
			ExitMode:  config.ExitModeDirect,
			Proxy:     config.ProxyMixed,
			BindHost:  "127.0.0.1",
			LocalPort: 10013,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := config.Save(path, cfg, true); err != nil {
		t.Fatal(err)
	}

	handler := RegisterHandler(path)
	body := bytes.NewBufferString(`{"token":"token-1","endpoint":"203.0.113.10","endpoint_port":30021,"server_private_key":"server-private","server_public_key":"server-public","listen_port":51820}`)
	req := httptest.NewRequest(http.MethodPost, "/register/prepare", body)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("prepare status %d: %s", rec.Code, rec.Body.String())
	}

	var prepare PrepareResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &prepare); err != nil {
		t.Fatal(err)
	}
	if !prepare.OK || prepare.Node.WGDevice == "" {
		t.Fatalf("unexpected prepare response: %#v", prepare)
	}
	if !strings.Contains(prepare.ServerConfig, "PrivateKey = server-private") {
		t.Fatalf("server config did not use provided key:\n%s", prepare.ServerConfig)
	}
	if prepare.Node.Endpoint != "203.0.113.10:30021" {
		t.Fatalf("unexpected node endpoint: %s", prepare.Node.Endpoint)
	}

	req = httptest.NewRequest(http.MethodPost, "/register/complete", bytes.NewBufferString(`{"token":"token-1"}`))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("complete status %d: %s", rec.Code, rec.Body.String())
	}

	next, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Nodes) != 1 || !next.Tokens[0].Used || !next.Tokens[0].Registered {
		t.Fatalf("unexpected config after complete: %#v", next)
	}
}

func TestRegisterPrepareShellFormat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := config.Default()
	var err error
	cfg, err = config.AddDeployToken(cfg, config.DeployToken{
		Token:     "token-1",
		ExpiresAt: time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
		Node: config.Node{
			Name:      "nat1",
			ExitMode:  config.ExitModeDirect,
			Proxy:     config.ProxyMixed,
			BindHost:  "127.0.0.1",
			LocalPort: 10013,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := config.Save(path, cfg, true); err != nil {
		t.Fatal(err)
	}

	handler := RegisterHandler(path)
	body := bytes.NewBufferString(`{"token":"token-1","endpoint":"203.0.113.10","server_private_key":"server-private","server_public_key":"server-public","listen_port":51820}`)
	req := httptest.NewRequest(http.MethodPost, "/register/prepare?format=sh", body)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("prepare status %d: %s", rec.Code, rec.Body.String())
	}
	for _, want := range []string{"OK=1", "WG_DEVICE_B64=", "SERVER_CONFIG_B64="} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("shell response missing %q:\n%s", want, rec.Body.String())
		}
	}
}
