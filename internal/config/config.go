package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config holds all application configuration loaded from config.yaml and env vars.
type Config struct {
	Server   ServerConfig `yaml:"server"`
	MQTT     MQTTConfig   `yaml:"mqtt"`
	Charging ChargingConfig `yaml:"charging"`
}

type ServerConfig struct {
	OCPPPort  int    `yaml:"ocpp_port"`
	OCPPPath  string `yaml:"ocpp_path"`
	LogLevel  string `yaml:"log_level"`
	LogFormat string `yaml:"log_format"`
}

type MQTTConfig struct {
	Broker                 string            `yaml:"broker"`
	ClientID               string            `yaml:"client_id"`
	Username               string            `yaml:"username"`
	Password               string            `yaml:"password"`
	BaseTopic              string            `yaml:"base_topic"`
	Topics                 map[string]string `yaml:"topics"`
	DisconnectThresholdSec int               `yaml:"disconnect_threshold_sec"`
}

type ChargingConfig struct {
	MinAmps              int `yaml:"min_amps"`
	MaxAmps              int `yaml:"max_amps"`
	ContactorCooldownSec int `yaml:"contactor_cooldown_sec"`
	DefaultAmps          int `yaml:"default_amps"`
}

// Load reads configuration from the given YAML file path, then overlays
// environment variable overrides (PANYA_<SECTION>_<KEY> format).
func Load(path string) (*Config, error) {
	cfg := defaultConfig()

	if path != "" {
		//nolint:gosec // G304: config path is from trusted CLI flag
		data, err := os.ReadFile(path)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return nil, fmt.Errorf("read config file %s: %w", path, err)
			}
		} else {
			if err := yaml.Unmarshal(data, cfg); err != nil {
				return nil, fmt.Errorf("parse config yaml: %w", err)
			}
		}
	}

	applyEnvOverrides(cfg)

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func defaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			OCPPPort:  8887,
			OCPPPath:  "/{ws}",
			LogLevel:  "info",
			LogFormat: "text",
		},
		MQTT: MQTTConfig{
			Broker:                 "tcp://localhost:1883",
			ClientID:               "panya-charge",
			BaseTopic:              "panya",
			DisconnectThresholdSec: 60,
		Topics: map[string]string{
			"charge_status":           "charge/status",
			"charge_power":            "charge/power",
			"charge_energy":           "charge/energy",
			"grid_power":              "grid/power",
			"solar_power":             "",
			"consumption_power":       "",
			"command_set_amps":        "charge/command/set_amps",
			"command_state":           "charge/command/state",
			"smart_charging_command":  "smart_charging/command",
		},
		},
		Charging: ChargingConfig{
			MinAmps:              6,
			MaxAmps:              32,
			ContactorCooldownSec: 180,
			DefaultAmps:          6,
		},
	}
}

func applyEnvOverrides(cfg *Config) {
	strOverrides := map[string]*string{
		"PANYA_MQTT_BROKER":       &cfg.MQTT.Broker,
		"PANYA_MQTT_CLIENT_ID":    &cfg.MQTT.ClientID,
		"PANYA_MQTT_USERNAME":     &cfg.MQTT.Username,
		"PANYA_MQTT_PASSWORD":     &cfg.MQTT.Password,
		"PANYA_MQTT_BASE_TOPIC":   &cfg.MQTT.BaseTopic,
		"PANYA_SERVER_LOG_LEVEL":  &cfg.Server.LogLevel,
		"PANYA_SERVER_LOG_FORMAT": &cfg.Server.LogFormat,
	}
	for env, ptr := range strOverrides {
		if val := os.Getenv(env); val != "" {
			*ptr = val
		}
	}

	intOverrides := map[string]*int{
		"PANYA_SERVER_OCPP_PORT": &cfg.Server.OCPPPort,
	}
	for env, ptr := range intOverrides {
		if val := os.Getenv(env); val != "" {
			if n, err := strconv.Atoi(val); err == nil {
				*ptr = n
			}
		}
	}
}

func (c *Config) validate() error {
	if c.Server.OCPPPort <= 0 || c.Server.OCPPPort > 65535 {
		return fmt.Errorf("server.ocpp_port must be 1-65535, got %d", c.Server.OCPPPort)
	}
	if c.Charging.MinAmps < 6 {
		return fmt.Errorf("charging.min_amps must be >= 6 (IEC 61851 minimum), got %d", c.Charging.MinAmps)
	}
	if c.Charging.MaxAmps > 32 {
		return fmt.Errorf("charging.max_amps must be <= 32 (Type 2 maximum), got %d", c.Charging.MaxAmps)
	}
	if c.Server.LogFormat != "" && c.Server.LogFormat != "json" && c.Server.LogFormat != "text" {
		return fmt.Errorf("server.log_format must be 'json' or 'text', got %q", c.Server.LogFormat)
	}
	if c.Charging.MinAmps > c.Charging.MaxAmps {
		return fmt.Errorf("charging.min_amps (%d) > charging.max_amps (%d)",
			c.Charging.MinAmps, c.Charging.MaxAmps)
	}
	return nil
}

// LogLevelUpper returns the log level in uppercase for slog.
func (c *Config) LogLevelUpper() string {
	return strings.ToUpper(c.Server.LogLevel)
}
