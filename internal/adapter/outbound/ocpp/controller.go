package ocpp

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chiabcc/panya-charge-oss/internal/domain/charger"
	"github.com/chiabcc/panya-charge-oss/internal/domain/ports"
	"github.com/chiabcc/panya-charge-oss/internal/domain/smartcharging"
	"github.com/chiabcc/panya-charge-oss/pkg/csms"
)

type Controller struct {
	commander      ports.ChargerCommander
	chargerRepo    ports.ChargerRepository
	energySource   ports.EnergySource
	publisher      ports.EventPublisher
	calc           *smartcharging.Calculator
	pollInterval   time.Duration
	safeAmps       atomic.Int32
	staleTimeout   time.Duration
	logger         *slog.Logger
	lastSetAmps    sync.Map
	lastShouldStop sync.Map
	emitter        EventEmitter
	enabled        atomic.Bool
}

func NewController(
	cmd ports.ChargerCommander,
	cr ports.ChargerRepository,
	energy ports.EnergySource,
	pub ports.EventPublisher,
	calc *smartcharging.Calculator,
	safeAmps int,
	pollInterval time.Duration,
	staleTimeout time.Duration,
	logger *slog.Logger,
) *Controller {
	c := &Controller{
		commander:    cmd,
		chargerRepo:  cr,
		energySource: energy,
		publisher:    pub,
		calc:         calc,
		pollInterval: pollInterval,
		staleTimeout: staleTimeout,
		logger:       logger,
	}
	c.safeAmps.Store(int32(safeAmps))
	c.enabled.Store(true)
	return c
}

// SetEnabled toggles the smart-charging loop. When false, tick() short-circuits
// and manual control via the per-charger switches/number entities takes over.
func (c *Controller) SetEnabled(enabled bool) {
	prev := c.enabled.Swap(enabled)
	if prev == enabled {
		return
	}
	c.logger.Info("smart charging toggled", "enabled", enabled)
}

// IsEnabled reports whether the smart-charging loop is currently active.
func (c *Controller) IsEnabled() bool {
	return c.enabled.Load()
}

func (c *Controller) SetEmitter(e EventEmitter) {
	c.emitter = e
}

// SetSafeAmps updates the safe amps used as fallback when grid data is stale.
func (c *Controller) SetSafeAmps(amps int) {
	c.safeAmps.Store(int32(amps))
	c.logger.Info("controller: safe amps updated", "safeAmps", amps)
}

func (c *Controller) emit(ev csms.Event) {
	if c.emitter == nil {
		return
	}
	c.emitter.Emit(ev)
}

func (c *Controller) Run(ctx context.Context) {
	ticker := time.NewTicker(c.pollInterval)
	defer ticker.Stop()

	c.logger.Info("smart charging controller started", "interval", c.pollInterval)

	for {
		select {
		case <-ctx.Done():
			c.logger.Info("smart charging controller stopped")
			return
		case <-ticker.C:
			c.tick(ctx)
		}
	}
}

func (c *Controller) tick(ctx context.Context) {
	if !c.enabled.Load() {
		return
	}
	if c.energySource.IsStale(c.staleTimeout) {
		c.logger.Warn("energy data stale — reverting all chargers to safe state",
			"threshold", c.staleTimeout)
		c.revertAllToSafe(ctx)
		return
	}

	chargers, err := c.chargerRepo.ListChargers(ctx)
	if err != nil {
		c.logger.Error("failed to list chargers", "err", err)
		return
	}

	sample, surplusW := c.collectMeterSample()
	c.warnSensorDrift(sample)

	for _, ch := range chargers {
		if !ch.Online {
			continue
		}
		c.processCharger(ctx, ch, sample, surplusW)
	}
}

func (c *Controller) collectMeterSample() (smartcharging.MeterSample, float64) {
	gridW := c.energySource.GetGridPowerW()

	solarW := 0.0
	if c.energySource.IsSolarAvailable(c.staleTimeout) {
		solarW = c.energySource.GetSolarPowerW()
	}

	consumptionW := 0.0
	if c.energySource.IsConsumptionAvailable(c.staleTimeout) {
		consumptionW = c.energySource.GetConsumptionPowerW()
	}

	surplusW := solarW - consumptionW
	if surplusW < 0 {
		surplusW = 0
	}

	return smartcharging.MeterSample{
		GridPowerW:        gridW,
		SolarPowerW:       solarW,
		ConsumptionPowerW: consumptionW,
	}, surplusW
}

