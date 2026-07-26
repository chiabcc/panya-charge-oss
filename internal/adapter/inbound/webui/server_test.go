package webui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// numVal extracts an int from a JSON-decoded value (int or float64).
func numVal(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case float64:
		return int(n)
	}
	return 0
}

func setupTestConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	cfg := `server:
  ocpp_port: 8887
  ocpp_path: "/{ws}"
  log_level: debug
  log_format: text
mqtt:
  broker: "tcp://localhost:1883"
  client_id: "panya-test"
  username: "test"
  password: "secret-pw"
  base_topic: "panya"
  disconnect_threshold_sec: 60
charging:
  min_amps: 10
  max_amps: 20
  contactor_cooldown_sec: 180
  default_amps: 10
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

func TestAuthMiddleware(t *testing.T) {
	const token = "test-token-here"

	t.Run("no_token_passes_through", func(t *testing.T) {
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		handler := authMiddleware(next, "")
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
		}
	})

	t.Run("valid_cookie_passes", func(t *testing.T) {
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		handler := authMiddleware(next, token)
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
		}
	})

	t.Run("missing_cookie_redirects", func(t *testing.T) {
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Error("next should not have been called")
		})
		handler := authMiddleware(next, token)
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusFound {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusFound)
		}
	})

	t.Run("wrong_cookie_forbidden", func(t *testing.T) {
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Error("next should not have been called")
		})
		handler := authMiddleware(next, token)
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "wrong-token"})
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
		}
	})
}

func TestSubtleValidate(t *testing.T) {
	tests := []struct {
		name   string
		a, b   string
		want   bool
	}{
		{name: "empty_a", a: "", b: "x", want: false},
		{name: "empty_b", a: "x", b: "", want: false},
		{name: "both_empty", a: "", b: "", want: false},
		{name: "same", a: "tokentoken", b: "tokentoken", want: true},
		{name: "diff_same_length", a: "aaaaaaaa", b: "bbbbbbbb", want: false},
		{name: "diff_lengths", a: "short", b: "longer", want: false},
		{name: "one_char_same", a: "x", b: "x", want: true},
		{name: "longer_tokens", a: "very-long-token-string-1234", b: "very-long-token-string-1234", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := subtleValidate(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("subtleValidate(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestSetSessionCookie(t *testing.T) {
	rec := httptest.NewRecorder()
	setSessionCookie(rec, "test-token")

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %d, want 1", len(cookies))
	}
	c := cookies[0]
	if c.Name != sessionCookieName {
		t.Errorf("name = %q, want %q", c.Name, sessionCookieName)
	}
	if c.Value != "test-token" {
		t.Errorf("value = %q, want %q", c.Value, "test-token")
	}
	if !c.HttpOnly {
		t.Error("HttpOnly = false, want true")
	}
	if c.SameSite != http.SameSiteStrictMode {
		t.Errorf("SameSite = %d, want %d", c.SameSite, http.SameSiteStrictMode)
	}
	if c.Path != "/" {
		t.Errorf("Path = %q, want %q", c.Path, "/")
	}
}

func TestClearSessionCookie(t *testing.T) {
	rec := httptest.NewRecorder()
	clearSessionCookie(rec)

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %d, want 1", len(cookies))
	}
	c := cookies[0]
	if c.Name != sessionCookieName {
		t.Errorf("name = %q, want %q", c.Name, sessionCookieName)
	}
	if c.Value != "" {
		t.Errorf("value = %q, want empty", c.Value)
	}
	if c.MaxAge >= 0 {
		t.Errorf("MaxAge = %d, want < 0", c.MaxAge)
	}
}

func TestHandleLogin(t *testing.T) {
	t.Parallel()

	t.Run("correct_token_redirects", func(t *testing.T) {
		t.Parallel()
		cfgPath := setupTestConfig(t)
		srv := NewServer(cfgPath, ":0", "test-token-32-chars-minimum-xx", true)

		body := bytes.NewReader([]byte("token=test-token-32-chars-minimum-xx"))
		req := httptest.NewRequest(http.MethodPost, "/login", body)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		srv.handleLogin(rec, req)

		if rec.Code != http.StatusFound {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusFound)
		}
		if rec.Header().Get("Location") != "/api/config" {
			t.Errorf("Location = %q, want %q", rec.Header().Get("Location"), "/api/config")
		}
	})

	t.Run("wrong_token_shows_error", func(t *testing.T) {
		t.Parallel()
		cfgPath := setupTestConfig(t)
		srv := NewServer(cfgPath, ":0", "test-token-32-chars-minimum-xx", true)

		body := bytes.NewReader([]byte("token=wrong"))
		req := httptest.NewRequest(http.MethodPost, "/login", body)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		srv.handleLogin(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		if rec.Header().Get("Content-Type") != "text/html; charset=utf-8" {
			t.Errorf("Content-Type = %q, want text/html", rec.Header().Get("Content-Type"))
		}
	})
}

func TestHandleLogout(t *testing.T) {
	t.Parallel()

	cfgPath := setupTestConfig(t)
	srv := NewServer(cfgPath, ":0", "test-token", true)

	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	rec := httptest.NewRecorder()
	srv.handleLogout(rec, req)

	if rec.Code != http.StatusFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusFound)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != sessionCookieName {
		t.Errorf("expected session cookie to be cleared, got %v", cookies)
	}
}

func TestHandleIndex(t *testing.T) {
	t.Parallel()

	t.Run("no_token_loopback_redirects_to_config", func(t *testing.T) {
		t.Parallel()
		cfgPath := setupTestConfig(t)
		srv := NewServer(cfgPath, ":0", "", true)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		srv.handleIndex(rec, req)

		if rec.Code != http.StatusFound {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusFound)
		}
	})

	t.Run("with_token_no_cookie_shows_login", func(t *testing.T) {
		t.Parallel()
		cfgPath := setupTestConfig(t)
		srv := NewServer(cfgPath, ":0", "test-token-32-chars-minimum-xx", true)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		srv.handleIndex(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		ct := rec.Header().Get("Content-Type")
		if ct != "text/html; charset=utf-8" {
			t.Errorf("Content-Type = %q, want text/html", ct)
		}
	})

	t.Run("with_token_valid_cookie_redirects", func(t *testing.T) {
		t.Parallel()
		cfgPath := setupTestConfig(t)
		srv := NewServer(cfgPath, ":0", "test-token-32-chars-minimum-xx", true)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "test-token-32-chars-minimum-xx"})
		rec := httptest.NewRecorder()
		srv.handleIndex(rec, req)

		if rec.Code != http.StatusFound {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusFound)
		}
	})
}

func TestHandleGetConfig(t *testing.T) {
	t.Parallel()

	cfgPath := setupTestConfig(t)
	srv := NewServer(cfgPath, ":0", "token", true)

	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	rec := httptest.NewRecorder()
	srv.handleGetConfig(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if rec.Header().Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", rec.Header().Get("Content-Type"))
	}

	var dto configDTO
	if err := json.NewDecoder(rec.Body).Decode(&dto); err != nil {
		t.Fatalf("decode DTO: %v", err)
	}

	t.Run("password_not_exposed", func(t *testing.T) {
		pwField, ok := dto.Fields["mqtt.password"]
		if !ok {
			t.Fatal("fields missing mqtt.password")
		}
		if pwField.Value != nil {
			t.Errorf("password value = %v, want nil", pwField.Value)
		}
		if !pwField.Secret {
			t.Error("password secret = false, want true")
		}
		if !pwField.PasswordSet {
			t.Error("password password_set = false, want true")
		}
	})

	t.Run("token_not_exposed", func(t *testing.T) {
		tkField, ok := dto.Fields["webui.token"]
		if !ok {
			t.Fatal("fields missing webui.token")
		}
		if tkField.Value != nil {
			t.Errorf("token value = %v, want nil", tkField.Value)
		}
		if !tkField.Secret {
			t.Error("token secret = false, want true")
		}
		if !tkField.TokenSet {
			t.Error("token token_set = false, want true")
		}
	})

	t.Run("charging_values_present", func(t *testing.T) {
		t.Parallel()
		f, ok := dto.Fields["charging.min_amps"]
		if !ok {
			t.Fatal("fields missing charging.min_amps")
		}
		if v := numVal(f.Value); v != 10 {
			t.Errorf("charging.min_amps value = %v (%T), want 10", f.Value, f.Value)
		}
	})

	t.Run("apply_classes_present", func(t *testing.T) {
		if dto.ApplyClasses["server.log_level"] != "hot" {
			t.Errorf("apply_classes[server.log_level] = %q, want hot", dto.ApplyClasses["server.log_level"])
		}
		if dto.ApplyClasses["webui.enabled"] != "process_restart" {
			t.Errorf("apply_classes[webui.enabled] = %q, want process_restart", dto.ApplyClasses["webui.enabled"])
		}
	})
}

func TestHandleGetConfigFresh(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	// Write initial config
	initial := fmt.Sprintf(`server:
  ocpp_port: 8887
  ocpp_path: "/{ws}"
  log_level: info
  log_format: text
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
  token: "tok"
