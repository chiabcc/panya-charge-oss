package main

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	"github.com/chiabcc/panya-charge-oss/internal/config"
	"github.com/chiabcc/panya-charge-oss/pkg/csms"
	"github.com/chiabcc/panya-charge-oss/pkg/csmsfactory"
)

type webuiApplier struct {
	mu     sync.Mutex
	facade csms.Facade
	ctx    context.Context
}

func newWebUIApplier(facade csms.Facade, ctx context.Context) *webuiApplier {
	return &webuiApplier{facade: facade, ctx: ctx}
}

func (a *webuiApplier) UpdateCharging(params csms.ChargingParams) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.facade.UpdateCharging(params)
}

func (a *webuiApplier) SetLogLevel(level string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.facade.SetLogLevel(level)
}

func (a *webuiApplier) HasActiveSession() ([]string, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.facade.HasActiveSession()
}

func (a *webuiApplier) Rebuild(cfg *config.Config) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.facade.Stop()

	newFacade, err := buildFacade(cfg)
	if err != nil {
		return err
	}
	a.facade = newFacade
	go runFacade(newFacade, a.ctx)
	slog.Info("csms rebuilt from webui config",
		"ocpp_port", cfg.Server.OCPPPort,
		"mqtt_broker", cfg.MQTT.Broker,
		"base_topic", cfg.MQTT.BaseTopic,
	)
	return nil
}

func (a *webuiApplier) Shutdown() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.facade.Stop()
}

func runFacade(f csms.Facade, ctx context.Context) {
	if err := f.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("csms stopped with error", "error", err)
	}
}

func buildFacade(cfg *config.Config) (csms.Facade, error) {
	return csmsfactory.New(csmsfactory.Config{
		Server: csmsfactory.ServerConfig{
			OCPPPort:  cfg.Server.OCPPPort,
			OCPPPath:  cfg.Server.OCPPPath,
			LogLevel:  cfg.Server.LogLevel,
			LogFormat: cfg.Server.LogFormat,
		},
		MQTT: csmsfactory.MQTTConfig{
			Broker:                 cfg.MQTT.Broker,
			ClientID:               cfg.MQTT.ClientID,
			Username:               cfg.MQTT.Username,
			Password:               cfg.MQTT.Password,
			BaseTopic:              cfg.MQTT.BaseTopic,
			Topics:                 cfg.MQTT.Topics,
			DisconnectThresholdSec: cfg.MQTT.DisconnectThresholdSec,
		},
		Charging: csmsfactory.ChargingConfig{
			MinAmps:              cfg.Charging.MinAmps,
			MaxAmps:              cfg.Charging.MaxAmps,
			ContactorCooldownSec: cfg.Charging.ContactorCooldownSec,
			DefaultAmps:          cfg.Charging.DefaultAmps,
		},
	})
}