func (c *Controller) warnSensorDrift(sample smartcharging.MeterSample) {
	if !smartcharging.HasSensorDrift(sample) {
		return
	}
	c.logger.Warn("sensor drift detected — solar/consumption disagree with grid",
		"grid_w", sample.GridPowerW,
		"solar_w", sample.SolarPowerW,
		"consumption_w", sample.ConsumptionPowerW,
		"drift_w", smartcharging.CrossValidationDrift(sample),
	)
}

func (c *Controller) processCharger(ctx context.Context, ch charger.Charger, sample smartcharging.MeterSample, surplusW float64) {
	conns, err := c.chargerRepo.ListConnectors(ctx, ch.ID)
	if err != nil {
		c.logger.Error("failed to list connectors", "err", err, "charger", ch.ID)
		return
	}

	for _, conn := range conns {
		if conn.Status != "Charging" && conn.Status != "SuspendedEVSE" {
			continue
		}
		c.processConnector(ch, conn, sample, surplusW)
	}
}

func (c *Controller) processConnector(ch charger.Charger, conn charger.Connector, sample smartcharging.MeterSample, surplusW float64) {
	result := c.calc.Compute(ch.ID, sample)

	key := fmt.Sprintf("%s:%d", ch.ID, conn.ConnectorID)
	if prev, loaded := c.lastShouldStop.Load(key); !loaded || prev.(bool) != result.ShouldStop {
		c.lastShouldStop.Store(key, result.ShouldStop)
		limitAmps := result.LimitAmps
		if result.ShouldStop {
			limitAmps = int(c.safeAmps.Load())
		}
		c.emit(csms.ChargingProfileUpdated{
			Timestamp:  time.Now(),
			ChargerID:  ch.ID,
			LimitAmps:  limitAmps,
			ShouldStop: result.ShouldStop,
			Reason:     result.Reason,
		})
	}

	if result.ShouldStop {
		c.logger.Info("insufficient surplus — safe state",
			"charger", ch.ID,
			"connector", conn.ConnectorID,
			"limit", c.safeAmps.Load(),
		)
		if err := c.commander.SetChargingProfile(ch.ID, conn.ConnectorID, int(c.safeAmps.Load())); err != nil {
			c.logger.Error("failed to set safe profile", "err", err, "charger", ch.ID)
		} else if c.publisher != nil {
			c.publisher.PublishChargerCurrent(ch.ID, int(c.safeAmps.Load()))
		}
		return
	}

	c.logger.Debug("adjusting charging profile",
		"charger", ch.ID,
		"connector", conn.ConnectorID,
		"limit_amps", result.LimitAmps,
		"reason", result.Reason,
		"grid_w", sample.GridPowerW,
		"solar_w", sample.SolarPowerW,
		"consumption_w", sample.ConsumptionPowerW,
	)

	if prev, ok := c.lastSetAmps.Load(key); ok && abs(result.LimitAmps-prev.(int)) < 1 {
		c.logger.Debug("skipping SetChargingProfile — delta < 1A",
			"charger", ch.ID,
			"connector", conn.ConnectorID,
			"previous", prev,
			"requested", result.LimitAmps,
		)
		return
	}

	if err := c.commander.SetChargingProfile(ch.ID, conn.ConnectorID, result.LimitAmps); err != nil {
		c.logger.Error("failed to set charging profile",
			"err", err,
			"charger", ch.ID,
			"connector", conn.ConnectorID,
		)
		return
	}

	c.lastSetAmps.Store(key, result.LimitAmps)
	if c.publisher != nil {
		c.publisher.PublishChargerCurrent(ch.ID, result.LimitAmps)
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func (c *Controller) revertAllToSafe(ctx context.Context) {
	chargers, err := c.chargerRepo.ListChargers(ctx)
	if err != nil {
		c.logger.Error("failed to list chargers for safe revert", "err", err)
		return
	}

	for _, ch := range chargers {
		if !ch.Online {
			continue
		}
		conns, err := c.chargerRepo.ListConnectors(ctx, ch.ID)
		if err != nil {
			c.logger.Error("failed to list connectors for safe revert", "err", err, "charger", ch.ID)
			continue
		}
		for _, conn := range conns {
			if conn.Status != "Charging" && conn.Status != "SuspendedEVSE" {
				continue
			}
			if err := c.commander.SetChargingProfile(ch.ID, conn.ConnectorID, int(c.safeAmps.Load())); err != nil {
				c.logger.Error("failed to set safe profile (stale revert)",
					"err", err, "charger", ch.ID, "connector", conn.ConnectorID)
			} else if c.publisher != nil {
				c.publisher.PublishChargerCurrent(ch.ID, int(c.safeAmps.Load()))
			}
		}
	}
}