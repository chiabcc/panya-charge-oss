package mocks_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/chiabcc/panya-charge-oss/internal/domain/charger"
	"github.com/chiabcc/panya-charge-oss/internal/domain/ports"
	"github.com/chiabcc/panya-charge-oss/internal/domain/session"
	"github.com/chiabcc/panya-charge-oss/internal/testutil/mocks"
)

func TestMockChargerRepository_UpsertAndGet(t *testing.T) {
	m := mocks.NewMockChargerRepository()
	ctx := context.Background()

	c := charger.Charger{ID: "ABB-001", Vendor: "ABB", Online: false}
	if err := m.UpsertCharger(ctx, c); err != nil {
		t.Fatalf("UpsertCharger() = %v, want nil", err)
	}

	got, err := m.GetCharger(ctx, "ABB-001")
	if err != nil {
		t.Fatalf("GetCharger() error = %v, want nil", err)
	}
	if got.ID != "ABB-001" {
		t.Errorf("GetCharger().ID = %q, want %q", got.ID, "ABB-001")
	}
}

func TestMockChargerRepository_GetNotFoun(t *testing.T) {
	m := mocks.NewMockChargerRepository()
	ctx := context.Background()

	_, err := m.GetCharger(ctx, "NOPE")
	if err == nil {
		t.Error("GetCharger() error = nil, want non-nil")
	}
}

func TestMockChargerRepository_UpsertChargerErr(t *testing.T) {
	m := mocks.NewMockChargerRepository()
	m.UpsertChargerErr = errors.New("db error")
	ctx := context.Background()

	err := m.UpsertCharger(ctx, charger.Charger{ID: "X"})
	if !errors.Is(err, m.UpsertChargerErr) {
		t.Errorf("UpsertCharger() error = %v, want %v", err, m.UpsertChargerErr)
	}
}

func TestMockChargerRepository_ListChargers(t *testing.T) {
	m := mocks.NewMockChargerRepository()
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		_ = m.UpsertCharger(ctx, charger.Charger{ID: "C1", Vendor: "ABB"})
	}

	got, err := m.ListChargers(ctx)
	if err != nil {
		t.Fatalf("ListChargers() error = %v, want nil", err)
	}
	if len(got) != 1 {
		t.Errorf("ListChargers() len = %d, want 1", len(got))
	}
}

func TestMockChargerRepository_MarkOnline(t *testing.T) {
	m := mocks.NewMockChargerRepository()
	ctx := context.Background()

	_ = m.UpsertCharger(ctx, charger.Charger{ID: "ABB-001", Online: false})
	if err := m.MarkOnline(ctx, "ABB-001", true); err != nil {
		t.Fatalf("MarkOnline() = %v, want nil", err)
	}
	c, _ := m.GetCharger(ctx, "ABB-001")
	if !c.Online {
		t.Error("GetCharger().Online = false, want true")
	}
}

func TestMockChargerRepository_UpsertConnector(t *testing.T) {
	m := mocks.NewMockChargerRepository()
	ctx := context.Background()

	conn := charger.Connector{ChargerID: "ABB-001", ConnectorID: 1, Status: charger.StatusAvailable}
	if err := m.UpsertConnector(ctx, conn); err != nil {
		t.Fatalf("UpsertConnector() = %v, want nil", err)
	}

	got, err := m.GetConnector(ctx, "ABB-001", 1)
	if err != nil {
		t.Fatalf("GetConnector() error = %v, want nil", err)
	}
	if got.Status != charger.StatusAvailable {
		t.Errorf("GetConnector().Status = %q, want %q", got.Status, charger.StatusAvailable)
	}
}

