package csms

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	inmqtt "github.com/chiabcc/panya-charge-oss/internal/adapter/inbound/mqtt"
	iha "github.com/chiabcc/panya-charge-oss/internal/adapter/inbound/ha"
	outmqtt "github.com/chiabcc/panya-charge-oss/internal/adapter/outbound/mqtt"
	"github.com/chiabcc/panya-charge-oss/internal/adapter/outbound/ocpp"
	"github.com/chiabcc/panya-charge-oss/internal/config"
	"github.com/chiabcc/panya-charge-oss/internal/domain/ports"
	"github.com/chiabcc/panya-charge-oss/internal/domain/proxy"
	"github.com/chiabcc/panya-charge-oss/internal/domain/smartcharging"
	pkgcsms "github.com/chiabcc/panya-charge-oss/pkg/csms"
)

const (
	controllerPollInterval = 10 * time.Second
	gridVoltage            = 230.0
)

type CSMS struct {
	emitter     *pkgcsms.Emitter
	chargerRepo *ports.InMemoryChargerRepository
	sessionRepo *ports.InMemorySessionRepository
	meterRepo   *ports.InMemoryMeterRepository
	proxyRepo   *ports.InMemoryProxyConfigRepository

	handler    *ocpp.Handler
	controller *ocpp.Controller
	calc       *smartcharging.Calculator
	server     *ocpp.Server
	commander  *ocpp.Commander
	publisher  *outmqtt.Publisher
	subscriber *inmqtt.Subscriber
	energy     ports.EnergySource

	cancelFn context.CancelFunc
	wg       sync.WaitGroup
	started  atomic.Bool
	logger   *slog.Logger
	levelVar *slog.LevelVar
}

