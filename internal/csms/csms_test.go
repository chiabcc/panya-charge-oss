package csms

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/chiabcc/panya-charge-oss/internal/domain/charger"
	"github.com/chiabcc/panya-charge-oss/internal/domain/session"
	"github.com/chiabcc/panya-charge-oss/internal/testutil/mocks"
)

func newTestCmdBridge(t *testing.T, commander *mocks.MockChargerCommander, cr *mocks.MockChargerRepository, sr *mocks.MockSessionRepository, pub *mocks.MockEventPublisher) *cmdBridge {
	t.Helper()
	return &cmdBridge{
		commander:   commander,
		chargerRepo: cr,
		sessionRepo: sr,
		publisher:   pub,
		logger:      slog.New(slog.NewTextHandler(os.Stderr, nil)),
	}
}

func TestStartCharging_ActiveSession_SkipsRemoteStart(t *testing.T) {
	commander := mocks.NewMockChargerCommander()
	cr := mocks.NewMockChargerRepository()
	cr.Chargers["CHG-001"] = charger.Charger{ID: "CHG-001", Online: true}
	cr.Connectors["CHG-001"] = []charger.Connector{
		{ChargerID: "CHG-001", ConnectorID: 1, Status: charger.StatusCharging},
	}
	sr := mocks.NewMockSessionRepository()
	sr.Sessions["sess-1"] = session.Session{
		ID:            "sess-1",
		TransactionID: 42,
		ChargerID:     "CHG-001",
		ConnectorID:   1,
	}
	pub := mocks.NewMockEventPublisher()

	b := newTestCmdBridge(t, commander, cr, sr, pub)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	b.startCharging(ctx, "CHG-001")

	if len(commander.RemoteStartCalls) != 0 {
		t.Errorf("RemoteStartCalls = %d, want 0 (already charging)", len(commander.RemoteStartCalls))
	}

	if len(pub.ChargingPublished) != 1 || !pub.ChargingPublished[0].Charging {
		t.Errorf("PublishChargingState(true) not called, got %+v", pub.ChargingPublished)
	}
}

func TestStartCharging_NoActiveSession_SendsRemoteStart(t *testing.T) {
	commander := mocks.NewMockChargerCommander()
	cr := mocks.NewMockChargerRepository()
	cr.Chargers["CHG-001"] = charger.Charger{ID: "CHG-001", Online: true}
	cr.Connectors["CHG-001"] = []charger.Connector{
		{ChargerID: "CHG-001", ConnectorID: 1, Status: charger.StatusAvailable},
	}
	sr := mocks.NewMockSessionRepository()
	pub := mocks.NewMockEventPublisher()

	b := newTestCmdBridge(t, commander, cr, sr, pub)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	b.startCharging(ctx, "CHG-001")

	if len(commander.RemoteStartCalls) != 1 {
		t.Fatalf("RemoteStartCalls = %d, want 1", len(commander.RemoteStartCalls))
	}
	call := commander.RemoteStartCalls[0]
	if call.ChargerID != "CHG-001" {
		t.Errorf("RemoteStart ChargerID = %s, want CHG-001", call.ChargerID)
	}
	if call.ConnectorID != 1 {
		t.Errorf("RemoteStart ConnectorID = %d, want 1", call.ConnectorID)
	}
}

func TestStartCharging_ActiveSession_LogsMessage(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))

	commander := mocks.NewMockChargerCommander()
	cr := mocks.NewMockChargerRepository()
	cr.Chargers["CHG-001"] = charger.Charger{ID: "CHG-001", Online: true}
	cr.Connectors["CHG-001"] = []charger.Connector{
		{ChargerID: "CHG-001", ConnectorID: 1, Status: charger.StatusCharging},
	}
	sr := mocks.NewMockSessionRepository()
	sr.Sessions["sess-1"] = session.Session{
		ID:            "sess-1",
		TransactionID: 42,
		ChargerID:     "CHG-001",
		ConnectorID:   1,
	}
	pub := mocks.NewMockEventPublisher()

	b := &cmdBridge{
		commander:   commander,
		chargerRepo: cr,
		sessionRepo: sr,
		publisher:   pub,
		logger:      logger,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	b.startCharging(ctx, "CHG-001")

	logStr := logBuf.String()
	if !contains(logStr, "charging already active") {
		t.Errorf("log does not contain 'charging already active': got %q", logStr)
	}
	if !contains(logStr, "tx_id=42") {
		t.Errorf("log does not contain 'tx_id=42': got %q", logStr)
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