func TestMockChargerRepository_UpsertConnector_UpdatesExisting(t *testing.T) {
	m := mocks.NewMockChargerRepository()
	ctx := context.Background()

	conn1 := charger.Connector{ChargerID: "ABB-001", ConnectorID: 1, Status: charger.StatusAvailable}
	_ = m.UpsertConnector(ctx, conn1)

	conn2 := charger.Connector{ChargerID: "ABB-001", ConnectorID: 1, Status: charger.StatusCharging}
	_ = m.UpsertConnector(ctx, conn2)

	got, _ := m.GetConnector(ctx, "ABB-001", 1)
	if got.Status != charger.StatusCharging {
		t.Errorf("GetConnector().Status = %q, want %q", got.Status, charger.StatusCharging)
	}

	conns, _ := m.ListConnectors(ctx, "ABB-001")
	if len(conns) != 1 {
		t.Errorf("ListConnectors() len = %d, want 1", len(conns))
	}
}

func TestMockSessionRepository_CreateAndGet(t *testing.T) {
	m := mocks.NewMockSessionRepository()
	ctx := context.Background()

	s := session.Session{ID: "s1", ChargerID: "ABB-001", ConnectorID: 1, TransactionID: 42}
	if err := m.CreateSession(ctx, s); err != nil {
		t.Fatalf("CreateSession() = %v, want nil", err)
	}

	got, err := m.GetSession(ctx, "s1")
	if err != nil {
		t.Fatalf("GetSession() error = %v, want nil", err)
	}
	if got.TransactionID != 42 {
		t.Errorf("GetSession().TransactionID = %d, want 42", got.TransactionID)
	}
}

func TestMockSessionRepository_GetActiveSession(t *testing.T) {
	m := mocks.NewMockSessionRepository()
	ctx := context.Background()

	s := session.Session{ID: "s1", ChargerID: "ABB-001", ConnectorID: 1, TransactionID: 42}
	_ = m.CreateSession(ctx, s)

	got, err := m.GetActiveSession(ctx, "ABB-001", 1)
	if err != nil {
		t.Fatalf("GetActiveSession() error = %v, want nil", err)
	}
	if got.ID != "s1" {
		t.Errorf("GetActiveSession().ID = %q, want %q", got.ID, "s1")
	}
}

func TestMockSessionRepository_GetActiveSession_NoActive(t *testing.T) {
	m := mocks.NewMockSessionRepository()
	ctx := context.Background()

	stopped := time.Now().Add(-time.Hour)
	s := session.Session{ID: "s1", ChargerID: "ABB-001", ConnectorID: 1, StoppedAt: &stopped}
	_ = m.CreateSession(ctx, s)

	_, err := m.GetActiveSession(ctx, "ABB-001", 1)
	if err == nil {
		t.Error("GetActiveSession() error = nil, want non-nil")
	}
}

func TestMockSessionRepository_GetByTransactionID(t *testing.T) {
	m := mocks.NewMockSessionRepository()
	ctx := context.Background()

	s := session.Session{ID: "s1", ChargerID: "ABB-001", ConnectorID: 1, TransactionID: 99}
	_ = m.CreateSession(ctx, s)

	got, err := m.GetSessionByTransactionID(ctx, "ABB-001", 99)
	if err != nil {
		t.Fatalf("GetSessionByTransactionID() error = %v, want nil", err)
	}
	if got.ID != "s1" {
		t.Errorf("GetSessionByTransactionID().ID = %q, want %q", got.ID, "s1")
	}
}

func TestMockSessionRepository_UpdateSession(t *testing.T) {
	m := mocks.NewMockSessionRepository()
	ctx := context.Background()

	s := session.Session{ID: "s1", ChargerID: "ABB-001"}
	_ = m.CreateSession(ctx, s)

	s.StopReason = "Remote"
	if err := m.UpdateSession(ctx, s); err != nil {
		t.Fatalf("UpdateSession() = %v, want nil", err)
	}

	got, _ := m.GetSession(ctx, "s1")
	if got.StopReason != "Remote" {
		t.Errorf("GetSession().StopReason = %q, want %q", got.StopReason, "Remote")
	}
}

