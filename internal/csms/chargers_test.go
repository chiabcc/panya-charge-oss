package csms

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/chiabcc/panya-charge-oss/internal/adapter/outbound/ocpp"
	"github.com/chiabcc/panya-charge-oss/internal/domain/charger"
	"github.com/chiabcc/panya-charge-oss/internal/domain/ports"
	"github.com/chiabcc/panya-charge-oss/internal/domain/session"
	"github.com/chiabcc/panya-charge-oss/internal/domain/smartcharging"
	"github.com/chiabcc/panya-charge-oss/internal/testutil/mocks"
)

func TestChargers_PopulatesPowerAndLimit(t *testing.T) {
	chargerRepo := ports.NewInMemoryChargerRepository()
	sessionRepo := ports.NewInMemorySessionRepository()
	meterRepo := ports.NewInMemoryMeterRepository()

	ctx := context.Background()
	must(t, chargerRepo.UpsertCharger(ctx, charger.Charger{ID: "CHG-001", Online: true, Vendor: "ABB", Model: "Terra AC"}))
	must(t, chargerRepo.UpsertConnector(ctx, charger.Connector{ChargerID: "CHG-001", ConnectorID: 1, Status: charger.StatusCharging}))
	must(t, sessionRepo.CreateSession(ctx, session.Session{
		ID:            "sess-1",
		TransactionID: 42,
		ChargerID:     "CHG-001",
		ConnectorID:   1,
		StartedAt:     time.Now(),
	}))

	now := time.Now()
	must(t, meterRepo.StoreMeterValue(ctx, ports.MeterValue{
		ChargerID: "CHG-001", ConnectorID: 1, SessionID: "sess-1",
		Timestamp: now.Add(-30 * time.Second), Measurand: "Power.Active.Import", Value: 5000, Unit: "W",
	}))
	must(t, meterRepo.StoreMeterValue(ctx, ports.MeterValue{
		ChargerID: "CHG-001", ConnectorID: 1, SessionID: "sess-1",
		Timestamp: now.Add(-5 * time.Second), Measurand: "Power.Active.Import", Value: 7300, Unit: "W",
	}))

	cmd := mocks.NewMockChargerCommander()
	grid := mocks.NewMockEnergySource()
	grid.GridPowerW = -4000
	grid.Stale = false
	pub := mocks.NewMockEventPublisher()
	calc := smartcharging.NewCalculator(6, 32, 230.0)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	controller := ocpp.NewController(cmd, chargerRepo, grid, pub, calc, 6, 10*time.Millisecond, 60*time.Second, logger)

	runCtx, runCancel := context.WithTimeout(ctx, 100*time.Millisecond)
	controller.Run(runCtx)
	runCancel()

	c := &CSMS{
		chargerRepo: chargerRepo,
		sessionRepo: sessionRepo,
		meterRepo:   meterRepo,
		controller:  controller,
		logger:      logger,
	}

	infos := c.Chargers()
	if len(infos) != 1 {
		t.Fatalf("Chargers() = %d items, want 1", len(infos))
	}
	info := infos[0]
	if info.ID != "CHG-001" {
		t.Errorf("ID = %s, want CHG-001", info.ID)
	}
	if info.Status != string(charger.StatusCharging) {
		t.Errorf("Status = %s, want Charging", info.Status)
	}
	if info.TxID != 42 {
		t.Errorf("TxID = %d, want 42", info.TxID)
	}
	if info.LimitAmps == 0 {
		t.Error("LimitAmps = 0, want non-zero (controller should have set a limit via tick)")
	}
	if info.ChargingPower != 7300 {
		t.Errorf("ChargingPower = %.0f, want 7300 (latest Power.Active.Import)", info.ChargingPower)
	}
}

func TestChargers_NoMeterData_PowerZero(t *testing.T) {
	chargerRepo := ports.NewInMemoryChargerRepository()
	sessionRepo := ports.NewInMemorySessionRepository()
	meterRepo := ports.NewInMemoryMeterRepository()

	ctx := context.Background()
	must(t, chargerRepo.UpsertCharger(ctx, charger.Charger{ID: "CHG-002", Online: true}))
	must(t, chargerRepo.UpsertConnector(ctx, charger.Connector{ChargerID: "CHG-002", ConnectorID: 1, Status: charger.StatusAvailable}))

	c := &CSMS{
		chargerRepo: chargerRepo,
		sessionRepo: sessionRepo,
		meterRepo:   meterRepo,
		controller: ocpp.NewController(
			mocks.NewMockChargerCommander(), chargerRepo,
			mocks.NewMockEnergySource(), mocks.NewMockEventPublisher(),
			smartcharging.NewCalculator(6, 32, 230.0), 6,
			10*time.Millisecond, 60*time.Second,
			slog.New(slog.NewTextHandler(io.Discard, nil)),
		),
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	infos := c.Chargers()
	if len(infos) != 1 {
		t.Fatalf("Chargers() = %d items, want 1", len(infos))
	}
	info := infos[0]
	if info.ChargingPower != 0 {
		t.Errorf("ChargingPower = %.0f, want 0 (no meter data)", info.ChargingPower)
	}
	if info.LimitAmps != 0 {
		t.Errorf("LimitAmps = %d, want 0 (no tick has run)", info.LimitAmps)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
