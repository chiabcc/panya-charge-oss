package ocpp

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/chiabcc/panya-charge-oss/internal/domain/charger"
	"github.com/chiabcc/panya-charge-oss/internal/domain/smartcharging"
	"github.com/chiabcc/panya-charge-oss/internal/testutil/mocks"
)

func newTestController(t *testing.T, cmd *mocks.MockChargerCommander, cr *mocks.MockChargerRepository, grid *mocks.MockEnergySource, pub *mocks.MockEventPublisher) *Controller {
	t.Helper()
	calc := smartcharging.NewCalculator(6, 32, 230.0)
	return NewController(cmd, cr, grid, pub, calc, 6, time.Hour, 60*time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestController_StaleGridRevertsToSafe(t *testing.T) {
	cmd := mocks.NewMockChargerCommander()
	cr := mocks.NewMockChargerRepository()
	cr.Chargers["CHG-A"] = charger.Charger{ID: "CHG-A", Online: true}
	cr.Chargers["CHG-B"] = charger.Charger{ID: "CHG-B", Online: false}
	cr.Connectors["CHG-A"] = []charger.Connector{
		{ChargerID: "CHG-A", ConnectorID: 1, Status: charger.StatusCharging},
	}
	cr.Connectors["CHG-B"] = []charger.Connector{
		{ChargerID: "CHG-B", ConnectorID: 1, Status: charger.StatusCharging},
	}
	grid := mocks.NewMockEnergySource()
	grid.Stale = true
	pub := mocks.NewMockEventPublisher()

	ctrl := newTestController(t, cmd, cr, grid, pub)
	ctrl.tick(context.Background())

	if len(cmd.SetChargingProfileCalls) != 1 {
		t.Fatalf("SetChargingProfileCalls = %d, want 1", len(cmd.SetChargingProfileCalls))
	}
	call := cmd.SetChargingProfileCalls[0]
	if call.ChargerID != "CHG-A" {
		t.Errorf("SetChargingProfile(%s, %d, %d), want ChargerID = CHG-A", call.ChargerID, call.ConnectorID, call.LimitAmps)
	}
	if call.ConnectorID != 1 {
		t.Errorf("SetChargingProfile(%s, %d, %d), want ConnectorID = 1", call.ChargerID, call.ConnectorID, call.LimitAmps)
	}
	if call.LimitAmps != 6 {
		t.Errorf("SetChargingProfile(%s, %d, %d), want LimitAmps = 6", call.ChargerID, call.ConnectorID, call.LimitAmps)
	}
}

func TestController_SolarSurplusAdjusts(t *testing.T) {
	cmd := mocks.NewMockChargerCommander()
	cr := mocks.NewMockChargerRepository()
	cr.Chargers["CHG-001"] = charger.Charger{ID: "CHG-001", Online: true}
	cr.Connectors["CHG-001"] = []charger.Connector{
		{ChargerID: "CHG-001", ConnectorID: 1, Status: charger.StatusCharging},
	}
	grid := mocks.NewMockEnergySource()
	grid.GridPowerW = -4000
	grid.Stale = false
	pub := mocks.NewMockEventPublisher()

	ctrl := newTestController(t, cmd, cr, grid, pub)
	ctrl.tick(context.Background())

	if len(cmd.SetChargingProfileCalls) != 1 {
		t.Fatalf("SetChargingProfileCalls = %d, want 1", len(cmd.SetChargingProfileCalls))
	}
	call := cmd.SetChargingProfileCalls[0]
	if call.LimitAmps != 17 {
		t.Errorf("SetChargingProfile(%s, %d, %d), want LimitAmps = 17", call.ChargerID, call.ConnectorID, call.LimitAmps)
	}

	if len(pub.CurrentPublished) != 1 {
		t.Fatalf("CurrentPublished = %d, want 1", len(pub.CurrentPublished))
	}
	if pub.CurrentPublished[0].Amps != 17 {
		t.Errorf("PublishChargerCurrent(%s, %d), want Amps = 17", pub.CurrentPublished[0].ChargerID, pub.CurrentPublished[0].Amps)
	}
}

func TestController_InsufficientSurplusFallsToSafe(t *testing.T) {
	cmd := mocks.NewMockChargerCommander()
	cr := mocks.NewMockChargerRepository()
	cr.Chargers["CHG-001"] = charger.Charger{ID: "CHG-001", Online: true}
	cr.Connectors["CHG-001"] = []charger.Connector{
		{ChargerID: "CHG-001", ConnectorID: 1, Status: charger.StatusCharging},
	}
	grid := mocks.NewMockEnergySource()
	grid.GridPowerW = 500
	grid.Stale = false
	pub := mocks.NewMockEventPublisher()

	ctrl := newTestController(t, cmd, cr, grid, pub)
	ctrl.tick(context.Background())

	if len(cmd.SetChargingProfileCalls) != 1 {
		t.Fatalf("SetChargingProfileCalls = %d, want 1", len(cmd.SetChargingProfileCalls))
	}
	call := cmd.SetChargingProfileCalls[0]
	if call.LimitAmps != 6 {
		t.Errorf("SetChargingProfile(%s, %d, %d), want LimitAmps = 6", call.ChargerID, call.ConnectorID, call.LimitAmps)
	}
}

func TestController_OfflineChargerSkipped(t *testing.T) {
	cmd := mocks.NewMockChargerCommander()
	cr := mocks.NewMockChargerRepository()
	cr.Chargers["CHG-001"] = charger.Charger{ID: "CHG-001", Online: false}
	cr.Connectors["CHG-001"] = []charger.Connector{
		{ChargerID: "CHG-001", ConnectorID: 1, Status: charger.StatusCharging},
	}
	grid := mocks.NewMockEnergySource()
	grid.GridPowerW = -4000
	grid.Stale = false
	pub := mocks.NewMockEventPublisher()

	ctrl := newTestController(t, cmd, cr, grid, pub)
	ctrl.tick(context.Background())

	if len(cmd.SetChargingProfileCalls) != 0 {
		t.Errorf("SetChargingProfileCalls = %d, want 0", len(cmd.SetChargingProfileCalls))
	}
}

func TestController_NonChargingConnectorSkipped(t *testing.T) {
	cmd := mocks.NewMockChargerCommander()
	cr := mocks.NewMockChargerRepository()
	cr.Chargers["CHG-001"] = charger.Charger{ID: "CHG-001", Online: true}
	cr.Connectors["CHG-001"] = []charger.Connector{
		{ChargerID: "CHG-001", ConnectorID: 1, Status: charger.StatusAvailable},
	}
	grid := mocks.NewMockEnergySource()
	grid.GridPowerW = -4000
	grid.Stale = false
	pub := mocks.NewMockEventPublisher()

	ctrl := newTestController(t, cmd, cr, grid, pub)
	ctrl.tick(context.Background())

	if len(cmd.SetChargingProfileCalls) != 0 {
		t.Errorf("SetChargingProfileCalls = %d, want 0", len(cmd.SetChargingProfileCalls))
	}
}

func TestController_SuspendedEVSEProcessed(t *testing.T) {
	cmd := mocks.NewMockChargerCommander()
	cr := mocks.NewMockChargerRepository()
	cr.Chargers["CHG-001"] = charger.Charger{ID: "CHG-001", Online: true}
	cr.Connectors["CHG-001"] = []charger.Connector{
		{ChargerID: "CHG-001", ConnectorID: 1, Status: charger.StatusSuspendedEVSE},
	}
	grid := mocks.NewMockEnergySource()
	grid.GridPowerW = -4000
	grid.Stale = false
	pub := mocks.NewMockEventPublisher()

	ctrl := newTestController(t, cmd, cr, grid, pub)
	ctrl.tick(context.Background())

	if len(cmd.SetChargingProfileCalls) != 1 {
		t.Fatalf("SetChargingProfileCalls = %d, want 1", len(cmd.SetChargingProfileCalls))
	}
	call := cmd.SetChargingProfileCalls[0]
	if call.ChargerID != "CHG-001" {
		t.Errorf("SetChargingProfile(%s, %d, %d), want ChargerID = CHG-001", call.ChargerID, call.ConnectorID, call.LimitAmps)
	}
	if call.ConnectorID != 1 {
		t.Errorf("SetChargingProfile(%s, %d, %d), want ConnectorID = 1", call.ChargerID, call.ConnectorID, call.LimitAmps)
	}
	if call.LimitAmps != 17 {
		t.Errorf("SetChargingProfile(%s, %d, %d), want LimitAmps = 17", call.ChargerID, call.ConnectorID, call.LimitAmps)
	}
}

func TestController_DebounceSkipsUnchangedAmps(t *testing.T) {
	cmd := mocks.NewMockChargerCommander()
	cr := mocks.NewMockChargerRepository()
	cr.Chargers["CHG-001"] = charger.Charger{ID: "CHG-001", Online: true}
	cr.Connectors["CHG-001"] = []charger.Connector{
		{ChargerID: "CHG-001", ConnectorID: 1, Status: charger.StatusCharging},
	}
	grid := mocks.NewMockEnergySource()
	grid.GridPowerW = -4000 // produces 17A from calculator
	grid.Stale = false
	pub := mocks.NewMockEventPublisher()

	ctrl := newTestController(t, cmd, cr, grid, pub)

	// First tick — should set profile
	ctrl.tick(context.Background())
	if len(cmd.SetChargingProfileCalls) != 1 {
		t.Fatalf("first tick: SetChargingProfileCalls = %d, want 1", len(cmd.SetChargingProfileCalls))
	}

	// Second tick — same grid power, same amps — should be debounced
	ctrl.tick(context.Background())
	if len(cmd.SetChargingProfileCalls) != 1 {
		t.Fatalf("second tick: SetChargingProfileCalls = %d, want 1 (debounced)", len(cmd.SetChargingProfileCalls))
	}

	// Change grid power to produce different amps — should fire
	grid.GridPowerW = -5000
	ctrl.tick(context.Background())
	if len(cmd.SetChargingProfileCalls) != 2 {
		t.Fatalf("third tick after change: SetChargingProfileCalls = %d, want 2", len(cmd.SetChargingProfileCalls))
	}
}

func TestController_MultiSourceSurplus(t *testing.T) {
	cmd := mocks.NewMockChargerCommander()
	cr := mocks.NewMockChargerRepository()
	cr.Chargers["CHG-001"] = charger.Charger{ID: "CHG-001", Online: true}
	cr.Connectors["CHG-001"] = []charger.Connector{
		{ChargerID: "CHG-001", ConnectorID: 1, Status: charger.StatusCharging},
	}
	grid := mocks.NewMockEnergySource()
	grid.SolarPowerW = 6000
	grid.ConsumptionPowerW = 1500
	grid.SolarAvail = true
	grid.ConsumptionAvail = true
	grid.Stale = false
	pub := mocks.NewMockEventPublisher()

	ctrl := newTestController(t, cmd, cr, grid, pub)
	ctrl.tick(context.Background())

	if len(cmd.SetChargingProfileCalls) != 1 {
		t.Fatalf("SetChargingProfileCalls = %d, want 1", len(cmd.SetChargingProfileCalls))
	}
	call := cmd.SetChargingProfileCalls[0]
	if call.LimitAmps != 19 {
		t.Errorf("SetChargingProfile(%s, %d, %d), want LimitAmps = 19 (4500/230)", call.ChargerID, call.ConnectorID, call.LimitAmps)
	}
}

func TestController_SolarNotAvailable_GridFallback(t *testing.T) {
	cmd := mocks.NewMockChargerCommander()
	cr := mocks.NewMockChargerRepository()
	cr.Chargers["CHG-001"] = charger.Charger{ID: "CHG-001", Online: true}
	cr.Connectors["CHG-001"] = []charger.Connector{
		{ChargerID: "CHG-001", ConnectorID: 1, Status: charger.StatusCharging},
	}
	grid := mocks.NewMockEnergySource()
	grid.GridPowerW = -4000
	grid.SolarAvail = false
	grid.ConsumptionAvail = false
	grid.Stale = false
	pub := mocks.NewMockEventPublisher()

	ctrl := newTestController(t, cmd, cr, grid, pub)
	ctrl.tick(context.Background())

	if len(cmd.SetChargingProfileCalls) != 1 {
		t.Fatalf("SetChargingProfileCalls = %d, want 1", len(cmd.SetChargingProfileCalls))
	}
	call := cmd.SetChargingProfileCalls[0]
	if call.LimitAmps != 17 {
		t.Errorf("SetChargingProfile(%s, %d, %d), want LimitAmps = 17 (grid fallback 4000/230)", call.ChargerID, call.ConnectorID, call.LimitAmps)
	}
}

func TestController_SensorDriftDoesNotCrash(t *testing.T) {
	cmd := mocks.NewMockChargerCommander()
	cr := mocks.NewMockChargerRepository()
	cr.Chargers["CHG-001"] = charger.Charger{ID: "CHG-001", Online: true}
	cr.Connectors["CHG-001"] = []charger.Connector{
		{ChargerID: "CHG-001", ConnectorID: 1, Status: charger.StatusCharging},
	}
	grid := mocks.NewMockEnergySource()
	grid.SolarPowerW = 8000
	grid.ConsumptionPowerW = 2000
	grid.GridPowerW = 5000
	grid.SolarAvail = true
	grid.ConsumptionAvail = true
	grid.Stale = false
	pub := mocks.NewMockEventPublisher()

	ctrl := newTestController(t, cmd, cr, grid, pub)

	ctrl.tick(context.Background())

	if len(cmd.SetChargingProfileCalls) != 1 {
		t.Fatalf("SetChargingProfileCalls = %d, want 1 (drift warning should not block)", len(cmd.SetChargingProfileCalls))
	}
}

func TestController_SetSafeAmps_RaceFree(t *testing.T) {
	cmd := mocks.NewMockChargerCommander()
	cr := mocks.NewMockChargerRepository()
	cr.Chargers["CHG-001"] = charger.Charger{ID: "CHG-001", Online: true}
	cr.Connectors["CHG-001"] = []charger.Connector{
		{ChargerID: "CHG-001", ConnectorID: 1, Status: charger.StatusCharging},
	}
	grid := mocks.NewMockEnergySource()
	grid.Stale = true
	pub := mocks.NewMockEventPublisher()

	ctrl := newTestController(t, cmd, cr, grid, pub)
	done := make(chan struct{})

	go func() {
		defer close(done)
		for i := 0; i < 100; i++ {
			ctrl.tick(context.Background())
		}
	}()

	for i := 0; i < 100; i++ {
		ctrl.SetSafeAmps(6 + i%7)
	}

	<-done
}

func TestController_ManualOverrideSkipsCharger(t *testing.T) {
	cmd := mocks.NewMockChargerCommander()
	cr := mocks.NewMockChargerRepository()
	cr.Chargers["CHG-001"] = charger.Charger{ID: "CHG-001", Online: true}
	cr.Connectors["CHG-001"] = []charger.Connector{
		{ChargerID: "CHG-001", ConnectorID: 1, Status: charger.StatusCharging},
	}
	grid := mocks.NewMockEnergySource()
	grid.GridPowerW = -4000
	grid.Stale = false
	pub := mocks.NewMockEventPublisher()

	ctrl := newTestController(t, cmd, cr, grid, pub)

	// First tick — normal, should fire
	ctrl.tick(context.Background())
	if len(cmd.SetChargingProfileCalls) != 1 {
		t.Fatalf("first tick: SetChargingProfileCalls = %d, want 1", len(cmd.SetChargingProfileCalls))
	}

	// Set manual override
	ctrl.SetManualOverride("CHG-001")

	// Second tick — overridden, should skip
	cmd.Reset()
	ctrl.tick(context.Background())
	if len(cmd.SetChargingProfileCalls) != 0 {
		t.Errorf("overridden tick: SetChargingProfileCalls = %d, want 0", len(cmd.SetChargingProfileCalls))
	}
}

func TestController_ManualOverrideClearedAllowsProcessing(t *testing.T) {
	cmd := mocks.NewMockChargerCommander()
	cr := mocks.NewMockChargerRepository()
	cr.Chargers["CHG-001"] = charger.Charger{ID: "CHG-001", Online: true}
	cr.Connectors["CHG-001"] = []charger.Connector{
		{ChargerID: "CHG-001", ConnectorID: 1, Status: charger.StatusCharging},
	}
	grid := mocks.NewMockEnergySource()
	grid.GridPowerW = -4000
	grid.Stale = false
	pub := mocks.NewMockEventPublisher()

	ctrl := newTestController(t, cmd, cr, grid, pub)

	ctrl.SetManualOverride("CHG-001")
	ctrl.tick(context.Background())
	if len(cmd.SetChargingProfileCalls) != 0 {
		t.Errorf("overridden tick: should have been skipped")
	}

	// Clear override
	ctrl.ClearManualOverride("CHG-001")

	// Should now process
	cmd.Reset()
	ctrl.tick(context.Background())
	if len(cmd.SetChargingProfileCalls) != 1 {
		t.Fatalf("after clear: SetChargingProfileCalls = %d, want 1", len(cmd.SetChargingProfileCalls))
	}
}

func TestController_ManualOverrideClearAll(t *testing.T) {
	ctrl := newTestController(t,
		mocks.NewMockChargerCommander(),
		mocks.NewMockChargerRepository(),
		mocks.NewMockEnergySource(),
		mocks.NewMockEventPublisher(),
	)

	ctrl.SetManualOverride("CHG-A")
	ctrl.SetManualOverride("CHG-B")

	if !ctrl.IsManualOverride("CHG-A") {
		t.Error("CHG-A should have override set")
	}
	if !ctrl.IsManualOverride("CHG-B") {
		t.Error("CHG-B should have override set")
	}

	ctrl.ClearAllManualOverrides()

	if ctrl.IsManualOverride("CHG-A") {
		t.Error("CHG-A override should be cleared")
	}
	if ctrl.IsManualOverride("CHG-B") {
		t.Error("CHG-B override should be cleared")
	}
}

func TestController_ManualOverridePerCharger(t *testing.T) {
	cmd := mocks.NewMockChargerCommander()
	cr := mocks.NewMockChargerRepository()
	cr.Chargers["CHG-A"] = charger.Charger{ID: "CHG-A", Online: true}
	cr.Chargers["CHG-B"] = charger.Charger{ID: "CHG-B", Online: true}
	cr.Connectors["CHG-A"] = []charger.Connector{
		{ChargerID: "CHG-A", ConnectorID: 1, Status: charger.StatusCharging},
	}
	cr.Connectors["CHG-B"] = []charger.Connector{
		{ChargerID: "CHG-B", ConnectorID: 1, Status: charger.StatusCharging},
	}
	grid := mocks.NewMockEnergySource()
	grid.GridPowerW = -4000
	grid.Stale = false
	pub := mocks.NewMockEventPublisher()

	ctrl := newTestController(t, cmd, cr, grid, pub)

	// Override only CHG-A
	ctrl.SetManualOverride("CHG-A")

	ctrl.tick(context.Background())

	// CHG-B should be processed, CHG-A should be skipped
	processed := false
	for _, call := range cmd.SetChargingProfileCalls {
		if call.ChargerID == "CHG-A" {
			t.Error("CHG-A should be skipped but was processed")
		}
		if call.ChargerID == "CHG-B" {
			processed = true
		}
	}
	if !processed {
		t.Error("CHG-B should have been processed but was not")
	}
}

func TestController_SafeStateDedup(t *testing.T) {
	cmd := mocks.NewMockChargerCommander()
	cr := mocks.NewMockChargerRepository()
	cr.Chargers["CHG-001"] = charger.Charger{ID: "CHG-001", Online: true}
	cr.Connectors["CHG-001"] = []charger.Connector{
		{ChargerID: "CHG-001", ConnectorID: 1, Status: charger.StatusCharging},
	}
	grid := mocks.NewMockEnergySource()
	grid.GridPowerW = 500 // positive = no surplus, ShouldStop
	grid.Stale = false
	pub := mocks.NewMockEventPublisher()

	ctrl := newTestController(t, cmd, cr, grid, pub)

	// First tick — ShouldStop fires safe state
	ctrl.tick(context.Background())
	if len(cmd.SetChargingProfileCalls) != 1 {
		t.Fatalf("first tick: SetChargingProfileCalls = %d, want 1", len(cmd.SetChargingProfileCalls))
	}
	if cmd.SetChargingProfileCalls[0].LimitAmps != 6 {
		t.Errorf("first tick: LimitAmps = %d, want 6", cmd.SetChargingProfileCalls[0].LimitAmps)
	}

	// Second tick — same state, should be deduped
	ctrl.tick(context.Background())
	if len(cmd.SetChargingProfileCalls) != 1 {
		t.Fatalf("second tick: SetChargingProfileCalls = %d, want 1 (deduped)", len(cmd.SetChargingProfileCalls))
	}

	// Third tick — same state, still deduped
	ctrl.tick(context.Background())
	if len(cmd.SetChargingProfileCalls) != 1 {
		t.Fatalf("third tick: SetChargingProfileCalls = %d, want 1 (deduped)", len(cmd.SetChargingProfileCalls))
	}
}

func TestController_SafeStateDedupResendsAfterLimitChange(t *testing.T) {
	cmd := mocks.NewMockChargerCommander()
	cr := mocks.NewMockChargerRepository()
	cr.Chargers["CHG-001"] = charger.Charger{ID: "CHG-001", Online: true}
	cr.Connectors["CHG-001"] = []charger.Connector{
		{ChargerID: "CHG-001", ConnectorID: 1, Status: charger.StatusCharging},
	}
	grid := mocks.NewMockEnergySource()
	grid.GridPowerW = 500
	grid.Stale = false
	pub := mocks.NewMockEventPublisher()

	ctrl := newTestController(t, cmd, cr, grid, pub)

	// First tick — calculator in holding state (not yet isStopped), sets 6A via normal path
	ctrl.tick(context.Background())
	if len(cmd.SetChargingProfileCalls) != 1 {
		t.Fatalf("first tick: calls = %d, want 1", len(cmd.SetChargingProfileCalls))
	}

	// Pre-seed lastSetAmps with a higher value so the safe path dedup detects the change.
	// This simulates the scenario where the controller had been running at a higher amp
	// before grid data dropped below threshold.
	ctrl.lastSetAmps.Store("CHG-001:1", 16)

	// Change safe amps to 10 — ShouldStop path should re-fire with the new safe limit
	ctrl.SetSafeAmps(10)

	// Need 2 more ticks to push the calculator into isStopped (> stopConfirmTicks)
	ctrl.tick(context.Background())
	ctrl.tick(context.Background())

	t.Logf("calls=%d safeAmps=%d", len(cmd.SetChargingProfileCalls), ctrl.safeAmps.Load())
	if len(cmd.SetChargingProfileCalls) < 2 {
		t.Fatalf("after safeAmps change: calls = %d, want >= 2 (safe state should re-fire at 10A)", len(cmd.SetChargingProfileCalls))
	}
}

func TestController_RevertAllToSafeDedup(t *testing.T) {
	cmd := mocks.NewMockChargerCommander()
	cr := mocks.NewMockChargerRepository()
	cr.Chargers["CHG-A"] = charger.Charger{ID: "CHG-A", Online: true}
	cr.Chargers["CHG-B"] = charger.Charger{ID: "CHG-B", Online: true}
	cr.Connectors["CHG-A"] = []charger.Connector{
		{ChargerID: "CHG-A", ConnectorID: 1, Status: charger.StatusCharging},
	}
	cr.Connectors["CHG-B"] = []charger.Connector{
		{ChargerID: "CHG-B", ConnectorID: 1, Status: charger.StatusCharging},
	}
	grid := mocks.NewMockEnergySource()
	grid.Stale = true
	pub := mocks.NewMockEventPublisher()

	ctrl := newTestController(t, cmd, cr, grid, pub)

	// First stale tick — both chargers should get safe state
	ctrl.tick(context.Background())
	if len(cmd.SetChargingProfileCalls) != 2 {
		t.Fatalf("first stale tick: calls = %d, want 2", len(cmd.SetChargingProfileCalls))
	}

	// Second stale tick — deduped, should be 0 new calls
	ctrl.tick(context.Background())
	if len(cmd.SetChargingProfileCalls) != 2 {
		t.Fatalf("second stale tick: calls = %d, want 2 (deduped)", len(cmd.SetChargingProfileCalls))
	}
}

func TestController_RevertAllToSafeFiresForOverrideChargers(t *testing.T) {
	cmd := mocks.NewMockChargerCommander()
	cr := mocks.NewMockChargerRepository()
	cr.Chargers["CHG-OVR"] = charger.Charger{ID: "CHG-OVR", Online: true}
	cr.Connectors["CHG-OVR"] = []charger.Connector{
		{ChargerID: "CHG-OVR", ConnectorID: 1, Status: charger.StatusCharging},
	}
	grid := mocks.NewMockEnergySource()
	grid.Stale = false
	pub := mocks.NewMockEventPublisher()

	ctrl := newTestController(t, cmd, cr, grid, pub)

	// Set override
	ctrl.SetManualOverride("CHG-OVR")

	// Normal tick with override — should skip
	ctrl.tick(context.Background())
	if len(cmd.SetChargingProfileCalls) != 0 {
		t.Errorf("normal tick with override: calls = %d, want 0", len(cmd.SetChargingProfileCalls))
	}

	// Stale grid — should NOT be blocked by override (safety path)
	grid.Stale = true
	ctrl.tick(context.Background())
	if len(cmd.SetChargingProfileCalls) != 1 {
		t.Fatalf("stale tick with override: calls = %d, want 1 (safety override)", len(cmd.SetChargingProfileCalls))
	}
}

func TestController_RevertAllToSafeDedupStillFiresOnAmpsChange(t *testing.T) {
	cmd := mocks.NewMockChargerCommander()
	cr := mocks.NewMockChargerRepository()
	cr.Chargers["CHG-A"] = charger.Charger{ID: "CHG-A", Online: true}
	cr.Connectors["CHG-A"] = []charger.Connector{
		{ChargerID: "CHG-A", ConnectorID: 1, Status: charger.StatusCharging},
	}
	grid := mocks.NewMockEnergySource()
	grid.Stale = true
	pub := mocks.NewMockEventPublisher()

	ctrl := newTestController(t, cmd, cr, grid, pub)

	// First stale tick — 6A safe state
	ctrl.tick(context.Background())
	if len(cmd.SetChargingProfileCalls) != 1 {
		t.Fatalf("first: calls = %d, want 1", len(cmd.SetChargingProfileCalls))
	}

	// Second stale tick — deduped
	ctrl.tick(context.Background())
	if len(cmd.SetChargingProfileCalls) != 1 {
		t.Fatalf("second: calls = %d, want 1 (deduped)", len(cmd.SetChargingProfileCalls))
	}

	// Change safe amps and tick again — should re-fire
	ctrl.SetSafeAmps(10)
	ctrl.tick(context.Background())
	if len(cmd.SetChargingProfileCalls) != 2 {
		t.Fatalf("after safeAmps change: calls = %d, want 2", len(cmd.SetChargingProfileCalls))
	}
	if cmd.SetChargingProfileCalls[1].LimitAmps != 10 {
		t.Errorf("second call: LimitAmps = %d, want 10", cmd.SetChargingProfileCalls[1].LimitAmps)
	}
}

func TestController_ManualOverrideConcurrent(t *testing.T) {
	cmd := mocks.NewMockChargerCommander()
	cr := mocks.NewMockChargerRepository()
	cr.Chargers["CHG-A"] = charger.Charger{ID: "CHG-A", Online: true}
	cr.Connectors["CHG-A"] = []charger.Connector{
		{ChargerID: "CHG-A", ConnectorID: 1, Status: charger.StatusCharging},
	}
	grid := mocks.NewMockEnergySource()
	grid.Stale = true
	pub := mocks.NewMockEventPublisher()

	ctrl := newTestController(t, cmd, cr, grid, pub)
	done := make(chan struct{})

	go func() {
		defer close(done)
		for i := 0; i < 100; i++ {
			ctrl.SetManualOverride("CHG-A")
			ctrl.ClearManualOverride("CHG-A")
			ctrl.ClearAllManualOverrides()
		}
	}()

	for i := 0; i < 100; i++ {
		ctrl.tick(context.Background())
	}

	<-done
}