func TestMockSessionRepository_ListSessions_Limit(t *testing.T) {
	m := mocks.NewMockSessionRepository()
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		_ = m.CreateSession(ctx, session.Session{ID: "s" + string(rune('0'+i))})
	}

	got, _ := m.ListSessions(ctx, 2, 0)
	if len(got) != 2 {
		t.Errorf("ListSessions(limit=2) len = %d, want 2", len(got))
	}
}

func TestMockMeterRepository_StoreAndGet(t *testing.T) {
	m := mocks.NewMockMeterRepository()
	ctx := context.Background()

	now := time.Now()
	mv := ports.MeterValue{ChargerID: "ABB-001", Timestamp: now, Value: 3.5}
	if err := m.StoreMeterValue(ctx, mv); err != nil {
		t.Fatalf("StoreMeterValue() = %v, want nil", err)
	}

	got, err := m.GetMeterValues(ctx, "ABB-001", now.Add(-time.Hour), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("GetMeterValues() error = %v, want nil", err)
	}
	if len(got) != 1 {
		t.Errorf("GetMeterValues() len = %d, want 1", len(got))
	}
}

func TestMockMeterRepository_StoreMeterValues(t *testing.T) {
	m := mocks.NewMockMeterRepository()
	ctx := context.Background()

	now := time.Now()
	mvs := []ports.MeterValue{
		{ChargerID: "ABB-001", Timestamp: now, Value: 1},
		{ChargerID: "ABB-001", Timestamp: now, Value: 2},
	}
	if err := m.StoreMeterValues(ctx, mvs); err != nil {
		t.Fatalf("StoreMeterValues() = %v, want nil", err)
	}

	got, _ := m.GetMeterValues(ctx, "ABB-001", now.Add(-time.Hour), now.Add(time.Hour))
	if len(got) != 2 {
		t.Errorf("GetMeterValues() len = %d, want 2", len(got))
	}
}

func TestMockMeterRepository_PurgeOlderThan(t *testing.T) {
	m := mocks.NewMockMeterRepository()
	ctx := context.Background()

	now := time.Now()
	m.Values = []ports.MeterValue{
		{ChargerID: "A", Timestamp: now.Add(-2 * time.Hour)},
		{ChargerID: "A", Timestamp: now.Add(-time.Minute)},
	}

	count, err := m.PurgeOlderThan(ctx, now.Add(-30*time.Minute))
	if err != nil {
		t.Fatalf("PurgeOlderThan() error = %v, want nil", err)
	}
	if count != 1 {
		t.Errorf("PurgeOlderThan() count = %d, want 1", count)
	}
	if len(m.Values) != 1 {
		t.Errorf("Values len = %d, want 1", len(m.Values))
	}
}

func TestMockEventPublisher_PublishAndLast(t *testing.T) {
	m := mocks.NewMockEventPublisher()

	m.PublishChargerStatus("ABB-001", charger.StatusAvailable)
	m.PublishChargerPower("ABB-001", 3.5)
	m.PublishChargerOnline("ABB-001", true)
	m.PublishChargerCurrent("ABB-001", 16)
	m.PublishChargingState("ABB-001", true)

	if status, ok := m.LastStatus("ABB-001"); !ok || status != charger.StatusAvailable {
		t.Errorf("LastStatus() = %v, %v, want %v, true", status, ok, charger.StatusAvailable)
	}
	if pwr, ok := m.LastPower("ABB-001"); !ok || pwr != 3.5 {
		t.Errorf("LastPower() = %v, %v, want %v, true", pwr, ok, 3.5)
	}
	if online, ok := m.LastOnline("ABB-001"); !ok || !online {
		t.Errorf("LastOnline() = %v, %v, want %v, true", online, ok, true)
	}
	if amps, ok := m.LastCurrent("ABB-001"); !ok || amps != 16 {
		t.Errorf("LastCurrent() = %v, %v, want %v, true", amps, ok, 16)
	}
	if chg, ok := m.LastCharging("ABB-001"); !ok || !chg {
		t.Errorf("LastCharging() = %v, %v, want %v, true", chg, ok, true)
	}
}

