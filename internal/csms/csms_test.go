package csms

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/chiabcc/panya-charge-oss/internal/domain/charger"
	"github.com/chiabcc/panya-charge-oss/internal/domain/session"
	"github.com/chiabcc/panya-charge-oss/internal/testutil/mocks"
)

func TestStartCharging_AlreadyActiveSkipsRemoteStart(t *testing.T) {
	cmd := mocks.NewMockChargerCommander()
	cr := mocks.NewMockChargerRepository()
	cr.Connectors["CHG-001"] = []charger.Connector{
		{ChargerID: "CHG-001", ConnectorID: 1, Status: charger.StatusCharging},
	}
	sr := mocks.NewMockSessionRepository()
	sr.Sessions["active"] = session.Session{
		ID:            "active",
		TransactionID: 42,
		ChargerID:     "CHG-001",
		ConnectorID:   1,
		StartedAt:     time.Now(),
	}
	pub := mocks.NewMockEventPublisher()

	b := &cmdBridge{
		commander:   cmd,
		chargerRepo: cr,
		sessionRepo: sr,
		publisher:   pub,
		logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	b.startCharging(context.Background(), "CHG-001")

	if len(cmd.RemoteStartCalls) != 0 {
		t.Errorf("RemoteStartCalls = %d, want 0 (session already active)", len(cmd.RemoteStartCalls))
	}
	if charging, ok := pub.LastCharging("CHG-001"); !ok || !charging {
		t.Error("PublishChargingState(true) not called to resync HA")
	}
}

func TestStartCharging_NoActiveSessionSendsRemoteStart(t *testing.T) {
	cmd := mocks.NewMockChargerCommander()
	cr := mocks.NewMockChargerRepository()
	cr.Connectors["CHG-001"] = []charger.Connector{
		{ChargerID: "CHG-001", ConnectorID: 1, Status: charger.StatusCharging},
	}
	sr := mocks.NewMockSessionRepository()
	pub := mocks.NewMockEventPublisher()

	b := &cmdBridge{
		commander:   cmd,
		chargerRepo: cr,
		sessionRepo: sr,
		publisher:   pub,
		logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	b.startCharging(context.Background(), "CHG-001")

	if len(cmd.RemoteStartCalls) != 1 {
		t.Errorf("RemoteStartCalls = %d, want 1", len(cmd.RemoteStartCalls))
	}
	call := cmd.RemoteStartCalls[0]
	if call.ChargerID != "CHG-001" || call.ConnectorID != 1 {
		t.Errorf("RemoteStart(%s, %d), want (CHG-001, 1)", call.ChargerID, call.ConnectorID)
	}
}
