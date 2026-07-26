package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// EffectiveConfig is a flattened view of the effective configuration after
// environment variable overrides. Secrets are never exposed — masked fields
// use *_set boolean markers instead.
type EffectiveConfig struct {
	// Server section
	ServerOCPPPort  int    `json:"server.ocpp_port"`
	ServerOCPPPath  string `json:"server.ocpp_path"`
	ServerLogLevel  string `json:"server.log_level"`
	ServerLogFormat string `json:"server.log_format"`

	// MQTT section
	MQTTBroker         string            `json:"mqtt.broker"`
	MQTTClientID       string            `json:"mqtt.client_id"`
	MQTTUsername       string            `json:"mqtt.username"`
	MQTTPasswordSet    bool              `json:"mqtt.password_set"`
	MQTTBaseTopic      string            `json:"mqtt.base_topic"`
	MQTTTopics         map[string]string `json:"mqtt.topics"`
	MQTTDisconnectSec  int               `json:"mqtt.disconnect_threshold_sec"`

	// Charging section
	ChargingMinAmps       int `json:"charging.min_amps"`
	ChargingMaxAmps       int `json:"charging.max_amps"`
	ChargingContactorsSec int `json:"charging.contactor_cooldown_sec"`
	ChargingDefaultAmps   int `json:"charging.default_amps"`

	// WebUI section
	WebUIEnabled  bool   `json:"webui.enabled"`
	WebUIListen   string `json:"webui.listen"`
	WebUITokenSet bool   `json:"webui.token_set"`

	// OverriddenByEnv indicates which dotted-path keys differ between the
	// file-only config and the final config (with env overrides applied).
	OverriddenByEnv map[string]bool `json:"overridden_by_env"`
}

// loadFileOnly reads the config from path without applying env overrides.
// Returns default config if path is empty or file does not exist.
func loadFileOnly(path string) (*Config, error) {
	cfg := defaultConfig()

	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				return cfg, nil
			}
			return nil, fmt.Errorf("read config file %s: %w", path, err)
		}
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parse config yaml: %w", err)
		}
	}

	return cfg, nil
}

// Effective loads the config at path, computes which fields are overridden by
// environment variables, and returns a flattened, secret-masked view.
func Effective(path string) (*EffectiveConfig, error) {
	fileOnly, err := loadFileOnly(path)
	if err != nil {
		return nil, fmt.Errorf("load file-only config: %w", err)
	}

	full, err := Load(path)
	if err != nil {
		return nil, fmt.Errorf("load full config: %w", err)
	}

	ec := flatten(full)
	overrideKeys := diffKeys(fileOnly, full)
	ec.OverriddenByEnv = overrideKeys

	return ec, nil
}

func flatten(cfg *Config) *EffectiveConfig {
	return &EffectiveConfig{
		ServerOCPPPort:    cfg.Server.OCPPPort,
		ServerOCPPPath:    cfg.Server.OCPPPath,
		ServerLogLevel:    cfg.Server.LogLevel,
		ServerLogFormat:   cfg.Server.LogFormat,
		MQTTBroker:        cfg.MQTT.Broker,
		MQTTClientID:      cfg.MQTT.ClientID,
		MQTTUsername:      cfg.MQTT.Username,
		MQTTPasswordSet:   cfg.MQTT.Password != "",
		MQTTBaseTopic:     cfg.MQTT.BaseTopic,
		MQTTTopics:       cfg.MQTT.Topics,
		MQTTDisconnectSec: cfg.MQTT.DisconnectThresholdSec,
		ChargingMinAmps:   cfg.Charging.MinAmps,
		ChargingMaxAmps:   cfg.Charging.MaxAmps,
		ChargingContactorsSec: cfg.Charging.ContactorCooldownSec,
		ChargingDefaultAmps:  cfg.Charging.DefaultAmps,
		WebUIEnabled:      cfg.WebUI.Enabled,
		WebUIListen:       cfg.WebUI.Listen,
		WebUITokenSet:     cfg.WebUI.Token != "",
	}
}

func diffKeys(fileOnly, full *Config) map[string]bool {
	m := make(map[string]bool)

	addIfDifferent(fileOnly.Server.OCPPPort, full.Server.OCPPPort, "server.ocpp_port", m)
	addIfDifferent(fileOnly.Server.OCPPPath, full.Server.OCPPPath, "server.ocpp_path", m)
	addIfDifferent(fileOnly.Server.LogLevel, full.Server.LogLevel, "server.log_level", m)
	addIfDifferent(fileOnly.Server.LogFormat, full.Server.LogFormat, "server.log_format", m)

	addIfDifferent(fileOnly.MQTT.Broker, full.MQTT.Broker, "mqtt.broker", m)
	addIfDifferent(fileOnly.MQTT.ClientID, full.MQTT.ClientID, "mqtt.client_id", m)
	addIfDifferent(fileOnly.MQTT.Username, full.MQTT.Username, "mqtt.username", m)
	addIfDifferent(fileOnly.MQTT.Password, full.MQTT.Password, "mqtt.password", m)
	addIfDifferent(fileOnly.MQTT.BaseTopic, full.MQTT.BaseTopic, "mqtt.base_topic", m)
	addIfDifferent(fileOnly.MQTT.DisconnectThresholdSec, full.MQTT.DisconnectThresholdSec, "mqtt.disconnect_threshold_sec", m)

	addIfDifferent(fileOnly.Charging.MinAmps, full.Charging.MinAmps, "charging.min_amps", m)
	addIfDifferent(fileOnly.Charging.MaxAmps, full.Charging.MaxAmps, "charging.max_amps", m)
	addIfDifferent(fileOnly.Charging.ContactorCooldownSec, full.Charging.ContactorCooldownSec, "charging.contactor_cooldown_sec", m)
	addIfDifferent(fileOnly.Charging.DefaultAmps, full.Charging.DefaultAmps, "charging.default_amps", m)

	addIfDifferent(fileOnly.WebUI.Enabled, full.WebUI.Enabled, "webui.enabled", m)
	addIfDifferent(fileOnly.WebUI.Listen, full.WebUI.Listen, "webui.listen", m)
	addIfDifferent(fileOnly.WebUI.Token, full.WebUI.Token, "webui.token", m)

	return m
}

func addIfDifferent[T comparable](fileVal, fullVal T, key string, m map[string]bool) {
	if fileVal != fullVal {
		m[key] = true
	}
}