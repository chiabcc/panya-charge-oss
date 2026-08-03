package ocpp

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/chiabcc/panya-charge-oss/internal/domain/ports"
	"github.com/chiabcc/panya-charge-oss/internal/domain/session"
	"github.com/xBlaz3kx/ocpp-go/ocpp1.6/core"
	"github.com/xBlaz3kx/ocpp-go/ocpp1.6/smartcharging"
	"github.com/xBlaz3kx/ocpp-go/ocpp1.6/types"
)

func TestCommander_SetCooldown_RaceFree(t *testing.T) {
	cmd := NewCommander(nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))

	var wg sync.WaitGroup
	cmd.mu.Lock()
	cmd.lastStartStop["CHG-001"] = time.Now().Add(-100 * time.Second)
	cmd.mu.Unlock()

	const iters = 200

	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < iters; i++ {
			_ = cmd.enforceCooldown("CHG-001")
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < iters; i++ {
			cmd.SetCooldown(time.Duration(180+i%60) * time.Second)
		}
	}()

	wg.Wait()
}

func TestCommander_SetCooldown_Applied(t *testing.T) {
	cmd := NewCommander(nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))

	cmd.mu.Lock()
	cmd.lastStartStop["CHG-001"] = time.Now().Add(-200 * time.Second)
	cmd.mu.Unlock()

	cmd.SetCooldown(300 * time.Second)

	err := cmd.enforceCooldown("CHG-001")
	if err == nil {
		t.Error("enforceCooldown() = nil, want error (200s < 300s cooldown)")
	}

	cmd.SetCooldown(100 * time.Second)

	err = cmd.enforceCooldown("CHG-001")
	if err != nil {
		t.Errorf("enforceCooldown() = %v, want nil (200s > 100s cooldown)", err)
	}
}

func TestBuildTxProfile_PurposeAndTransactionID(t *testing.T) {
	profile := buildTxProfile(16, 42)

	if profile.ChargingProfilePurpose != types.ChargingProfilePurposeTxProfile {
		t.Errorf("purpose = %s, want TxProfile", profile.ChargingProfilePurpose)
	}
	if profile.TransactionId != 42 {
		t.Errorf("transactionId = %d, want 42", profile.TransactionId)
	}
	if profile.StackLevel != abbTxDefaultStackLevel {
		t.Errorf("stackLevel = %d, want %d", profile.StackLevel, abbTxDefaultStackLevel)
	}
	if profile.ChargingProfileKind != types.ChargingProfileKindRelative {
		t.Errorf("kind = %s, want Relative", profile.ChargingProfileKind)
	}
	if profile.ChargingSchedule == nil {
		t.Error("schedule is nil")
	}
}

type mockCS struct {
	mu                   sync.Mutex
	calls                []callRecord
	setChargingProfileFn func(callback func(*smartcharging.SetChargingProfileConfirmation, error), connectorID int, profile *types.ChargingProfile) error
}

type callRecord struct {
	purpose       types.ChargingProfilePurposeType
	transactionID int
	connectorID   int
}

func (m *mockCS) SetChargingProfile(clientID string, callback func(*smartcharging.SetChargingProfileConfirmation, error), connectorID int, profile *types.ChargingProfile, props ...func(request *smartcharging.SetChargingProfileRequest)) error {
	if m.setChargingProfileFn != nil {
		return m.setChargingProfileFn(callback, connectorID, profile)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, callRecord{
		purpose:       profile.ChargingProfilePurpose,
		transactionID: profile.TransactionId,
		connectorID:   connectorID,
	})
	callback(&smartcharging.SetChargingProfileConfirmation{Status: smartcharging.ChargingProfileStatusAccepted}, nil)
	return nil
}
func (m *mockCS) RemoteStartTransaction(string, func(*core.RemoteStartTransactionConfirmation, error), string, ...func(request *core.RemoteStartTransactionRequest)) error {
	return nil
}
func (m *mockCS) RemoteStopTransaction(string, func(*core.RemoteStopTransactionConfirmation, error), int, ...func(request *core.RemoteStopTransactionRequest)) error {
	return nil
}
func (m *mockCS) ClearChargingProfile(string, func(*smartcharging.ClearChargingProfileConfirmation, error), ...func(request *smartcharging.ClearChargingProfileRequest)) error {
	return nil
}

type mockSR struct {
	activeSession *session.Session
	err           error
}

