package config

import "testing"

func TestDefaultsWhenNoFile(t *testing.T) {
	cfg, err := Load("/nonexistent/config.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.OCPPPort != 8887 {
		t.Errorf("OCPPPort = %d, want 8887", cfg.Server.OCPPPort)
	}
	if cfg.Charging.MinAmps != 6 {
		t.Errorf("MinAmps = %d, want 6", cfg.Charging.MinAmps)
	}
	if cfg.Charging.ContactorCooldownSec != 180 {
		t.Errorf("ContactorCooldownSec = %d, want 180", cfg.Charging.ContactorCooldownSec)
	}
}

func TestEnvOverrides(t *testing.T) {
	t.Setenv("PANYA_SERVER_OCPP_PORT", "7777")
	t.Setenv("PANYA_MQTT_BROKER", "tcp://override:1883")

	cfg, err := Load("/nonexistent/config.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.OCPPPort != 7777 {
		t.Errorf("OCPPPort = %d, want 7777 (env override)", cfg.Server.OCPPPort)
	}
	if cfg.MQTT.Broker != "tcp://override:1883" {
		t.Errorf("MQTT.Broker = %q, want %q (env override)", cfg.MQTT.Broker, "tcp://override:1883")
	}
}

func TestEnvOverride_LogFormat(t *testing.T) {
	t.Setenv("PANYA_SERVER_LOG_FORMAT", "json")

	cfg, err := Load("/nonexistent/config.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.LogFormat != "json" {
		t.Errorf("LogFormat = %q, want %q (env override)", cfg.Server.LogFormat, "json")
	}
}

func TestValidation_LogFormat(t *testing.T) {
	cfg := defaultConfig()
	cfg.Server.LogFormat = "xml"
	if err := cfg.validate(); err == nil {
		t.Error("validate should fail for invalid log_format")
	}
}

func TestValidation_MinAmps(t *testing.T) {
	cfg := defaultConfig()
	cfg.Charging.MinAmps = 3
	if err := cfg.validate(); err == nil {
		t.Error("validate should fail for min_amps < 6")
	}
}

func TestValidation_MinExceedsMax(t *testing.T) {
	cfg := defaultConfig()
	cfg.Charging.MinAmps = 20
	cfg.Charging.MaxAmps = 10
	if err := cfg.validate(); err == nil {
		t.Error("validate should fail when min_amps > max_amps")
	}
}

func TestWebUIDefaults(t *testing.T) {
	t.Helper()

	cfg := defaultConfig()

	if cfg.WebUI.Enabled != false {
		t.Errorf("Enabled = %v, want false", cfg.WebUI.Enabled)
	}
	if cfg.WebUI.Listen != "127.0.0.1:8888" {
		t.Errorf("Listen = %q, want %q", cfg.WebUI.Listen, "127.0.0.1:8888")
	}
	if cfg.WebUI.Token != "" {
		t.Errorf("Token = %q, want empty", cfg.WebUI.Token)
	}
}

func TestWebUIEnabled(t *testing.T) {
	t.Helper()

	cfg := defaultConfig()
	cfg.WebUI.Enabled = true

	if err := cfg.validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
}

func TestWebUIEnablement(t *testing.T) {
	t.Helper()

	t.Run("false disabled", func(t *testing.T) {
		cfg := defaultConfig()
		cfg.WebUI.Enabled = false
		if err := cfg.validate(); err != nil {
			t.Errorf("validate: %v", err)
		}
	})

	t.Run("true enabled", func(t *testing.T) {
		cfg := defaultConfig()
		cfg.WebUI.Enabled = true
		if err := cfg.validate(); err != nil {
			t.Errorf("validate: %v", err)
		}
	})
}

func TestWebUIListen(t *testing.T) {
	t.Helper()

	tests := []struct {
		name    string
		listen  string
		wantErr bool
	}{
		{"loopback", "127.0.0.1:8888", false},
		{"localhost", "localhost:8888", false},
		{"ipv6 loopback", "::1:8888", false},
		{"ipv6 loopback bracketed", "[::1]:8888", false},
		{"all", "0.0.0.0:8888", true},
		{"localhost any", "localhost", false},
		{"port only", ":8888", true},
		{"ipv4 any", "0.0.0.0", true},
		{"ipv6 any", "::", true},
		{"ipv6 any bracketed", "[::]", true},
		{"invalid", "127.256.0:8888", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := defaultConfig()
			cfg.WebUI.Listen = tt.listen
			cfg.WebUI.Enabled = true
			if err := cfg.validate(); (err != nil) != tt.wantErr {
				t.Errorf("validate() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestWebUIListenAddr(t *testing.T) {
	t.Helper()

	tests := []struct {
		name    string
		addr    string
		wantErr bool
	}{
		{"valid port", ":8888", true},
		{"valid port 0", ":0", true},
		{"no port", "localhost", false},
		{"no port all", "0.0.0.0", true},
		{"no port any", "::", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := defaultConfig()
			cfg.WebUI.Listen = tt.addr
			cfg.WebUI.Enabled = true
			if err := cfg.validate(); (err != nil) != tt.wantErr {
				t.Errorf("validate() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestWebUIToken(t *testing.T) {
	t.Helper()

	cfg := defaultConfig()
	cfg.WebUI.Listen = "0.0.0.0:8888"
	cfg.WebUI.Enabled = true
	cfg.WebUI.Token = "mytoken"

	if err := cfg.validate(); err != nil {
		t.Errorf("validate: %v", err)
	}
}

func TestWebUIEnv(t *testing.T) {
	t.Helper()

	t.Setenv("PANYA_WEBUI_ENABLED", "true")
	t.Setenv("PANYA_WEBUI_LISTEN", "0.0.0.0:8888")
	t.Setenv("PANYA_WEBUI_TOKEN", "envtoken")

	cfg, err := Load("/nonexistent/config.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if !cfg.WebUI.Enabled {
		t.Errorf("Enabled = %v, want true (env)", cfg.WebUI.Enabled)
	}
	if cfg.WebUI.Listen != "0.0.0.0:8888" {
		t.Errorf("Listen = %q, want %q (env)", cfg.WebUI.Listen, "0.0.0.0:8888")
	}
	if cfg.WebUI.Token != "envtoken" {
		t.Errorf("Token = %q, want %q (env)", cfg.WebUI.Token, "envtoken")
	}
}

func TestWebUIEnvBool(t *testing.T) {
	t.Helper()

	tests := []struct {
		name   string
		val    string
		wantEn bool
	}{
		{"1 enabled", "1", true},
		{"true enabled", "true", true},
		{"0 disabled", "0", false},
		{"false disabled", "false", false},
		{"empty disabled", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("PANYA_WEBUI_ENABLED", tt.val)
			cfg, err := Load("/nonexistent/config.yaml")
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if cfg.WebUI.Enabled != tt.wantEn {
				t.Errorf("Enabled = %v, want %v (env %q)", cfg.WebUI.Enabled, tt.wantEn, tt.val)
			}
		})
	}
}
