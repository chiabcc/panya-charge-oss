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
	sessionRepo    ports.SessionRepository
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
	manualOverride sync.Map // keyed by chargerID; value is struct{}{}
	solarThreshold    atomic.Int32
	solarGateStopped  sync.Map
	solarBelowTicks   atomic.Int32
	solarAboveTicks   atomic.Int32
}

const (
	solarGateStopTicks  = 3
	solarGateStartTicks = 2
)

func NewController(
	cmd ports.ChargerCommander,
	cr ports.ChargerRepository,
	sr ports.SessionRepository,
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
		sessionRepo:  sr,
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

// State returns a point-in-time snapshot of the controller's runtime state.
func (c *Controller) State() smartcharging.ChargingState {
	gridW := c.energySource.GetGridPowerW()
	solarW := 0.0
	if c.energySource.IsSolarAvailable(c.staleTimeout) {
		solarW = c.energySource.GetSolarPowerW()
	}
	consumptionW := 0.0
	if c.energySource.IsConsumptionAvailable(c.staleTimeout) {
		consumptionW = c.energySource.GetConsumptionPowerW()
	}
	return smartcharging.ChargingState{
		CurrentAmps:  c.safeAmps.Load(),
		GridPowerW:   gridW,
		SolarPowerW:  solarW,
		ConsumptionW: consumptionW,
		Enabled:      c.enabled.Load(),
	}
}

func (c *Controller) SetEmitter(e EventEmitter) {
	c.emitter = e
}

// SetSafeAmps updates the safe amps used as fallback when grid data is stale.
func (c *Controller) SetSafeAmps(amps int) {
	c.safeAmps.Store(int32(amps))
	c.logger.Info("controller: safe amps updated", "safeAmps", amps)
}

// SetManualOverride marks a charger as manually controlled by HA.
// The smart-charging controller will skip this charger in tick() until
// the override is explicitly cleared.
func (c *Controller) SetManualOverride(chargerID string) {
	c.manualOverride.Store(chargerID, struct{}{})
	c.logger.Debug("manual override set", "charger", chargerID)
	if c.publisher != nil {
		c.publisher.PublishChargerMode(chargerID, "manual")
	}
}

// ClearManualOverride removes the manual override for a single charger.
func (c *Controller) ClearManualOverride(chargerID string) {
	c.manualOverride.Delete(chargerID)
	c.logger.Debug("manual override cleared", "charger", chargerID)
	if c.publisher != nil {
		c.publisher.PublishChargerMode(chargerID, "auto")
	}
}

// ClearAllManualOverrides clears all per-charger overrides.
func (c *Controller) ClearAllManualOverrides() {
	if c.publisher != nil {
		c.manualOverride.Range(func(key, _ any) bool {
			c.publisher.PublishChargerMode(key.(string), "auto")
			return true
		})
	}
	c.manualOverride.Range(func(key, _ any) bool {
		c.manualOverride.Delete(key)
		return true
	})
	c.logger.Info("all manual overrides cleared")
}

// IsManualOverride reports whether a charger is under manual override.
func (c *Controller) IsManualOverride(chargerID string) bool {
	_, ok := c.manualOverride.Load(chargerID)
	return ok
}

// LastSetAmps returns the last current limit (A) sent to a connector, or 0 if
// the controller has not yet sent a profile for it.
func (c *Controller) LastSetAmps(chargerID string, connectorID int) int {
	key := fmt.Sprintf("%s:%d", chargerID, connectorID)
	if prev, ok := c.lastSetAmps.Load(key); ok {
		return prev.(int)
	}
	return 0
}

// SetSolarThreshold sets the solar production threshold (Watts).
// Values <= 0 disable the solar gate (always allow charging).
func (c *Controller) SetSolarThreshold(watts int) {
	c.solarThreshold.Store(int32(watts))
}

// GetSolarThreshold returns the current solar production threshold in Watts.
func (c *Controller) GetSolarThreshold() int {
	return int(c.solarThreshold.Load())
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

	// Solar gate: if enabled (threshold > 0), check solar production before
	// running normal surplus calculation. Gate runs before processCharger.
	if throttleProcess := c.runSolarGate(ctx, chargers); throttleProcess {
		return
	}

	sample, _ := c.collectMeterSample()
	c.warnSensorDrift(sample)

	for _, ch := range chargers {
		if !ch.Online {
			continue
		}
		c.processCharger(ctx, ch, sample)
	}
}

func (c *Controller) runSolarGate(ctx context.Context, chargers []charger.Charger) bool {
	threshold := int(c.solarThreshold.Load())
	if threshold <= 0 {
		c.solarBelowTicks.Store(0)
		c.solarAboveTicks.Store(0)
		return false
	}

	if !c.energySource.IsSolarAvailable(c.staleTimeout) {
		return false
	}

	solarW := c.energySource.GetSolarPowerW()

	for _, ch := range chargers {
		if !ch.Online {
			c.solarGateStopped.Delete(ch.ID)
		}
	}

	if solarW < float64(threshold) {
		c.solarAboveTicks.Store(0)
		below := c.solarBelowTicks.Add(1)

		if below < int32(solarGateStopTicks) {
			c.logger.Debug("solar below threshold — debouncing stop",
				"solar_w", solarW, "threshold_w", threshold,
				"tick", below, "need", solarGateStopTicks)
			return true
		}
		if below > int32(solarGateStopTicks) {
			return true
		}

		for _, ch := range chargers {
			if !ch.Online || c.IsManualOverride(ch.ID) {
				continue
			}
			if _, loaded := c.solarGateStopped.LoadOrStore(ch.ID, struct{}{}); loaded {
				continue
			}
			conns, err := c.chargerRepo.ListConnectors(ctx, ch.ID)
			if err != nil {
				continue
			}
			stopped := false
			for _, conn := range conns {
				if conn.ConnectorID == 0 {
					continue
				}
				active, err := c.sessionRepo.GetActiveSession(ctx, ch.ID, conn.ConnectorID)
				if err != nil || active == nil {
					continue
				}
				_ = c.commander.RemoteStopTransaction(ch.ID, active.TransactionID)
				stopped = true
			}
			if stopped {
				c.logger.Info("solar production below threshold — stopping charge",
					"charger", ch.ID, "solar_w", solarW, "threshold_w", threshold)
			}
		}
		return true
	}

	c.solarBelowTicks.Store(0)
	above := c.solarAboveTicks.Add(1)

	if above < int32(solarGateStartTicks) {
		c.logger.Debug("solar above threshold — debouncing restart",
			"solar_w", solarW, "threshold_w", threshold,
			"tick", above, "need", solarGateStartTicks)
		return true
	}
	if above > int32(solarGateStartTicks) {
		return false
	}

	for _, ch := range chargers {
		if !ch.Online {
			c.solarGateStopped.Delete(ch.ID)
			continue
		}
		if _, loaded := c.solarGateStopped.LoadAndDelete(ch.ID); !loaded {
			continue
		}
		conns, err := c.chargerRepo.ListConnectors(ctx, ch.ID)
		if err != nil {
			continue
		}
		for _, conn := range conns {
			if conn.ConnectorID == 0 {
				continue
			}
			_ = c.commander.RemoteStartTransaction(ch.ID, conn.ConnectorID, "solar")
		}
		c.logger.Info("solar production sufficient — resuming charge",
			"charger", ch.ID, "solar_w", solarW, "threshold_w", threshold)
	}

	return false
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

func (c *Controller) processCharger(ctx context.Context, ch charger.Charger, sample smartcharging.MeterSample) {
	if c.IsManualOverride(ch.ID) {
		return
	}
	conns, err := c.chargerRepo.ListConnectors(ctx, ch.ID)
	if err != nil {
		c.logger.Error("failed to list connectors", "err", err, "charger", ch.ID)
		return
	}

	for _, conn := range conns {
		if conn.Status != "Charging" && conn.Status != "SuspendedEVSE" {
			continue
		}
		c.processConnector(ch, conn, sample)
	}
}

func (c *Controller) processConnector(ch charger.Charger, conn charger.Connector, sample smartcharging.MeterSample) {
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
		safeAmps := int(c.safeAmps.Load())
		if prev, ok := c.lastSetAmps.Load(key); ok && abs(safeAmps-prev.(int)) < 1 {
			c.logger.Debug("skipping safe SetChargingProfile — unchanged",
				"charger", ch.ID,
				"connector", conn.ConnectorID,
				"safeAmps", safeAmps,
			)
			return
		}
		c.logger.Info("insufficient surplus — safe state",
			"charger", ch.ID,
			"connector", conn.ConnectorID,
			"limit", c.safeAmps.Load(),
		)
		if err := c.commander.SetChargingProfile(ch.ID, conn.ConnectorID, safeAmps); err != nil {
			c.logger.Error("failed to set safe profile", "err", err, "charger", ch.ID)
		} else {
			c.lastSetAmps.Store(key, safeAmps)
			if c.publisher != nil {
				c.publisher.PublishChargerCurrent(ch.ID, safeAmps)
			}
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
	safeAmps := int(c.safeAmps.Load())
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
			key := fmt.Sprintf("%s:%d", ch.ID, conn.ConnectorID)
			if prev, ok := c.lastSetAmps.Load(key); ok && abs(safeAmps-prev.(int)) < 1 {
				continue
			}
			if err := c.commander.SetChargingProfile(ch.ID, conn.ConnectorID, safeAmps); err != nil {
				c.logger.Error("failed to set safe profile (stale revert)",
					"err", err, "charger", ch.ID, "connector", conn.ConnectorID)
			} else {
				c.lastSetAmps.Store(key, safeAmps)
				if c.publisher != nil {
					c.publisher.PublishChargerCurrent(ch.ID, safeAmps)
				}
			}
		}
	}
}