`)
	if err := os.WriteFile(cfgPath, []byte(initial), 0o600); err != nil {
		t.Fatal(err)
	}

	srv := NewServer(cfgPath, ":0", "tok", true)

	// First GET
	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	rec := httptest.NewRecorder()
	srv.handleGetConfig(rec, req)
	var dto1 configDTO
	if err := json.NewDecoder(rec.Body).Decode(&dto1); err != nil {
		t.Fatal(err)
	}
	minAmps1 := numVal(dto1.Fields["charging.min_amps"].Value)
	if minAmps1 != 6 {
		t.Errorf("initial min_amps = %d, want 6", minAmps1)
	}

	// Edit on disk
	edited := fmt.Sprintf(`server:
  ocpp_port: 8887
  ocpp_path: "/{ws}"
  log_level: info
  log_format: text
mqtt:
  broker: "tcp://localhost:1883"
  client_id: "panya"
  base_topic: "panya"
charging:
  min_amps: 12
  max_amps: 32
  contactor_cooldown_sec: 180
  default_amps: 12
webui:
  enabled: true
  listen: "127.0.0.1:8888"
  token: "tok"
`)
	if err := os.WriteFile(cfgPath, []byte(edited), 0o600); err != nil {
		t.Fatal(err)
	}

	// Second GET — should reflect the change
	req = httptest.NewRequest(http.MethodGet, "/api/config", nil)
	rec = httptest.NewRecorder()
	srv.handleGetConfig(rec, req)
	var dto2 configDTO
	if err := json.NewDecoder(rec.Body).Decode(&dto2); err != nil {
		t.Fatal(err)
	}
	minAmps2 := numVal(dto2.Fields["charging.min_amps"].Value)
	if minAmps2 != 12 {
		t.Errorf("after edit min_amps = %d, want 12 (proves no caching)", minAmps2)
	}
}

func TestHandlePostConfig(t *testing.T) {
	t.Parallel()

	cfgPath := setupTestConfig(t)
	srv := NewServer(cfgPath, ":0", "tok", true)

	req := httptest.NewRequest(http.MethodPost, "/api/config", bytes.NewReader(nil))
	rec := httptest.NewRecorder()
	srv.handlePostConfig(rec, req)

	if rec.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

func TestStaticAssetsServed(t *testing.T) {
	t.Parallel()

	cfgPath := setupTestConfig(t)
	srv := NewServer(cfgPath, ":0", "tok", true)

	// Route through full mux
	req := httptest.NewRequest(http.MethodGet, "/static/htmx.min.js", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /static/htmx.min.js status = %d, want 200", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if ct == "" {
		t.Error("Content-Type empty for static asset")
	}
	body, _ := io.ReadAll(rec.Body)
	if len(body) == 0 {
		t.Error("static asset body is empty")
	}
}

func TestEnvOverrideFlags(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	// Config file says broker is file-broker
	cfg := `server:
  ocpp_port: 8887
  ocpp_path: "/{ws}"
  log_level: info
  log_format: text