func (m *mockSR) CreateSession(context.Context, session.Session) error { return nil }
func (m *mockSR) UpdateSession(context.Context, session.Session) error { return nil }
func (m *mockSR) GetActiveSession(context.Context, string, int) (*session.Session, error) {
	return m.activeSession, m.err
}
func (m *mockSR) GetSessionByTransactionID(context.Context, string, int) (*session.Session, error) {
	return nil, nil
}
func (m *mockSR) GetSession(context.Context, string) (*session.Session, error)      { return nil, nil }
func (m *mockSR) ListSessions(context.Context, int, int) ([]session.Session, error) { return nil, nil }

var _ chargingProfileSender = (*mockCS)(nil)
var _ ports.SessionRepository = (*mockSR)(nil)

func TestCommander_NoActiveSession_SendsOnlyTxDefault(t *testing.T) {
	cs := &mockCS{}
	sr := &mockSR{}
	cmd := NewCommander(cs, sr, slog.New(slog.NewTextHandler(io.Discard, nil)))

	err := cmd.SetChargingProfile("CHG-001", 1, 16)
	if err != nil {
		t.Fatalf("SetChargingProfile() = %v, want nil", err)
	}

	cs.mu.Lock()
	defer cs.mu.Unlock()
	if len(cs.calls) != 1 {
		t.Fatalf("calls = %d, want 1 (TxDefaultProfile only)", len(cs.calls))
	}
	if cs.calls[0].purpose != types.ChargingProfilePurposeTxDefaultProfile {
		t.Errorf("call[0] purpose = %s, want TxDefaultProfile", cs.calls[0].purpose)
	}
	if cs.calls[0].transactionID != 0 {
		t.Errorf("call[0] transactionID = %d, want 0", cs.calls[0].transactionID)
	}
}

func TestCommander_ActiveSession_SendsBothProfiles(t *testing.T) {
	cs := &mockCS{}
	sess := &session.Session{ID: "s1", TransactionID: 99, ChargerID: "CHG-001", ConnectorID: 1}
	sr := &mockSR{activeSession: sess}
	cmd := NewCommander(cs, sr, slog.New(slog.NewTextHandler(io.Discard, nil)))

	err := cmd.SetChargingProfile("CHG-001", 1, 16)
	if err != nil {
		t.Fatalf("SetChargingProfile() = %v, want nil", err)
	}

	cs.mu.Lock()
	defer cs.mu.Unlock()
	if len(cs.calls) != 2 {
		t.Fatalf("calls = %d, want 2 (TxDefault + TxProfile)", len(cs.calls))
	}
	if cs.calls[0].purpose != types.ChargingProfilePurposeTxDefaultProfile {
		t.Errorf("call[0] purpose = %s, want TxDefaultProfile", cs.calls[0].purpose)
	}
	if cs.calls[1].purpose != types.ChargingProfilePurposeTxProfile {
		t.Errorf("call[1] purpose = %s, want TxProfile", cs.calls[1].purpose)
	}
	if cs.calls[1].transactionID != 99 {
		t.Errorf("call[1] transactionID = %d, want 99", cs.calls[1].transactionID)
	}
}

func TestCommander_TxProfileError_DoesNotFail(t *testing.T) {
	cs := &mockCS{}
	cs.setChargingProfileFn = func(callback func(*smartcharging.SetChargingProfileConfirmation, error), connectorID int, profile *types.ChargingProfile) error {
		switch profile.ChargingProfilePurpose {
		case types.ChargingProfilePurposeTxDefaultProfile:
			callback(&smartcharging.SetChargingProfileConfirmation{Status: smartcharging.ChargingProfileStatusAccepted}, nil)
			return nil
		case types.ChargingProfilePurposeTxProfile:
			callback(nil, context.DeadlineExceeded)
			return nil
		}
		return nil
	}
	sess := &session.Session{ID: "s1", TransactionID: 42, ChargerID: "CHG-001", ConnectorID: 1}
	sr := &mockSR{activeSession: sess}
	cmd := NewCommander(cs, sr, slog.New(slog.NewTextHandler(io.Discard, nil)))

	err := cmd.SetChargingProfile("CHG-001", 1, 16)
	if err != nil {
		t.Fatalf("SetChargingProfile() = %v, want nil (TxProfile error should not propagate)", err)
	}
}

