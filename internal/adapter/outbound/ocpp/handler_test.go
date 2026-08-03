package ocpp

import (
	"context"
	"fmt"
	"testing"

	"github.com/xBlaz3kx/ocpp-go/ocpp1.6/core"
	"github.com/xBlaz3kx/ocpp-go/ocpp1.6/types"

	"github.com/chiabcc/panya-charge-oss/internal/domain/charger"
	"github.com/chiabcc/panya-charge-oss/internal/domain/proxy"
	"github.com/chiabcc/panya-charge-oss/internal/domain/session"
	"github.com/chiabcc/panya-charge-oss/internal/testutil/mocks"
)

func newTestHandler(t *testing.T, cr *mocks.MockChargerRepository, sr *mocks.MockSessionRepository, mr *mocks.MockMeterRepository, pub *mocks.MockEventPublisher) *Handler {
	t.Helper()
	relay := mockProxyRelay{}
	router := NewRouter(proxy.DefaultPolicy(), &relay, discardedLogger())
	return NewHandler(
		router, cr, sr, mr, nil, pub, nil, nil, 6, 32, discardedLogger(), nil,
	)
}

type mockProxyRelay struct{}

func (m *mockProxyRelay) Forward(_ context.Context, _, _ string, _ any) error { return nil }
func (m *mockProxyRelay) IsConnected(_ string) bool                              { return false }

func newMeterValuesReq(connectorID int, txID *int, energyWh float64) *core.MeterValuesRequest {
	return &core.MeterValuesRequest{
		ConnectorId:   connectorID,
		TransactionId: txID,
		MeterValue: []types.MeterValue{
			{
				SampledValue: []types.SampledValue{
					{Value: "10000", Measurand: types.MeasurandPowerActiveImport, Unit: "W"},
					{Value: fmt.Sprintf("%.0f", energyWh), Measurand: types.MeasurandEnergyActiveImportRegister, Unit: "Wh"},
				},
			},
		},
	}
}

func txIntPtr(v int) *int { return &v }

func TestOnMeterValues_RecoverSession_NoPriorSession(t *testing.T) {
	cr := mocks.NewMockChargerRepository()
	sr := mocks.NewMockSessionRepository()
	mr := mocks.NewMockMeterRepository()
	pub := mocks.NewMockEventPublisher()
	h := newTestHandler(t, cr, sr, mr, pub)

	txID := 42
	req := newMeterValuesReq(1, txIntPtr(txID), 5000)

	_, err := h.OnMeterValues("CHG-001", req)
	if err != nil {
		t.Fatalf("OnMeterValues error: %v", err)
	}

	sessions, _ := sr.ListSessions(context.Background(), 0, 0)
	if len(sessions) != 1 {
		t.Fatalf("sessions created = %d, want 1", len(sessions))
	}
	if sessions[0].TransactionID != txID {
		t.Errorf("session TransactionID = %d, want %d", sessions[0].TransactionID, txID)
	}
	if sessions[0].ChargerID != "CHG-001" {
		t.Errorf("session ChargerID = %s, want CHG-001", sessions[0].ChargerID)
	}
	if sessions[0].IDTag != "recovered" {
		t.Errorf("session IDTag = %s, want recovered", sessions[0].IDTag)
	}

	conns, _ := cr.ListConnectors(context.Background(), "CHG-001")
	if len(conns) != 1 || conns[0].Status != charger.StatusCharging {
		t.Errorf("connector status = %v, want Charging", conns)
	}

	if len(pub.ChargingPublished) != 1 || !pub.ChargingPublished[0].Charging {
		t.Errorf("ChargingPublished = %v, want [%+v]", pub.ChargingPublished, mocks.ChargingPublish{ChargerID: "CHG-001", Charging: true})
	}
}