mqtt:
  broker: "tcp://file-broker:1883"
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
  token: "tok"
`
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}

	// Set env override
	prev := os.Getenv("PANYA_MQTT_BROKER")
	t.Cleanup(func() { os.Setenv("PANYA_MQTT_BROKER", prev) })
	os.Setenv("PANYA_MQTT_BROKER", "tcp://env-broker:1883")

	srv := NewServer(cfgPath, ":0", "tok", true)

	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	rec := httptest.NewRecorder()
	srv.handleGetConfig(rec, req)

	var dto configDTO
	if err := json.NewDecoder(rec.Body).Decode(&dto); err != nil {
		t.Fatal(err)
	}

	f, ok := dto.Fields["mqtt.broker"]
	if !ok {
		t.Fatal("fields missing mqtt.broker")
	}
	if v, ok := f.Value.(string); !ok || v != "tcp://env-broker:1883" {
		t.Errorf("mqtt.broker value = %v, want tcp://env-broker:1883", f.Value)
	}
	if !f.OverriddenByEnv {
		t.Error("mqtt.broker overridden_by_env = false, want true")
	}

	// min_amps was not overridden
	maField, ok := dto.Fields["charging.min_amps"]
	if !ok {
		t.Fatal("fields missing charging.min_amps")
	}
	if maField.OverriddenByEnv {
		t.Error("charging.min_amps overridden_by_env = true, want false")
	}
}

func TestNewServerNoAuthOnLoopback(t *testing.T) {
	t.Parallel()

	cfgPath := setupTestConfig(t)
	srv := NewServer(cfgPath, ":0", "", true)

	// GET /api/config should work without cookie
	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (no auth on loopback without token)", rec.Code)
	}
}

func TestNewServerAuthRequiredWithToken(t *testing.T) {
	t.Parallel()

	cfgPath := setupTestConfig(t)
	srv := NewServer(cfgPath, ":0", "test-token-32-chars-minimum-xx", true)

	// GET /api/config without cookie should get 401 (redirect from authMiddleware)
	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	// authMiddleware redirects (302) when no cookie
	if rec.Code != http.StatusFound {
		t.Errorf("status = %d, want 302 redirect to login", rec.Code)
	}
}