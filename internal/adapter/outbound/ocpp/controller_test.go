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