func New(cfg config.Config) (*CSMS, error) {
	levelVar := &slog.LevelVar{}
	levelVar.Set(parseLogLevel(cfg.Server.LogLevel))

	logger := buildLogger(levelVar, cfg.Server.LogFormat)
	cfg.CheckDeprecatedEnergyTopics(logger)

	chargerRepo := ports.NewInMemoryChargerRepository()
	sessionRepo := ports.NewInMemorySessionRepository()
	meterRepo := ports.NewInMemoryMeterRepository()
	proxyRepo := ports.NewInMemoryProxyConfigRepository()

	emitter := pkgcsms.NewEmitter(0, logger)

	var energy ports.EnergySource
	energyConfigured := cfg.Energy.HASS.GridEntityID != "" || cfg.Energy.HASS.SolarEntityID != "" || cfg.Energy.HASS.ConsumptionEntityID != ""
	if energyConfigured {
		hassCfg := iha.HASSConfig{
			GridEntityID:        cfg.Energy.HASS.GridEntityID,
			SolarEntityID:       cfg.Energy.HASS.SolarEntityID,
			ConsumptionEntityID: cfg.Energy.HASS.ConsumptionEntityID,
			Token:               cfg.Energy.HASS.Token,
		}
		energy = iha.NewEnergySource(hassCfg, "http://supervisor/core/api", cfg.Energy.HASS.Token, logger)
		logger.Info("energy source configured — smart charging enabled",
			"grid", hassCfg.GridEntityID,
			"solar", hassCfg.SolarEntityID,
			"consumption", hassCfg.ConsumptionEntityID,
		)
	} else {
		energy = ports.NoOpEnergySource{}
		logger.Info("no energy entities configured — smart charging disabled")
	}

	publisher, err := outmqtt.NewPublisher(
		cfg.MQTT.Broker, cfg.MQTT.ClientID,
		cfg.MQTT.Username, cfg.MQTT.Password,
		cfg.MQTT.BaseTopic, cfg.MQTT.Topics,
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("mqtt publisher: %w", err)
	}

	relay := ocpp.NewNoopRelay(logger)
	router := ocpp.NewRouter(proxy.DefaultPolicy(), relay, logger)

	handler := ocpp.NewHandler(
		router,
		chargerRepo,
		sessionRepo,
		meterRepo,
		proxyRepo,
		publisher,
		publisher,
		nil,
		cfg.Charging.MinAmps,
		cfg.Charging.MaxAmps,
		logger,
		nil,
	)
	handler.SetEmitter(emitter)

	server, err := ocpp.NewServer(cfg.Server.OCPPPort, cfg.Server.OCPPPath, handler, logger)
	if err != nil {
		publisher.Close()
		return nil, fmt.Errorf("ocpp server: %w", err)
	}

	commander := ocpp.NewCommander(server.CentralSystem(), logger)

	calc := smartcharging.NewCalculator(cfg.Charging.MinAmps, cfg.Charging.MaxAmps, gridVoltage)

	staleTimeout := time.Duration(cfg.MQTT.DisconnectThresholdSec) * time.Second
	if staleTimeout <= 0 {
		staleTimeout = 60 * time.Second
	}

	controller := ocpp.NewController(
		commander,
		chargerRepo,
		energy,
		publisher,
		calc,
		cfg.Charging.MinAmps,
		controllerPollInterval,
		staleTimeout,
		logger,
	)
	controller.SetEmitter(emitter)

	// Disable smart charging when no energy entities are configured.
	// NoOpEnergySource reports always-stale, which would cause the controller
	// to revert all chargers to safe state (6A) every 10 seconds — even though
	// the user has no intention of using smart charging.
	if !energyConfigured {
		controller.SetEnabled(false)
	}

	publisher.SetOnReconnect(func() {
		publisher.PublishGlobalDiscovery()
		publisher.PublishSmartChargingEnabled(controller.IsEnabled())
	})

	cmd := &cmdBridge{
		commander:   commander,
		chargerRepo: chargerRepo,
		sessionRepo: sessionRepo,
		publisher:   publisher,
		controller:  controller,
		logger:      logger,
	}

	subscriber, err := inmqtt.NewSubscriber(
		cfg.MQTT.Broker, cfg.MQTT.ClientID,
		cfg.MQTT.Username, cfg.MQTT.Password,
		cfg.MQTT.BaseTopic, cfg.MQTT.Topics,
		cmd, logger,
	)
	if err != nil {
		publisher.Close()
		return nil, fmt.Errorf("mqtt subscriber: %w", err)
	}

	return &CSMS{
		emitter:     emitter,
		chargerRepo: chargerRepo,
		sessionRepo: sessionRepo,
		meterRepo:   meterRepo,
		proxyRepo:   proxyRepo,
		handler:     handler,
		controller:  controller,
		calc:        calc,
		server:      server,
		commander:   commander,
		publisher:   publisher,
		subscriber:  subscriber,
		energy:      energy,
		logger:      logger,
		levelVar:    levelVar,
	}, nil
}

func (c *CSMS) Start(ctx context.Context) error {
	if !c.started.CompareAndSwap(false, true) {
		return fmt.Errorf("csms already started")
	}

	ctx, cancel := context.WithCancel(ctx)
	c.cancelFn = cancel

	c.publisher.PublishGlobalDiscovery()
	c.publisher.PublishSmartChargingEnabled(c.controller.IsEnabled())

	c.energy.Start(ctx)
	c.server.Start()

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.controller.Run(ctx)
	}()

	<-ctx.Done()
	return ctx.Err()
}

func (c *CSMS) Stop() {
	if c.cancelFn != nil {
		c.cancelFn()
	}
	c.energy.Stop()
	c.server.Stop()
	if c.subscriber != nil {
		c.subscriber.Close()
	}
	if c.publisher != nil {
		c.publisher.Close()
	}
	c.wg.Wait()
	c.started.Store(false)
}

