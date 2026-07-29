package csmsfactory

import (
	"fmt"

	"github.com/chiabcc/panya-charge-oss/internal/config"
	internalcsms "github.com/chiabcc/panya-charge-oss/internal/csms"
	"github.com/chiabcc/panya-charge-oss/pkg/csms"
)

type Config struct {
	Server   ServerConfig
	MQTT     MQTTConfig
	Charging ChargingConfig
	Energy   config.EnergyConfig
}

type ServerConfig struct {
	OCPPPort  int
	OCPPPath  string
	LogLevel  string
	LogFormat string
}

type MQTTConfig struct {
	Broker                 string
	ClientID               string
	Username               string
	Password               string
	BaseTopic              string
	Topics                 map[string]string
	DisconnectThresholdSec int
}

type ChargingConfig struct {
	MinAmps              int
	MaxAmps              int
	ContactorCooldownSec int
	DefaultAmps          int
}

func New(cfg Config) (csms.Facade, error) {
	if cfg.Charging.MinAmps != 0 && cfg.Charging.MinAmps < 6 {
		return nil, fmt.Errorf("charging.min_amps must be >= 6, got %d", cfg.Charging.MinAmps)
	}
	if cfg.Charging.MaxAmps > 32 {
		return nil, fmt.Errorf("charging.max_amps must be <= 32, got %d", cfg.Charging.MaxAmps)
	}

	ocppPort := cfg.Server.OCPPPort
	if ocppPort == 0 {
		ocppPort = 8887
	}
	ocppPath := cfg.Server.OCPPPath
	if ocppPath == "" {
		ocppPath = "/{ws}"
	}
	logLevel := cfg.Server.LogLevel
	if logLevel == "" {
		logLevel = "info"
	}
	broker := cfg.MQTT.Broker
	if broker == "" {
		broker = "tcp://localhost:1883"
	}
	baseTopic := cfg.MQTT.BaseTopic
	if baseTopic == "" {
		baseTopic = "panya"
	}
	disconnectSec := cfg.MQTT.DisconnectThresholdSec
	if disconnectSec == 0 {
		disconnectSec = 60
	}
	minAmps := cfg.Charging.MinAmps
	if minAmps == 0 {
		minAmps = 6
	}
	maxAmps := cfg.Charging.MaxAmps
	if maxAmps == 0 {
		maxAmps = 32
	}

	internalCfg := config.Config{
		Server: config.ServerConfig{
			OCPPPort:  ocppPort,
			OCPPPath:  ocppPath,
			LogLevel:  logLevel,
			LogFormat: cfg.Server.LogFormat,
		},
		MQTT: config.MQTTConfig{
			Broker:                 broker,
			ClientID:               cfg.MQTT.ClientID,
			Username:               cfg.MQTT.Username,
			Password:               cfg.MQTT.Password,
			BaseTopic:              baseTopic,
			Topics:                 cfg.MQTT.Topics,
			DisconnectThresholdSec: disconnectSec,
		},
		Charging: config.ChargingConfig{
			MinAmps:              minAmps,
			MaxAmps:              maxAmps,
			ContactorCooldownSec: cfg.Charging.ContactorCooldownSec,
			DefaultAmps:          cfg.Charging.DefaultAmps,
		},
		Energy: cfg.Energy,
	}

	return internalcsms.New(internalCfg)
}
