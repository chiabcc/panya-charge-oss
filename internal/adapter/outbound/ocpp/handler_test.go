package ocpp

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/xBlaz3kx/ocpp-go/ocpp1.6/core"

	"github.com/chiabcc/panya-charge-oss/internal/domain/charger"
	"github.com/chiabcc/panya-charge-oss/internal/domain/proxy"
	"github.com/chiabcc/panya-charge-oss/internal/domain/session"
	"github.com/chiabcc/panya-charge-oss/internal/testutil/mocks"
)

func newRecoveryTestHandler(sr *mocks.MockSessionRepository, cr *mocks.MockChargerRepository, mr *mocks.MockMeterRepository, pub *mocks.MockEventPublisher) *Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := NewRouter(proxy.DefaultPolicy(), &fakeRelay{}, logger)
	return &Handler{
		router:      router,
		chargerRepo: cr,
		sessionRepo: sr,
		meterRepo:   mr,
		publisher:   pub,
		logger:      logger,
		minAmps:     6,
		maxAmps:     32,
	}
}

func TestHandler_MeterValuesRecoversSession(t *testing.T) {
	sr := mocks.NewMockSessionRepository()
	cr := mocks.NewMockChargerRepository()
	mr := mocks.NewMockMeterRepository()
	pub := mocks.NewMockEventPublisher()
	h := newRecoveryTestHandler(sr, cr, mr, pub)

	txID := 42
	_, err := h.OnMeterValues("CHG-001", &core.MeterValuesRequest{
		ConnectorId:   1,
		TransactionId: &txID,
	})
	if err != nil {
		t.Fatalf("OnMeterValues error: %v", err)
	}

	if sr.Count() != 1 {
		t.Fatalf("sessions = %d, want 1 after recovery", sr.Count())
	}
	s, _ := sr.GetSessionByTransactionID(context.Background(), "CHG-001", txID)
	if s == nil {
		t.Fatal("recovered session not found by txID")
	}
	if s.TransactionID != txID {
		t.Errorf("TransactionID = %d, want %d", s.TransactionID, txID)
	}
	if s.IDTag != "recovered" {
		t.Errorf("IDTag = %q, want \"recovered\"", s.IDTag)
	}
	if !s.IsActive() {
		t.Error("recovered session should be active (StoppedAt == nil)")
	}

	conn, _ := cr.GetConnector(context.Background(), "CHG-001", 1)
	if conn == nil {
		t.Fatal("connector not upserted during recovery")
	}
	if conn.Status != charger.StatusCharging {
		t.Errorf("connector status = %q, want %q", conn.Status, charger.StatusCharging)
	}

	if charging, ok := pub.LastCharging("CHG-001"); !ok || !charging {
		t.Error("PublishChargingState(true) not called after recovery")
	}
	if status, ok := pub.LastStatus("CHG-001"); !ok || status != charger.StatusCharging {
		t.Error("PublishChargerStatus(Charging) not called after recovery")
	}
}

func TestHandler_MeterValuesRecoveryIdempotent(t *testing.T) {
	sr := mocks.NewMockSessionRepository()
	cr := mocks.NewMockChargerRepository()
	mr := mocks.NewMockMeterRepository()
	pub := mocks.NewMockEventPublisher()
	h := newRecoveryTestHandler(sr, cr, mr, pub)

	txID := 42
	for i := 0; i < 3; i++ {
		_, err := h.OnMeterValues("CHG-001", &core.MeterValuesRequest{
			ConnectorId:   1,
			TransactionId: &txID,
		})
		if err != nil {
			t.Fatalf("call %d error: %v", i, err)
		}
	}

	if sr.Count() != 1 {
		t.Fatalf("sessions = %d, want 1 (recovery must be idempotent)", sr.Count())
	}
}

func TestHandler_MeterValuesNoRecoveryWithExistingSession(t *testing.T) {
	sr := mocks.NewMockSessionRepository()
	sr.Sessions["pre-existing"] = session.Session{
		ID:            "pre-existing",
		TransactionID: 99,
		ChargerID:     "CHG-001",
		ConnectorID:   1,
		IDTag:         "test",
		StartedAt:     time.Now(),
	}
	cr := mocks.NewMockChargerRepository()
	mr := mocks.NewMockMeterRepository()
	pub := mocks.NewMockEventPublisher()
	h := newRecoveryTestHandler(sr, cr, mr, pub)

	txID := 99
	_, err := h.OnMeterValues("CHG-001", &core.MeterValuesRequest{
		ConnectorId:   1,
		TransactionId: &txID,
	})
	if err != nil {
		t.Fatalf("OnMeterValues error: %v", err)
	}

	if sr.Count() != 1 {
		t.Fatalf("sessions = %d, want 1 (no new session created)", sr.Count())
	}
}

func TestHandler_MeterValuesNoRecoveryWithoutTxID(t *testing.T) {
	sr := mocks.NewMockSessionRepository()
	cr := mocks.NewMockChargerRepository()
	mr := mocks.NewMockMeterRepository()
	pub := mocks.NewMockEventPublisher()
	h := newRecoveryTestHandler(sr, cr, mr, pub)

	_, err := h.OnMeterValues("CHG-001", &core.MeterValuesRequest{
		ConnectorId: 1,
	})
	if err != nil {
		t.Fatalf("OnMeterValues error: %v", err)
	}

	if sr.Count() != 0 {
		t.Fatalf("sessions = %d, want 0 (no txID → no recovery)", sr.Count())
	}
}

func TestHandler_StopTransactionFinalizesRecoveredSession(t *testing.T) {
	sr := mocks.NewMockSessionRepository()
	cr := mocks.NewMockChargerRepository()
	mr := mocks.NewMockMeterRepository()
	pub := mocks.NewMockEventPublisher()
	h := newRecoveryTestHandler(sr, cr, mr, pub)

	txID := 42
	_, err := h.OnMeterValues("CHG-001", &core.MeterValuesRequest{
		ConnectorId:   1,
		TransactionId: &txID,
	})
	if err != nil {
		t.Fatalf("OnMeterValues (recovery) error: %v", err)
	}
	if sr.Count() != 1 {
		t.Fatalf("sessions = %d, want 1 after recovery", sr.Count())
	}

	_, err = h.OnStopTransaction("CHG-001", &core.StopTransactionRequest{
		TransactionId: txID,
	})
	if err != nil {
		t.Fatalf("OnStopTransaction error: %v", err)
	}

	s, _ := sr.GetSessionByTransactionID(context.Background(), "CHG-001", txID)
	if s == nil {
		t.Fatal("session not found after stop")
	}
	if s.IsActive() {
		t.Error("recovered session should be finalized (StoppedAt != nil)")
	}
}