func TestCommander_TxProfileRejected_DoesNotFail(t *testing.T) {
	cs := &mockCS{}
	cs.setChargingProfileFn = func(callback func(*smartcharging.SetChargingProfileConfirmation, error), connectorID int, profile *types.ChargingProfile) error {
		switch profile.ChargingProfilePurpose {
		case types.ChargingProfilePurposeTxDefaultProfile:
			callback(&smartcharging.SetChargingProfileConfirmation{Status: smartcharging.ChargingProfileStatusAccepted}, nil)
			return nil
		case types.ChargingProfilePurposeTxProfile:
			callback(&smartcharging.SetChargingProfileConfirmation{Status: smartcharging.ChargingProfileStatusRejected}, nil)
			return nil
		}
		return nil
	}
	sess := &session.Session{ID: "s1", TransactionID: 42, ChargerID: "CHG-001", ConnectorID: 1}
	sr := &mockSR{activeSession: sess}
	cmd := NewCommander(cs, sr, slog.New(slog.NewTextHandler(io.Discard, nil)))

	err := cmd.SetChargingProfile("CHG-001", 1, 16)
	if err != nil {
		t.Fatalf("SetChargingProfile() = %v, want nil (TxProfile rejection should not propagate)", err)
	}
}

func TestCommander_TxDefaultError_ReturnsError(t *testing.T) {
	cs := &mockCS{}
	cs.setChargingProfileFn = func(callback func(*smartcharging.SetChargingProfileConfirmation, error), connectorID int, profile *types.ChargingProfile) error {
		if profile.ChargingProfilePurpose == types.ChargingProfilePurposeTxDefaultProfile {
			callback(nil, context.DeadlineExceeded)
			return nil
		}
		return nil
	}
	sr := &mockSR{}
	cmd := NewCommander(cs, sr, slog.New(slog.NewTextHandler(io.Discard, nil)))

	err := cmd.SetChargingProfile("CHG-001", 1, 16)
	if err == nil {
		t.Fatal("SetChargingProfile() = nil, want error (TxDefaultProfile failure must propagate)")
	}
}

func TestCommander_ZeroConnector_SKipsTxProfile(t *testing.T) {
	cs := &mockCS{}
	sess := &session.Session{ID: "s1", TransactionID: 42, ChargerID: "CHG-001", ConnectorID: 1}
	sr := &mockSR{activeSession: sess}
	cmd := NewCommander(cs, sr, slog.New(slog.NewTextHandler(io.Discard, nil)))

	err := cmd.SetChargingProfile("CHG-001", 0, 16)
	if err != nil {
		t.Fatalf("SetChargingProfile() = %v, want nil", err)
	}

	cs.mu.Lock()
	defer cs.mu.Unlock()
	if len(cs.calls) != 1 {
		t.Fatalf("calls = %d, want 1 (connectorID=0 skips TxProfile)", len(cs.calls))
	}
	if cs.calls[0].purpose != types.ChargingProfilePurposeTxDefaultProfile {
		t.Errorf("call[0] purpose = %s, want TxDefaultProfile", cs.calls[0].purpose)
	}
}

func TestCommander_NilSessionRepo_SKipsTxProfile(t *testing.T) {
	cs := &mockCS{}
	cmd := NewCommander(cs, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))

	err := cmd.SetChargingProfile("CHG-001", 1, 16)
	if err != nil {
		t.Fatalf("SetChargingProfile() = %v, want nil", err)
	}

	cs.mu.Lock()
	defer cs.mu.Unlock()
	if len(cs.calls) != 1 {
		t.Fatalf("calls = %d, want 1 (nil sessionRepo skips TxProfile)", len(cs.calls))
	}
}

func TestCommander_SessionRepoError_SKipsTxProfile(t *testing.T) {
	cs := &mockCS{}
	sr := &mockSR{err: context.DeadlineExceeded}
	cmd := NewCommander(cs, sr, slog.New(slog.NewTextHandler(io.Discard, nil)))

	err := cmd.SetChargingProfile("CHG-001", 1, 16)
	if err != nil {
		t.Fatalf("SetChargingProfile() = %v, want nil", err)
	}

	cs.mu.Lock()
	defer cs.mu.Unlock()
	if len(cs.calls) != 1 {
		t.Fatalf("calls = %d, want 1 (sessionRepo error skips TxProfile)", len(cs.calls))
	}
}
