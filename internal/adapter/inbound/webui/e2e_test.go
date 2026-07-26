package webui

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chiabcc/panya-charge-oss/internal/config"
)

const testToken = "test-token-32-chars-minimum-xx"

// extractSessionCookie returns session cookie from response
func extractSessionCookie(resp *http.Response) string {
	for _, c := range resp.Cookies() {
		if c.Name == sessionCookieName {
			return c.Value
		}
	}
	return ""
}

// numVal extracts an int from a JSON-decoded value (int or float64)
func e2eNumVal(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case float64:
		return int(n)
	}
	return 0
}

func e2eSetupConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cfg := `server:
  ocpp_port: 8887
  ocpp_path: "/{ws}"
  log_level: info
mqtt:
  broker: "tcp://localhost:1883"
  client_id: "panya"
  base_topic: "panya"
charging:
  min_amps: 6
  max_amps: 32
  contactor_cooldown_sec: 180
  default_amps: 6
webui:
  enabled: true
  listen: "127.0.0.1:8888"
  token: "test-token-32-chars-minimum-xx"
`
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestE2E_LoginAndConfig verifies the login flow and config retrieval.
func TestE2E_LoginAndConfig(t *testing.T) {
	t.Parallel()

	cfgPath := e2eSetupConfig(t)
	srv := NewServer(cfgPath, ":0", testToken, true, nil)

	// GET / should show login page (token is set, no valid cookie)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET / status = %d, want 200 (login page)", rec.Code)
	}
	if rec.Header().Get("Content-Type") != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/html", rec.Header().Get("Content-Type"))
	}

	// POST /login with correct token redirects to /api/config
	rec = httptest.NewRecorder()
	body := bytes.NewReader([]byte("token=" + testToken))
	req := httptest.NewRequest(http.MethodPost, "/login", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Errorf("POST /login status = %d, want 302", rec.Code)
	}
	if rec.Header().Get("Location") != "/api/config" {
		t.Errorf("Location = %q, want /api/config", rec.Header().Get("Location"))
	}
	cookieValue := extractSessionCookie(rec.Result())
	if cookieValue == "" {
		t.Fatal("no session cookie in login response")
	}

	// GET /api/config with session cookie returns JSON config
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/config", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: testToken})
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/config status = %d, want 200", rec.Code)
	}
	if rec.Header().Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", rec.Header().Get("Content-Type"))
	}

	var dto configDTO
	if err := json.NewDecoder(rec.Body).Decode(&dto); err != nil {
		t.Fatalf("decode config DTO: %v", err)
	}
	if len(dto.Fields) == 0 {
		t.Error("config DTO fields empty")
	}
	if _, ok := dto.Fields["charging.min_amps"]; !ok {
		t.Error("config DTO missing charging.min_amps")
	}
	if dto.Fields["charging.min_amps"] == nil {
		t.Error("charging.min_amps field is nil")
	}
	if v := e2eNumVal(dto.Fields["charging.min_amps"].Value); v != 6 {
		t.Errorf("charging.min_amps = %d, want 6", v)
	}
}

// TestE2E_HotSaveMinAmps verifies POST /api/config with hot-reloadable fields.
func TestE2E_HotSaveMinAmps(t *testing.T) {
	t.Parallel()

	cfgPath := e2eSetupConfig(t)
	srv := NewServer(cfgPath, ":0", testToken, true, nil)

	// POST charging.min_amps=10 returns hot_reload
	rec := httptest.NewRecorder()
	body := "charging.min_amps=10"
	req := httptest.NewRequest(http.MethodPost, "/api/config", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: testToken})
	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/config status = %d, want 200", rec.Code)
	}

	var result applyResult
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result.Action != "hot_reload" {
		t.Errorf("action = %q, want hot_reload", result.Action)
	}
	if !contains(result.FieldsChanged, "charging.min_amps") {
		t.Errorf("fields_changed missing charging.min_amps: %v", result.FieldsChanged)
	}

	// Verify config.yaml updated on disk
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte("min_amps: 10")) {
		t.Error("config file not updated: min_amps should be 10")
	}
}