func TestOnMeterValues_RecoverSession_Idempotent(t *testing.T) {
	cr := mocks.NewMockChargerRepository()
	sr := mocks.NewMockSessionRepository()
	mr := mocks.NewMockMeterRepository()
	pub := mocks.NewMockEventPublisher()
	h := newTestHandler(t, cr, sr, mr, pub)

	txID := 42
	req := newMeterValuesReq(1, txIntPtr(txID), 5000)

	_, _ = h.OnMeterValues("CHG-001", req)
	countBefore := sr.Count()

	_, _ = h.OnMeterValues("CHG-001", req)
	countAfter := sr.Count()

	if countAfter > countBefore {
		t.Errorf("session count after 2nd pulse = %d, want %d (idempotent)", countAfter, countBefore)
	}
}

func TestOnMeterValues_SessionExists_NoRecovery(t *testing.T) {
	cr := mocks.NewMockChargerRepository()
	sr := mocks.NewMockSessionRepository()
	mr := mocks.NewMockMeterRepository()
	pub := mocks.NewMockEventPublisher()
	h := newTestHandler(t, cr, sr, mr, pub)

	txID := 42
	if err := sr.CreateSession(context.Background(), session.Session{
		ID: "existing-sess", TransactionID: txID, ChargerID: "CHG-001", ConnectorID: 1,
	}); err != nil {
		t.Fatalf("pre-create session: %v", err)
	}

	req := newMeterValuesReq(1, txIntPtr(txID), 5000)
	_, _ = h.OnMeterValues("CHG-001", req)

	sessions, _ := sr.ListSessions(context.Background(), 0, 0)
	if len(sessions) != 1 {
		t.Fatalf("sessions = %d, want 1", len(sessions))
	}
	if sessions[0].ID != "existing-sess" {
		t.Errorf("session ID = %s, want existing-sess", sessions[0].ID)
	}
}

func TestOnMeterValues_NoTransactionId_NoRecovery(t *testing.T) {
	cr := mocks.NewMockChargerRepository()
	sr := mocks.NewMockSessionRepository()
	mr := mocks.NewMockMeterRepository()
	pub := mocks.NewMockEventPublisher()
	h := newTestHandler(t, cr, sr, mr, pub)

	req := newMeterValuesReq(1, nil, 5000)
	_, _ = h.OnMeterValues("CHG-001", req)

	sessions, _ := sr.ListSessions(context.Background(), 0, 0)
	if len(sessions) != 0 {
		t.Errorf("sessions = %d, want 0 (no txID = no recovery)", len(sessions))
	}
}

func TestOnStopTransaction_FinalizesRecoveredSession(t *testing.T) {
	cr := mocks.NewMockChargerRepository()
	sr := mocks.NewMockSessionRepository()
	mr := mocks.NewMockMeterRepository()
	pub := mocks.NewMockEventPublisher()
	h := newTestHandler(t, cr, sr, mr, pub)

	txID := 42
	req := newMeterValuesReq(1, txIntPtr(txID), 5000)

	_, _ = h.OnMeterValues("CHG-001", req)

	sessions, _ := sr.ListSessions(context.Background(), 0, 0)
	if len(sessions) != 1 {
		t.Fatalf("recovery failed: sessions = %d", len(sessions))
	}
	recoveredID := sessions[0].ID

	stopReq := &core.StopTransactionRequest{
		TransactionId: txID,
		MeterStop:     15000,
		Reason:        core.ReasonLocal,
	}
	_, err := h.OnStopTransaction("CHG-001", stopReq)
	if err != nil {
		t.Fatalf("OnStopTransaction error: %v", err)
	}

	s, err := sr.GetSession(context.Background(), recoveredID)
	if err != nil {
		t.Fatalf("GetSession(%s): %v", recoveredID, err)
	}
	if s.StoppedAt == nil {
		t.Error("StoppedAt is nil, want non-nil")
	}
	if s.MeterStopWh == nil || *s.MeterStopWh != 15000 {
		t.Errorf("MeterStopWh = %v, want 15000", s.MeterStopWh)
	}
}