func TestMockEventPublisher_Last_MissingCharger(t *testing.T) {
	m := mocks.NewMockEventPublisher()

	if _, ok := m.LastStatus("NOPE"); ok {
		t.Error("LastStatus(NOPE) = _, true, want _, false")
	}
}

func TestMockEventPublisher_Reset(t *testing.T) {
	m := mocks.NewMockEventPublisher()
	m.PublishChargerStatus("ABB-001", charger.StatusAvailable)
	m.Reset()

	if len(m.StatusPublished) != 0 {
		t.Errorf("Reset() StatusPublished len = %d, want 0", len(m.StatusPublished))
	}
}

func TestMockDiscoveryPublisher(t *testing.T) {
	m := mocks.NewMockDiscoveryPublisher()

	m.PublishDiscovery(charger.Charger{ID: "ABB-001"}, 6, 32, true)
	if m.CallCount() != 1 {
		t.Errorf("CallCount() = %d, want 1", m.CallCount())
	}

	call := m.DiscoveryCalls[0]
	if call.Charger.ID != "ABB-001" || call.MinAmps != 6 || call.MaxAmps != 32 || !call.ProxyEnabled {
		t.Errorf("DiscoveryCalls[0] = %+v, want Charger.ID=ABB-001 MinAmps=6 MaxAmps=32 ProxyEnabled=true", call)
	}

	m.Reset()
	if m.CallCount() != 0 {
		t.Errorf("Reset() CallCount() = %d, want 0", m.CallCount())
	}
}

func TestMockEnergySource(t *testing.T) {
	m := mocks.NewMockEnergySource()

	m.GridPowerW = -5000
	if m.GetGridPowerW() != -5000 {
		t.Errorf("GetGridPowerW() = %v, want -5000", m.GetGridPowerW())
	}

	m.SolarPowerW = 3000
	if m.GetSolarPowerW() != 3000 {
		t.Errorf("GetSolarPowerW() = %v, want 3000", m.GetSolarPowerW())
	}

	m.ConsumptionPowerW = 800
	if m.GetConsumptionPowerW() != 800 {
		t.Errorf("GetConsumptionPowerW() = %v, want 800", m.GetConsumptionPowerW())
	}

	if m.IsStale(time.Minute) {
		t.Error("IsStale() = true, want false")
	}

	m.Stale = true
	if !m.IsStale(time.Minute) {
		t.Error("IsStale() = false, want true")
	}

	m.GridStale = true
	if !m.IsGridStale(time.Minute) {
		t.Error("IsGridStale() = false, want true")
	}

	m.SolarAvail = true
	if !m.IsSolarAvailable(time.Minute) {
		t.Error("IsSolarAvailable() = false, want true")
	}

	m.ConsumptionAvail = true
	if !m.IsConsumptionAvailable(time.Minute) {
		t.Error("IsConsumptionAvailable() = false, want true")
	}
}

