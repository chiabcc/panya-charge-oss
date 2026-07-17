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