// TestE2E_RebuildBrokerChange verifies rebuild-required fields.
func TestE2E_RebuildBrokerChange(t *testing.T) {
	t.Parallel()

	cfgPath := e2eSetupConfig(t)
	srv := NewServer(cfgPath, ":0", testToken, true, nil)

	// POST mqtt.broker change triggers rebuild path (no applier = written to disk)
	rec := httptest.NewRecorder()
	body := "mqtt.broker=tcp://new-broker:1883"
	req := httptest.NewRequest(http.MethodPost, "/api/config", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: testToken})
	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/config status = %d, want 200", rec.Code)
	}

	var result applyResult
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode result: %v", err)
	}

	if !contains(result.FieldsChanged, "mqtt.broker") {
		t.Error("fields_changed should include mqtt.broker")
	}
}

// TestE2E_SecuritySweep verifies wrong-token rejection, no auth bypass, and no token leakage.
func TestE2E_SecuritySweep(t *testing.T) {
	t.Parallel()

	cfgPath := e2eSetupConfig(t)
	srv := NewServer(cfgPath, ":0", testToken, true, nil)

	// 5x wrong-token POST /login, all rejected
	for i := 0; i < 5; i++ {
		rec := httptest.NewRecorder()
		wrongBody := bytes.NewReader([]byte("token=wrong-token-attempt"))
		req := httptest.NewRequest(http.MethodPost, "/login", wrongBody)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		srv.mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("attempt %d: wrong token status = %d, want 200 (login with error)", i, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "Invalid token") {
			t.Errorf("attempt %d: expected 'Invalid token' in response", i)
		}
		for _, c := range rec.Result().Cookies() {
			if c.Name == sessionCookieName && c.Value == testToken {
				t.Errorf("attempt %d: session cookie set with correct token", i)
			}
		}
	}

	// GET /api/config without auth cookie redirects to login
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Errorf("unauthenticated GET /api/config status = %d, want 302", rec.Code)
	}

	// Wrong cookie gets 403
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/config", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "wrong-cookie-value"})
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("wrong cookie GET /api/config status = %d, want 403", rec.Code)
	}

	// Token value not exposed in config JSON
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/config", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: testToken})
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/config status = %d, want 200", rec.Code)
	}
	configBody := rec.Body.String()
	if strings.Contains(configBody, testToken) {
		t.Error("webui.token VALUE leaked in /api/config response")
	}
}

// TestE2E_LANBindWithoutTokenRejected verifies validation prevents non-loopback bind.
func TestE2E_LANBindWithoutTokenRejected(t *testing.T) {
	cfg := `server:
  ocpp_port: 8887
mqtt:
  broker: "tcp://localhost:1883"
charging:
  min_amps: 6
  max_amps: 32
webui:
  enabled: true
  listen: "0.0.0.0:8888"
`
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(cfg), 0o600); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	_, err := config.Load(path)
	if err == nil {
		t.Error("expected validation error for non-loopback bind without token")
	} else if !strings.Contains(err.Error(), "token") {
		t.Errorf("expected token validation error, got: %v", err)
	}
}

// TestE2E_DefaultsWebUIDisabled verifies default config has webui disabled.
func TestE2E_DefaultsWebUIDisabled(t *testing.T) {
	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("load default config: %v", err)
	}
	if cfg.WebUI.Enabled {
		t.Error("default config should have webui disabled")
	}
	if cfg.WebUI.Listen != "127.0.0.1:8888" {
		t.Errorf("default listen = %q, want 127.0.0.1:8888", cfg.WebUI.Listen)
	}
}

// TestE2E_EnvOverrides verifies env var overrides for webui config.
func TestE2E_EnvOverridesWebUI(t *testing.T) {
	t.Setenv("PANYA_WEBUI_ENABLED", "true")
	t.Setenv("PANYA_WEBUI_LISTEN", "127.0.0.1:9999")
	t.Setenv("PANYA_WEBUI_TOKEN", "env-token-32-chars-minimum-ok")

	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.WebUI.Enabled {
		t.Error("webui should be enabled via env")
	}
	if cfg.WebUI.Listen != "127.0.0.1:9999" {
		t.Errorf("webui.listen = %q, want 127.0.0.1:9999", cfg.WebUI.Listen)
	}
	if cfg.WebUI.Token != "env-token-32-chars-minimum-ok" {
		t.Errorf("webui.token = %q, want env value", cfg.WebUI.Token)
	}
}