func TestMockChargerCommander_RecordCalls(t *testing.T) {
	m := mocks.NewMockChargerCommander()

	_ = m.SetChargingProfile("ABB-001", 1, 16)
	_ = m.RemoteStartTransaction("ABB-001", 1, "tag123")
	_ = m.RemoteStopTransaction("ABB-001", 42)
	_ = m.ClearChargingProfile("ABB-001", 1)

	if len(m.SetChargingProfileCalls) != 1 {
		t.Errorf("SetChargingProfileCalls len = %d, want 1", len(m.SetChargingProfileCalls))
	}
	call := m.SetChargingProfileCalls[0]
	if call.ChargerID != "ABB-001" || call.ConnectorID != 1 || call.LimitAmps != 16 {
		t.Errorf("SetChargingProfileCalls[0] = %+v, want ChargerID=ABB-001 ConnectorID=1 LimitAmps=16", call)
	}

	if len(m.RemoteStartCalls) != 1 {
		t.Errorf("RemoteStartCalls len = %d, want 1", len(m.RemoteStartCalls))
	}
	if m.RemoteStartCalls[0].IDTag != "tag123" {
		t.Errorf("RemoteStartCalls[0].IDTag = %q, want %q", m.RemoteStartCalls[0].IDTag, "tag123")
	}

	if len(m.RemoteStopCalls) != 1 {
		t.Errorf("RemoteStopCalls len = %d, want 1", len(m.RemoteStopCalls))
	}
	if m.RemoteStopCalls[0].TransactionID != 42 {
		t.Errorf("RemoteStopCalls[0].TransactionID = %d, want 42", m.RemoteStopCalls[0].TransactionID)
	}

	if len(m.ClearChargingProfileCalls) != 1 {
		t.Errorf("ClearChargingProfileCalls len = %d, want 1", len(m.ClearChargingProfileCalls))
	}
}

func TestMockChargerCommander_ErrorInjection(t *testing.T) {
	m := mocks.NewMockChargerCommander()
	wantErr := errors.New("OCPP timeout")

	m.SetChargingProfileErr = wantErr
	err := m.SetChargingProfile("X", 1, 16)
	if !errors.Is(err, wantErr) {
		t.Errorf("SetChargingProfile() error = %v, want %v", err, wantErr)
	}

	m.RemoteStartErr = wantErr
	err = m.RemoteStartTransaction("X", 1, "t")
	if !errors.Is(err, wantErr) {
		t.Errorf("RemoteStartTransaction() error = %v, want %v", err, wantErr)
	}

	m.RemoteStopErr = wantErr
	err = m.RemoteStopTransaction("X", 1)
	if !errors.Is(err, wantErr) {
		t.Errorf("RemoteStopTransaction() error = %v, want %v", err, wantErr)
	}

	m.ClearChargingProfileErr = wantErr
	err = m.ClearChargingProfile("X", 1)
	if !errors.Is(err, wantErr) {
		t.Errorf("ClearChargingProfile() error = %v, want %v", err, wantErr)
	}
}

func TestMockChargerCommander_Reset(t *testing.T) {
	m := mocks.NewMockChargerCommander()
	_ = m.SetChargingProfile("X", 1, 16)
	m.SetChargingProfileErr = errors.New("err")
	m.Reset()

	if len(m.SetChargingProfileCalls) != 0 {
		t.Errorf("Reset() calls len = %d, want 0", len(m.SetChargingProfileCalls))
	}
	if m.SetChargingProfileErr != nil {
		t.Errorf("Reset() error = %v, want nil", m.SetChargingProfileErr)
	}
}

func TestMockCommandReceiver_RecordCalls(t *testing.T) {
	m := mocks.NewMockCommandReceiver()

	m.OnSetAmps("ABB-001", 16)
	m.OnSetState("ABB-001", true)

	if len(m.SetAmpsCalls) != 1 {
		t.Errorf("SetAmpsCalls len = %d, want 1", len(m.SetAmpsCalls))
	}
	if m.SetAmpsCalls[0].Amps != 16 {
		t.Errorf("SetAmpsCalls[0].Amps = %d, want 16", m.SetAmpsCalls[0].Amps)
	}

	if len(m.SetStateCalls) != 1 {
		t.Errorf("SetStateCalls len = %d, want 1", len(m.SetStateCalls))
	}
	if !m.SetStateCalls[0].Charging {
		t.Errorf("SetStateCalls[0].Charging = %v, want true", m.SetStateCalls[0].Charging)
	}
}

func TestMockCommandReceiver_Reset(t *testing.T) {
	m := mocks.NewMockCommandReceiver()
	m.OnSetAmps("X", 1)
	m.Reset()

	if len(m.SetAmpsCalls) != 0 {
		t.Errorf("Reset() len = %d, want 0", len(m.SetAmpsCalls))
	}
}

