package config

import (
	"os"
	"testing"
)

func TestEnvOverrides_ChargingIntegers(t *testing.T) {
	t.Run("charging_min_amps", func(t *testing.T) {
		t.Setenv("PANYA_CHARGING_MIN_AMPS", "10")
		cfg, err := Load("/nonexistent/config.yaml")
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.Charging.MinAmps != 10 {
			t.Errorf("MinAmps = %d, want 10 (env override)", cfg.Charging.MinAmps)
		}
	})

	t.Run("charging_max_amps", func(t *testing.T) {
		t.Setenv("PANYA_CHARGING_MAX_AMPS", "24")
		cfg, err := Load("/nonexistent/config.yaml")
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.Charging.MaxAmps != 24 {
			t.Errorf("MaxAmps = %d, want 24 (env override)", cfg.Charging.MaxAmps)
		}
	})

	t.Run("charging_default_amps", func(t *testing.T) {
		t.Setenv("PANYA_CHARGING_DEFAULT_AMPS", "16")
		cfg, err := Load("/nonexistent/config.yaml")
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.Charging.DefaultAmps != 16 {
			t.Errorf("DefaultAmps = %d, want 16 (env override)", cfg.Charging.DefaultAmps)
		}
	})

	t.Run("charging_contactor_cooldown_sec", func(t *testing.T) {
		t.Setenv("PANYA_CHARGING_CONTACTOR_COOLDOWN_SEC", "240")
		cfg, err := Load("/nonexistent/config.yaml")
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.Charging.ContactorCooldownSec != 240 {
			t.Errorf("ContactorCooldownSec = %d, want 240 (env override)", cfg.Charging.ContactorCooldownSec)
		}
	})
}

func TestEnvOverrides_MQTTDisconnectThreshold(t *testing.T) {
	t.Setenv("PANYA_MQTT_DISCONNECT_THRESHOLD_SEC", "120")
	cfg, err := Load("/nonexistent/config.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.MQTT.DisconnectThresholdSec != 120 {
		t.Errorf("DisconnectThresholdSec = %d, want 120 (env override)", cfg.MQTT.DisconnectThresholdSec)
	}
}

func TestEnvOverrides_MQTTEncoding(t *testing.T) {
	t.Run("topic_grid_power", func(t *testing.T) {
		t.Setenv("PANYA_MQTT_TOPIC_GRID_POWER", "home/home/energy/power/total/instantaneous")
		cfg, err := Load("/nonexistent/config.yaml")
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.MQTT.Topics["grid_power"] != "home/home/energy/power/total/instantaneous" {
			t.Errorf("Topics[grid_power] = %q, want %q (env override)",
				cfg.MQTT.Topics["grid_power"], "home/home/energy/power/total/instantaneous")
		}
	})

	t.Run("topic_solar_power", func(t *testing.T) {
		t.Setenv("PANYA_MQTT_TOPIC_SOLAR_POWER", "energy/solar/power/instantaneous")
		cfg, err := Load("/nonexistent/config.yaml")
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.MQTT.Topics["solar_power"] != "energy/solar/power/instantaneous" {
			t.Errorf("Topics[solar_power] = %q, want %q (env override)",
				cfg.MQTT.Topics["solar_power"], "energy/solar/power/instantaneous")
		}
	})

	t.Run("topic_consumption_power", func(t *testing.T) {
		t.Setenv("PANYA_MQTT_TOPIC_CONSUMPTION_POWER", "energy/power/instantaneous")
		cfg, err := Load("/nonexistent/config.yaml")
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.MQTT.Topics["consumption_power"] != "energy/power/instantaneous" {
			t.Errorf("Topics[consumption_power] = %q, want %q (env override)",
				cfg.MQTT.Topics["consumption_power"], "energy/power/instantaneous")
		}
	})
}

func TestEnvOverrides_Defaults(t *testing.T) {
	t.Run("charging_defaults_without_env", func(t *testing.T) {
		_ = os.Unsetenv("PANYA_CHARGING_MIN_AMPS")
		_ = os.Unsetenv("PANYA_CHARGING_MAX_AMPS")
		_ = os.Unsetenv("PANYA_CHARGING_DEFAULT_AMPS")
		_ = os.Unsetenv("PANYA_CHARGING_CONTACTOR_COOLDOWN_SEC")

		cfg, err := Load("/nonexistent/config.yaml")
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.Charging.MinAmps != 6 {
			t.Errorf("MinAmps = %d, want 6 (default)", cfg.Charging.MinAmps)
		}
		if cfg.Charging.MaxAmps != 32 {
			t.Errorf("MaxAmps = %d, want 32 (default)", cfg.Charging.MaxAmps)
		}
		if cfg.Charging.DefaultAmps != 6 {
			t.Errorf("DefaultAmps = %d, want 6 (default)", cfg.Charging.DefaultAmps)
		}
		if cfg.Charging.ContactorCooldownSec != 180 {
			t.Errorf("ContactorCooldownSec = %d, want 180 (default)", cfg.Charging.ContactorCooldownSec)
		}
	})

	t.Run("empty_env_does_not_clear_default", func(t *testing.T) {
		os.Setenv("PANYA_CHARGING_MIN_AMPS", "")
		os.Unsetenv("PANYA_CHARGING_MIN_AMPS")

		cfg, err := Load("/nonexistent/config.yaml")
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.Charging.MinAmps != 6 {
			t.Errorf("MinAmps = %d, want 6 (empty env should not override default)", cfg.Charging.MinAmps)
		}

		os.Setenv("PANYA_MQTT_DISCONNECT_THRESHOLD_SEC", "")
		os.Unsetenv("PANYA_MQTT_DISCONNECT_THRESHOLD_SEC")

		if cfg.MQTT.DisconnectThresholdSec != 60 {
			t.Errorf("DisconnectThresholdSec = %d, want 60 (empty env should not override default)", cfg.MQTT.DisconnectThresholdSec)
		}
	})
}

func TestEnvOverrides_MQTTTopicsEmptyDoesNotClear(t *testing.T) {
	_ = os.Unsetenv("PANYA_MQTT_TOPIC_GRID_POWER")
	_ = os.Unsetenv("PANYA_MQTT_TOPIC_SOLAR_POWER")
	_ = os.Unsetenv("PANYA_MQTT_TOPIC_CONSUMPTION_POWER")

	cfg, err := Load("/nonexistent/config.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.MQTT.Topics["grid_power"] != "grid/power" {
		t.Errorf("Topics[grid_power] = %q, want %q (empty env should not clear default)",
			cfg.MQTT.Topics["grid_power"], "grid/power")
	}
}