func (c *CSMS) Subscribe(ctx context.Context, buffer int) <-chan pkgcsms.Event {
	if !c.started.Load() {
		return nil
	}
	if buffer <= 0 {
		buffer = pkgcsms.DefaultEventBufferSize
	}

	src := c.emitter.Subscribe()
	out := make(chan pkgcsms.Event, buffer)

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-src:
				if !ok {
					return
				}
				select {
				case out <- ev:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return out
}

func (c *CSMS) Chargers() []pkgcsms.ChargerInfo {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	chargers, err := c.chargerRepo.ListChargers(ctx)
	if err != nil {
		return []pkgcsms.ChargerInfo{}
	}

	result := make([]pkgcsms.ChargerInfo, 0, len(chargers))
	for _, ch := range chargers {
		info := pkgcsms.ChargerInfo{
			ID:           ch.ID,
			Vendor:       ch.Vendor,
			Model:        ch.Model,
			Firmware:     ch.FirmwareVersion,
			SerialNumber: ch.SerialNumber,
		}
		if !ch.Online {
			info.Status = "Unavailable"
		}

		conns, _ := c.chargerRepo.ListConnectors(ctx, ch.ID)
		if len(conns) > 0 {
			info.ConnectorID = conns[0].ConnectorID
			info.Status = string(conns[0].Status)
		}

		if active, _ := c.sessionRepo.GetActiveSession(ctx, ch.ID, info.ConnectorID); active != nil {
			info.TxID = active.TransactionID
		}

		result = append(result, info)
	}

	return result
}

// UpdateCharging validates and applies charging parameter changes at runtime.
func (c *CSMS) UpdateCharging(params pkgcsms.ChargingParams) error {
	if params.MinAmps >= 6 && params.MaxAmps <= 32 && params.MinAmps <= params.MaxAmps {
		c.calc.SetLimits(params.MinAmps, params.MaxAmps)
		c.controller.SetSafeAmps(params.DefaultAmps)
		c.commander.SetCooldown(time.Duration(params.ContactorCooldownSec) * time.Second)
		c.handler.SetMinMax(params.MinAmps, params.MaxAmps)
		c.logger.Info("charging params updated",
			"minAmps", params.MinAmps,
			"maxAmps", params.MaxAmps,
			"cooldownSec", params.ContactorCooldownSec,
			"defaultAmps", params.DefaultAmps,
		)
		return nil
	}
	if params.MinAmps < 6 {
		return fmt.Errorf("minAmps must be >= 6, got %d", params.MinAmps)
	}
	if params.MaxAmps > 32 {
		return fmt.Errorf("maxAmps must be <= 32, got %d", params.MaxAmps)
	}
	return fmt.Errorf("minAmps (%d) > maxAmps (%d)", params.MinAmps, params.MaxAmps)
}

// HasActiveSession returns the IDs of chargers with an active transaction.
func (c *CSMS) HasActiveSession() ([]string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	chargers, err := c.chargerRepo.ListChargers(ctx)
	if err != nil {
		return nil, false
	}

	var ids []string
	for _, ch := range chargers {
		conns, err := c.chargerRepo.ListConnectors(ctx, ch.ID)
		if err != nil {
			continue
		}
		for _, conn := range conns {
			if active, _ := c.sessionRepo.GetActiveSession(ctx, ch.ID, conn.ConnectorID); active != nil {
				ids = append(ids, ch.ID)
				break
			}
		}
	}
	return ids, len(ids) > 0
}

// SetLogLevel adjusts the log level at runtime via LevelVar.
func (c *CSMS) SetLogLevel(level string) error {
	lv := parseLogLevel(level)
	upper := strings.ToUpper(level)
	if upper != "DEBUG" && upper != "INFO" && upper != "WARN" && upper != "ERROR" {
		return fmt.Errorf("unknown log level %q (use debug, info, warn, or error)", level)
	}
	c.levelVar.Set(lv)
	c.logger.Info("log level updated", "level", level)
	return nil
}

func (c *CSMS) MQTTStatus() (bool, string) {
	return c.publisher.Status()
}

func (c *CSMS) ChargingState() pkgcsms.ChargingState {
	state := c.controller.State()
	return pkgcsms.ChargingState{
		CurrentAmps:    state.CurrentAmps,
		GridPowerW:     state.GridPowerW,
		SolarPowerW:    state.SolarPowerW,
		ConsumptionW:   state.ConsumptionW,
		Enabled:        state.Enabled,
	}
}

type cmdBridge struct {
	commander   ports.ChargerCommander
	chargerRepo ports.ChargerRepository
	sessionRepo ports.SessionRepository
	publisher   ports.EventPublisher
	controller  smartChargingToggle
	logger      *slog.Logger
}

type smartChargingToggle interface {
	SetEnabled(enabled bool)
	IsEnabled() bool
}

func (b *cmdBridge) OnSetAmps(chargerID string, amps int) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if chargerID == "" {
		chargers, err := b.chargerRepo.ListChargers(ctx)
		if err != nil {
			b.logger.Error("cmd: list chargers failed", "err", err)
			return
		}
		for _, ch := range chargers {
			b.applyAmps(ctx, ch.ID, amps)
		}
	} else {
		b.applyAmps(ctx, chargerID, amps)
	}
}