func TestMockChargerRepository_ConcurrentAccess(t *testing.T) {
	m := mocks.NewMockChargerRepository()
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			_ = m.UpsertCharger(ctx, charger.Charger{ID: "C1"})
			_ = m.UpsertConnector(ctx, charger.Connector{ChargerID: "C1", ConnectorID: id})
		}(i)
	}
	wg.Wait()

	conns, _ := m.ListConnectors(ctx, "C1")
	if len(conns) != 10 {
		t.Errorf("ListConnectors() len = %d, want 10", len(conns))
	}
}

func TestMockEventPublisher_ConcurrentAccess(t *testing.T) {
	m := mocks.NewMockEventPublisher()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.PublishChargerStatus("ABB-001", charger.StatusCharging)
		}()
	}
	wg.Wait()

	if len(m.StatusPublished) != 10 {
		t.Errorf("StatusPublished len = %d, want 10", len(m.StatusPublished))
	}
}

func TestMockSessionRepository_CreateSessionErr(t *testing.T) {
	m := mocks.NewMockSessionRepository()
	m.CreateSessionErr = errors.New("db error")
	ctx := context.Background()

	err := m.CreateSession(ctx, session.Session{ID: "s1"})
	if !errors.Is(err, m.CreateSessionErr) {
		t.Errorf("CreateSession() error = %v, want %v", err, m.CreateSessionErr)
	}
}

func TestMockMeterRepository_StoreMeterValueErr(t *testing.T) {
	m := mocks.NewMockMeterRepository()
	m.StoreMeterValueErr = errors.New("db error")
	ctx := context.Background()

	err := m.StoreMeterValue(ctx, ports.MeterValue{ChargerID: "X"})
	if !errors.Is(err, m.StoreMeterValueErr) {
		t.Errorf("StoreMeterValue() error = %v, want %v", err, m.StoreMeterValueErr)
	}
}

func TestMockMeterRepository_GetMeterValuesErr(t *testing.T) {
	m := mocks.NewMockMeterRepository()
	m.GetMeterValuesErr = errors.New("db error")
	ctx := context.Background()

	got, err := m.GetMeterValues(ctx, "X", time.Time{}, time.Time{})
	if !errors.Is(err, m.GetMeterValuesErr) {
		t.Errorf("GetMeterValues() error = %v, want %v", err, m.GetMeterValuesErr)
	}
	if got != nil {
		t.Errorf("GetMeterValues() got = %v, want nil", got)
	}
}

func TestMockSessionRepository_GetActiveSessionErr(t *testing.T) {
	m := mocks.NewMockSessionRepository()
	m.GetActiveSessionErr = errors.New("db error")
	ctx := context.Background()

	_, err := m.GetActiveSession(ctx, "X", 1)
	if !errors.Is(err, m.GetActiveSessionErr) {
		t.Errorf("GetActiveSession() error = %v, want %v", err, m.GetActiveSessionErr)
	}
}

func TestMockSessionRepository_UpdateSessionErr(t *testing.T) {
	m := mocks.NewMockSessionRepository()
	m.UpdateSessionErr = errors.New("db error")
	ctx := context.Background()

	err := m.UpdateSession(ctx, session.Session{ID: "s1"})
	if !errors.Is(err, m.UpdateSessionErr) {
		t.Errorf("UpdateSession() error = %v, want %v", err, m.UpdateSessionErr)
	}
}

func TestMockMeterRepository_StoreMeterValuesErr(t *testing.T) {
	m := mocks.NewMockMeterRepository()
	m.StoreMeterValuesErr = errors.New("db error")
	ctx := context.Background()

	err := m.StoreMeterValues(ctx, []ports.MeterValue{{ChargerID: "X"}})
	if !errors.Is(err, m.StoreMeterValuesErr) {
		t.Errorf("StoreMeterValues() error = %v, want %v", err, m.StoreMeterValuesErr)
	}
}