func (b *cmdBridge) applyAmps(ctx context.Context, chargerID string, amps int) {
	conns, err := b.chargerRepo.ListConnectors(ctx, chargerID)
	if err != nil {
		b.logger.Error("cmd: list connectors failed", "charger", chargerID, "err", err)
		return
	}
	for _, conn := range conns {
		if conn.ConnectorID == 0 {
			continue
		}
		if err := b.commander.SetChargingProfile(chargerID, conn.ConnectorID, amps); err != nil {
			b.logger.Error("cmd: set charging profile failed", "charger", chargerID, "err", err)
		}
	}
}

func (b *cmdBridge) OnSetState(chargerID string, charging bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	ids := []string{chargerID}
	if chargerID == "" {
		chargers, err := b.chargerRepo.ListChargers(ctx)
		if err != nil {
			b.logger.Error("cmd: list chargers failed", "err", err)
			return
		}
		ids = ids[:0]
		for _, ch := range chargers {
			ids = append(ids, ch.ID)
		}
	}

	for _, id := range ids {
		if charging {
			b.startCharging(ctx, id)
		} else {
			b.stopCharging(ctx, id)
		}
	}
}

func (b *cmdBridge) OnSetSmartCharging(enabled bool) {
	if b.controller == nil {
		b.logger.Warn("smart charging toggle received but controller not wired")
		return
	}
	b.controller.SetEnabled(enabled)
	if b.publisher != nil {
		b.publisher.PublishSmartChargingEnabled(enabled)
	}
}

func (b *cmdBridge) startCharging(ctx context.Context, chargerID string) {
	conns, err := b.chargerRepo.ListConnectors(ctx, chargerID)
	if err != nil {
		b.logger.Error("cmd: list connectors for start failed", "charger", chargerID, "err", err)
		return
	}
	for _, conn := range conns {
		if conn.ConnectorID == 0 {
			continue
		}
		if err := b.commander.RemoteStartTransaction(chargerID, conn.ConnectorID, "default"); err != nil {
			b.logger.Error("cmd: remote start failed", "charger", chargerID, "err", err)
		}
	}
}

func (b *cmdBridge) stopCharging(ctx context.Context, chargerID string) {
	conns, err := b.chargerRepo.ListConnectors(ctx, chargerID)
	if err != nil {
		b.logger.Error("cmd: list connectors for stop failed", "charger", chargerID, "err", err)
		return
	}
	for _, conn := range conns {
		active, _ := b.sessionRepo.GetActiveSession(ctx, chargerID, conn.ConnectorID)
		if active == nil || active.TransactionID == 0 {
			continue
		}
		if err := b.commander.RemoteStopTransaction(chargerID, active.TransactionID); err != nil {
			b.logger.Error("cmd: remote stop failed", "charger", chargerID, "err", err)
		}
	}
}

func buildLogger(level *slog.LevelVar, format string) *slog.Logger {
	opts := &slog.HandlerOptions{Level: level}
	var h slog.Handler
	if strings.ToLower(format) == "json" {
		h = slog.NewJSONHandler(os.Stderr, opts)
	} else {
		h = slog.NewTextHandler(os.Stderr, opts)
	}
	return slog.New(h)
}

func parseLogLevel(level string) slog.Level {
	switch strings.ToUpper(level) {
	case "DEBUG":
		return slog.LevelDebug
	case "WARN":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
