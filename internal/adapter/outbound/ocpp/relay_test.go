package ocpp

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/xBlaz3kx/ocpp-go/ocpp1.6/core"
	"github.com/xBlaz3kx/ocpp-go/ocpp1.6/firmware"
	"github.com/xBlaz3kx/ocpp-go/ocpp1.6/types"

	"github.com/chiabcc/panya-charge-oss/internal/domain/proxy"
	"github.com/chiabcc/panya-charge-oss/internal/domain/session"
)

type fakeConfigRepo struct {
	mu   sync.Mutex
	cfgs map[string]*proxy.ProxyConfig
	err  error
}

func newFakeConfigRepo() *fakeConfigRepo {
	return &fakeConfigRepo{cfgs: make(map[string]*proxy.ProxyConfig)}
}

func (f *fakeConfigRepo) GetProxyConfig(_ context.Context, chargerID string) (*proxy.ProxyConfig, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	return f.cfgs[chargerID], nil
}

func (f *fakeConfigRepo) set(chargerID string, cfg *proxy.ProxyConfig) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cfgs[chargerID] = cfg
}

func discardedLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type stubCP struct {
	mu             sync.Mutex
	connected      bool
	bootCalls      int
	heartbeatCalls int
	authCalls      int
	statusCalls    int
	meterCalls     int
	startTxCalls   int
	stopTxCalls    int
	dataCalls      int
	fwCalls        int
	diagCalls      int
	stopCalls      int
}

func (s *stubCP) BootNotification(string, string, ...func(*core.BootNotificationRequest)) (*core.BootNotificationConfirmation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bootCalls++
	return &core.BootNotificationConfirmation{Interval: 60, Status: core.RegistrationStatusAccepted}, nil
}
func (s *stubCP) Heartbeat(...func(*core.HeartbeatRequest)) (*core.HeartbeatConfirmation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.heartbeatCalls++
	return &core.HeartbeatConfirmation{}, nil
}
func (s *stubCP) Authorize(string, ...func(*core.AuthorizeRequest)) (*core.AuthorizeConfirmation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.authCalls++
	return &core.AuthorizeConfirmation{}, nil
}
func (s *stubCP) StatusNotification(int, core.ChargePointErrorCode, core.ChargePointStatus, ...func(*core.StatusNotificationRequest)) (*core.StatusNotificationConfirmation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.statusCalls++
	return &core.StatusNotificationConfirmation{}, nil
}
func (s *stubCP) MeterValues(int, []types.MeterValue, ...func(*core.MeterValuesRequest)) (*core.MeterValuesConfirmation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.meterCalls++
	return &core.MeterValuesConfirmation{}, nil
}
func (s *stubCP) StartTransaction(int, string, int, *types.DateTime, ...func(*core.StartTransactionRequest)) (*core.StartTransactionConfirmation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.startTxCalls++
	return &core.StartTransactionConfirmation{TransactionId: 1}, nil
}
func (s *stubCP) StopTransaction(int, *types.DateTime, int, ...func(*core.StopTransactionRequest)) (*core.StopTransactionConfirmation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopTxCalls++
	return &core.StopTransactionConfirmation{}, nil
}
func (s *stubCP) DataTransfer(string, ...func(*core.DataTransferRequest)) (*core.DataTransferConfirmation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dataCalls++
	return &core.DataTransferConfirmation{}, nil
}
func (s *stubCP) FirmwareStatusNotification(firmware.FirmwareStatus, ...func(*firmware.FirmwareStatusNotificationRequest)) (*firmware.FirmwareStatusNotificationConfirmation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fwCalls++
	return &firmware.FirmwareStatusNotificationConfirmation{}, nil
}
func (s *stubCP) DiagnosticsStatusNotification(firmware.DiagnosticsStatus, ...func(*firmware.DiagnosticsStatusNotificationRequest)) (*firmware.DiagnosticsStatusNotificationConfirmation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.diagCalls++
	return &firmware.DiagnosticsStatusNotificationConfirmation{}, nil
}
func (s *stubCP) IsConnected() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.connected
}

func injectStub(relay *UpstreamRelay, chargerID string, cp *stubCP) {
	relay.mu.Lock()
	defer relay.mu.Unlock()
	relay.clients[chargerID] = &relayEntry{
		cp:   cp,
		stop: func() { cp.mu.Lock(); cp.stopCalls++; cp.mu.Unlock() },
	}
}

func TestUpstreamRelay_ConnectDisabled_NoConnection(t *testing.T) {
	repo := newFakeConfigRepo()
	repo.set("CP-1", &proxy.ProxyConfig{
		ChargerID:    "CP-1",
		ProxyEnabled: false,
		UpstreamURL:  "ws://localhost:19999",
	})
	relay := NewUpstreamRelay(repo, discardedLogger())

	if err := relay.Connect(context.Background(), "CP-1"); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if relay.IsConnected("CP-1") {
		t.Error("IsConnected = true, want false (proxy disabled)")
	}
}

func TestUpstreamRelay_ConnectMissingConfig_NoConnection(t *testing.T) {
	repo := newFakeConfigRepo()
	relay := NewUpstreamRelay(repo, discardedLogger())

	if err := relay.Connect(context.Background(), "CP-MISSING"); err != nil {
		t.Fatalf("Connect with missing config: %v", err)
	}
	if relay.IsConnected("CP-MISSING") {
		t.Error("IsConnected = true, want false (no config)")
	}
}

func TestUpstreamRelay_ForwardNoConnection_Error(t *testing.T) {
	repo := newFakeConfigRepo()
	relay := NewUpstreamRelay(repo, discardedLogger())

	err := relay.Forward(context.Background(), "CP-1", proxy.ActionHeartbeat, &core.HeartbeatRequest{})
	if err == nil {
		t.Fatal("Forward without connection should error")
	}
}

func TestUpstreamRelay_DisconnectIdempotent(t *testing.T) {
	repo := newFakeConfigRepo()
	relay := NewUpstreamRelay(repo, discardedLogger())

	relay.Disconnect("CP-NEVER")
	relay.Disconnect("CP-NEVER")
}

func TestUpstreamRelay_DisconnectCallsStop(t *testing.T) {
	repo := newFakeConfigRepo()
	relay := NewUpstreamRelay(repo, discardedLogger())
	cp := &stubCP{}
	injectStub(relay, "CP-1", cp)

	relay.Disconnect("CP-1")

	cp.mu.Lock()
	calls := cp.stopCalls
	cp.mu.Unlock()
	if calls != 1 {
		t.Errorf("stopCalls = %d, want 1", calls)
	}
	if relay.IsConnected("CP-1") {
		t.Error("IsConnected after disconnect = true")
	}
}

func TestUpstreamRelay_ForwardNilPayload_Error(t *testing.T) {
	relay := NewUpstreamRelay(newFakeConfigRepo(), discardedLogger())
	cp := &stubCP{}
	injectStub(relay, "CP-1", cp)

	err := relay.Forward(context.Background(), "CP-1", proxy.ActionHeartbeat, nil)
	if err == nil {
		t.Fatal("Forward with nil payload should error")
	}
}

func TestUpstreamRelay_ForwardUnsupportedType_Error(t *testing.T) {
	relay := NewUpstreamRelay(newFakeConfigRepo(), discardedLogger())
	cp := &stubCP{}
	injectStub(relay, "CP-1", cp)

	err := relay.Forward(context.Background(), "CP-1", "UnknownAction", "some string")
	if err == nil {
		t.Fatal("Forward with unsupported type should error")
	}
}

func TestUpstreamRelay_ForwardHeartbeat(t *testing.T) {
	relay := NewUpstreamRelay(newFakeConfigRepo(), discardedLogger())
	cp := &stubCP{}
	injectStub(relay, "CP-1", cp)

	if err := relay.Forward(context.Background(), "CP-1", proxy.ActionHeartbeat, &core.HeartbeatRequest{}); err != nil {
		t.Fatalf("Forward Heartbeat: %v", err)
	}
	cp.mu.Lock()
	defer cp.mu.Unlock()
	if cp.heartbeatCalls != 1 {
		t.Errorf("heartbeatCalls = %d, want 1", cp.heartbeatCalls)
	}
}

func TestUpstreamRelay_ForwardBootNotification(t *testing.T) {
	relay := NewUpstreamRelay(newFakeConfigRepo(), discardedLogger())
	cp := &stubCP{}
	injectStub(relay, "CP-1", cp)

	req := &core.BootNotificationRequest{
		ChargePointVendor: "ABB",
		ChargePointModel:  "Terra AC",
	}
	if err := relay.Forward(context.Background(), "CP-1", proxy.ActionBootNotification, req); err != nil {
		t.Fatalf("Forward BootNotification: %v", err)
	}
	cp.mu.Lock()
	defer cp.mu.Unlock()
	if cp.bootCalls != 1 {
		t.Errorf("bootCalls = %d, want 1", cp.bootCalls)
	}
}

func TestUpstreamRelay_ForwardStartTransaction(t *testing.T) {
	relay := NewUpstreamRelay(newFakeConfigRepo(), discardedLogger())
	cp := &stubCP{}
	injectStub(relay, "CP-1", cp)

	req := &core.StartTransactionRequest{
		ConnectorId: 1,
		IdTag:       "TAG1",
		MeterStart:  0,
	}
	if err := relay.Forward(context.Background(), "CP-1", proxy.ActionStartTransaction, req); err != nil {
		t.Fatalf("Forward StartTransaction: %v", err)
	}
	cp.mu.Lock()
	defer cp.mu.Unlock()
	if cp.startTxCalls != 1 {
		t.Errorf("startTxCalls = %d, want 1", cp.startTxCalls)
	}
}

func TestUpstreamRelay_ForwardStopTransaction(t *testing.T) {
	relay := NewUpstreamRelay(newFakeConfigRepo(), discardedLogger())
	cp := &stubCP{}
	injectStub(relay, "CP-1", cp)

	req := &core.StopTransactionRequest{
		TransactionId: 42,
		MeterStop:     1000,
	}
	if err := relay.Forward(context.Background(), "CP-1", proxy.ActionStopTransaction, req); err != nil {
		t.Fatalf("Forward StopTransaction: %v", err)
	}
	cp.mu.Lock()
	defer cp.mu.Unlock()
	if cp.stopTxCalls != 1 {
		t.Errorf("stopTxCalls = %d, want 1", cp.stopTxCalls)
	}
}

func TestUpstreamRelay_ForwardStatusNotification(t *testing.T) {
	relay := NewUpstreamRelay(newFakeConfigRepo(), discardedLogger())
	cp := &stubCP{}
	injectStub(relay, "CP-1", cp)

	req := &core.StatusNotificationRequest{
		ConnectorId: 1,
		ErrorCode:   core.NoError,
		Status:      core.ChargePointStatusCharging,
	}
	if err := relay.Forward(context.Background(), "CP-1", proxy.ActionStatusNotification, req); err != nil {
		t.Fatalf("Forward StatusNotification: %v", err)
	}
	cp.mu.Lock()
	defer cp.mu.Unlock()
	if cp.statusCalls != 1 {
		t.Errorf("statusCalls = %d, want 1", cp.statusCalls)
	}
}

func TestUpstreamRelay_ForwardMeterValues(t *testing.T) {
	relay := NewUpstreamRelay(newFakeConfigRepo(), discardedLogger())
	cp := &stubCP{}
	injectStub(relay, "CP-1", cp)

	req := &core.MeterValuesRequest{
		ConnectorId: 1,
		MeterValue: []types.MeterValue{
			{Timestamp: types.NewDateTime(time.Now()), SampledValue: []types.SampledValue{{Value: "10000", Measurand: types.MeasurandPowerActiveImport, Unit: "W"}}},
		},
	}
	if err := relay.Forward(context.Background(), "CP-1", proxy.ActionMeterValues, req); err != nil {
		t.Fatalf("Forward MeterValues: %v", err)
	}
	cp.mu.Lock()
	defer cp.mu.Unlock()
	if cp.meterCalls != 1 {
		t.Errorf("meterCalls = %d, want 1", cp.meterCalls)
	}
}

func TestUpstreamRelay_ForwardAuthorize(t *testing.T) {
	relay := NewUpstreamRelay(newFakeConfigRepo(), discardedLogger())
	cp := &stubCP{}
	injectStub(relay, "CP-1", cp)

	if err := relay.Forward(context.Background(), "CP-1", proxy.ActionAuthorize, &core.AuthorizeRequest{IdTag: "TAG1"}); err != nil {
		t.Fatalf("Forward Authorize: %v", err)
	}
	cp.mu.Lock()
	defer cp.mu.Unlock()
	if cp.authCalls != 1 {
		t.Errorf("authCalls = %d, want 1", cp.authCalls)
	}
}

func TestUpstreamRelay_ForwardDataTransfer(t *testing.T) {
	relay := NewUpstreamRelay(newFakeConfigRepo(), discardedLogger())
	cp := &stubCP{}
	injectStub(relay, "CP-1", cp)

	if err := relay.Forward(context.Background(), "CP-1", proxy.ActionDataTransfer, &core.DataTransferRequest{VendorId: "ABB"}); err != nil {
		t.Fatalf("Forward DataTransfer: %v", err)
	}
	cp.mu.Lock()
	defer cp.mu.Unlock()
	if cp.dataCalls != 1 {
		t.Errorf("dataCalls = %d, want 1", cp.dataCalls)
	}
}

func TestUpstreamRelay_ForwardFirmwareStatus(t *testing.T) {
	relay := NewUpstreamRelay(newFakeConfigRepo(), discardedLogger())
	cp := &stubCP{}
	injectStub(relay, "CP-1", cp)

	if err := relay.Forward(context.Background(), "CP-1", proxy.ActionFirmwareStatusNotification, &firmware.FirmwareStatusNotificationRequest{Status: firmware.FirmwareStatusDownloaded}); err != nil {
		t.Fatalf("Forward FirmwareStatusNotification: %v", err)
	}
	cp.mu.Lock()
	defer cp.mu.Unlock()
	if cp.fwCalls != 1 {
		t.Errorf("fwCalls = %d, want 1", cp.fwCalls)
	}
}

func TestUpstreamRelay_ForwardDiagnosticsStatus(t *testing.T) {
	relay := NewUpstreamRelay(newFakeConfigRepo(), discardedLogger())
	cp := &stubCP{}
	injectStub(relay, "CP-1", cp)

	if err := relay.Forward(context.Background(), "CP-1", proxy.ActionDiagnosticsStatusNotification, &firmware.DiagnosticsStatusNotificationRequest{Status: firmware.DiagnosticsStatusIdle}); err != nil {
		t.Fatalf("Forward DiagnosticsStatusNotification: %v", err)
	}
	cp.mu.Lock()
	defer cp.mu.Unlock()
	if cp.diagCalls != 1 {
		t.Errorf("diagCalls = %d, want 1", cp.diagCalls)
	}
}

func TestUpstreamRelay_ConcurrentForward(t *testing.T) {
	relay := NewUpstreamRelay(newFakeConfigRepo(), discardedLogger())
	cp := &stubCP{}
	injectStub(relay, "CP-1", cp)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = relay.Forward(context.Background(), "CP-1", proxy.ActionHeartbeat, &core.HeartbeatRequest{})
		}()
	}
	wg.Wait()

	cp.mu.Lock()
	defer cp.mu.Unlock()
	if cp.heartbeatCalls != 20 {
		t.Errorf("heartbeatCalls = %d, want 20", cp.heartbeatCalls)
	}
}

type stubDownstream struct {
	mu          sync.Mutex
	startCalls  int
	stopCalls   int
	startErr    error
	stopErr     error
	lastCharger string
	lastConnID  int
	lastIDTag   string
	lastTxID    int
}

func (s *stubDownstream) RemoteStartTransaction(chargerID string, connectorID int, idTag string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.startCalls++
	s.lastCharger = chargerID
	s.lastConnID = connectorID
	s.lastIDTag = idTag
	return s.startErr
}

func (s *stubDownstream) RemoteStopTransaction(chargerID string, transactionID int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopCalls++
	s.lastCharger = chargerID
	s.lastTxID = transactionID
	return s.stopErr
}

type stubSessionRepo struct {
	mu      sync.Mutex
	session *session.Session
	err     error
}

func (s *stubSessionRepo) GetActiveSession(_ context.Context, _ string, _ int) (*session.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.session, s.err
}

func TestRelayCoreHandler_RemoteStartTransaction_Accepted(t *testing.T) {
	cmdr := &stubDownstream{}
	handler := &relayCoreHandler{
		chargerID: "CP-1",
		commander: cmdr,
		sessions:  &stubSessionRepo{},
		logger:    discardedLogger(),
	}

	connID := 2
	conf, err := handler.OnRemoteStartTransaction(&core.RemoteStartTransactionRequest{
		ConnectorId: &connID,
		IdTag:       "TAG123",
	})
	if err != nil {
		t.Fatalf("OnRemoteStartTransaction error: %v", err)
	}
	if conf.Status != types.RemoteStartStopStatusAccepted {
		t.Errorf("status = %s, want Accepted", conf.Status)
	}
	cmdr.mu.Lock()
	defer cmdr.mu.Unlock()
	if cmdr.startCalls != 1 {
		t.Errorf("startCalls = %d, want 1", cmdr.startCalls)
	}
	if cmdr.lastIDTag != "TAG123" {
		t.Errorf("lastIDTag = %q, want TAG123", cmdr.lastIDTag)
	}
	if cmdr.lastConnID != 2 {
		t.Errorf("lastConnID = %d, want 2", cmdr.lastConnID)
	}
}

func TestRelayCoreHandler_RemoteStartTransaction_DefaultConnectorAndIdTag(t *testing.T) {
	cmdr := &stubDownstream{}
	handler := &relayCoreHandler{
		chargerID: "CP-1",
		commander: cmdr,
		sessions:  &stubSessionRepo{},
		logger:    discardedLogger(),
	}

	conf, err := handler.OnRemoteStartTransaction(&core.RemoteStartTransactionRequest{})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if conf.Status != types.RemoteStartStopStatusAccepted {
		t.Errorf("status = %s, want Accepted", conf.Status)
	}
	cmdr.mu.Lock()
	defer cmdr.mu.Unlock()
	if cmdr.lastConnID != 1 {
		t.Errorf("lastConnID = %d, want 1 (default)", cmdr.lastConnID)
	}
	if cmdr.lastIDTag != "upstream" {
		t.Errorf("lastIDTag = %q, want 'upstream'", cmdr.lastIDTag)
	}
}

func TestRelayCoreHandler_RemoteStartTransaction_CommanderError_Rejected(t *testing.T) {
	cmdr := &stubDownstream{startErr: errors.New("contactor cooldown")}
	handler := &relayCoreHandler{
		chargerID: "CP-1",
		commander: cmdr,
		sessions:  &stubSessionRepo{},
		logger:    discardedLogger(),
	}

	conf, err := handler.OnRemoteStartTransaction(&core.RemoteStartTransactionRequest{IdTag: "TAG1"})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if conf.Status != types.RemoteStartStopStatusRejected {
		t.Errorf("status = %s, want Rejected", conf.Status)
	}
}

func TestRelayCoreHandler_RemoteStopTransaction_Accepted(t *testing.T) {
	cmdr := &stubDownstream{}
	activeSession := &session.Session{TransactionID: 42}
	handler := &relayCoreHandler{
		chargerID: "CP-1",
		commander: cmdr,
		sessions:  &stubSessionRepo{session: activeSession},
		logger:    discardedLogger(),
	}

	conf, err := handler.OnRemoteStopTransaction(&core.RemoteStopTransactionRequest{TransactionId: 999})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if conf.Status != types.RemoteStartStopStatusAccepted {
		t.Errorf("status = %s, want Accepted", conf.Status)
	}
	cmdr.mu.Lock()
	defer cmdr.mu.Unlock()
	if cmdr.stopCalls != 1 {
		t.Errorf("stopCalls = %d, want 1", cmdr.stopCalls)
	}
	if cmdr.lastTxID != 42 {
		t.Errorf("lastTxID = %d, want 42 (local tx id)", cmdr.lastTxID)
	}
}

func TestRelayCoreHandler_RemoteStopTransaction_NoActiveSession_Rejected(t *testing.T) {
	cmdr := &stubDownstream{}
	handler := &relayCoreHandler{
		chargerID: "CP-1",
		commander: cmdr,
		sessions:  &stubSessionRepo{},
		logger:    discardedLogger(),
	}

	conf, err := handler.OnRemoteStopTransaction(&core.RemoteStopTransactionRequest{TransactionId: 1})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if conf.Status != types.RemoteStartStopStatusRejected {
		t.Errorf("status = %s, want Rejected", conf.Status)
	}
	cmdr.mu.Lock()
	defer cmdr.mu.Unlock()
	if cmdr.stopCalls != 0 {
		t.Errorf("stopCalls = %d, want 0", cmdr.stopCalls)
	}
}

func TestRelayCoreHandler_RemoteStopTransaction_CommanderError_Rejected(t *testing.T) {
	cmdr := &stubDownstream{stopErr: errors.New("contactor cooldown")}
	activeSession := &session.Session{TransactionID: 7}
	handler := &relayCoreHandler{
		chargerID: "CP-1",
		commander: cmdr,
		sessions:  &stubSessionRepo{session: activeSession},
		logger:    discardedLogger(),
	}

	conf, err := handler.OnRemoteStopTransaction(&core.RemoteStopTransactionRequest{TransactionId: 100})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if conf.Status != types.RemoteStartStopStatusRejected {
		t.Errorf("status = %s, want Rejected", conf.Status)
	}
}

func TestRelayCoreHandler_OnReset_Accepted(t *testing.T) {
	handler := &relayCoreHandler{
		chargerID: "CP-1",
		commander: &stubDownstream{},
		sessions:  &stubSessionRepo{},
		logger:    discardedLogger(),
	}
	conf, err := handler.OnReset(&core.ResetRequest{Type: core.ResetTypeSoft})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if conf.Status != core.ResetStatusAccepted {
		t.Errorf("status = %s, want Accepted", conf.Status)
	}
}

func TestRelayCoreHandler_OnChangeAvailability_Accepted(t *testing.T) {
	handler := &relayCoreHandler{
		chargerID: "CP-1",
		commander: &stubDownstream{},
		sessions:  &stubSessionRepo{},
		logger:    discardedLogger(),
	}
	conf, err := handler.OnChangeAvailability(&core.ChangeAvailabilityRequest{
		ConnectorId: 1,
		Type:        core.AvailabilityTypeOperative,
	})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if conf.Status != core.AvailabilityStatusAccepted {
		t.Errorf("status = %s, want Accepted", conf.Status)
	}
}

func TestRelayCoreHandler_OnGetConfiguration_Empty(t *testing.T) {
	handler := &relayCoreHandler{
		chargerID: "CP-1",
		commander: &stubDownstream{},
		sessions:  &stubSessionRepo{},
		logger:    discardedLogger(),
	}
	conf, err := handler.OnGetConfiguration(&core.GetConfigurationRequest{})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if conf == nil {
		t.Fatal("conf = nil")
	}
}

func TestRelayCoreHandler_OnDataTransfer_Accepted(t *testing.T) {
	handler := &relayCoreHandler{
		chargerID: "CP-1",
		commander: &stubDownstream{},
		sessions:  &stubSessionRepo{},
		logger:    discardedLogger(),
	}
	conf, err := handler.OnDataTransfer(&core.DataTransferRequest{VendorId: "ABB"})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if conf.Status != core.DataTransferStatusAccepted {
		t.Errorf("status = %s, want Accepted", conf.Status)
	}
}

func TestRelayCoreHandler_OnClearCache_Accepted(t *testing.T) {
	handler := &relayCoreHandler{
		chargerID: "CP-1",
		commander: &stubDownstream{},
		sessions:  &stubSessionRepo{},
		logger:    discardedLogger(),
	}
	conf, err := handler.OnClearCache(&core.ClearCacheRequest{})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if conf.Status != core.ClearCacheStatusAccepted {
		t.Errorf("status = %s, want Accepted", conf.Status)
	}
}

func TestRelayCoreHandler_OnChangeConfiguration_Accepted(t *testing.T) {
	handler := &relayCoreHandler{
		chargerID: "CP-1",
		commander: &stubDownstream{},
		sessions:  &stubSessionRepo{},
		logger:    discardedLogger(),
	}
	conf, err := handler.OnChangeConfiguration(&core.ChangeConfigurationRequest{Key: "foo", Value: "bar"})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if conf.Status != core.ConfigurationStatusAccepted {
		t.Errorf("status = %s, want Accepted", conf.Status)
	}
}

func TestRelayCoreHandler_OnUnlockConnector_NotSupported(t *testing.T) {
	handler := &relayCoreHandler{
		chargerID: "CP-1",
		commander: &stubDownstream{},
		sessions:  &stubSessionRepo{},
		logger:    discardedLogger(),
	}
	conf, err := handler.OnUnlockConnector(&core.UnlockConnectorRequest{ConnectorId: 1})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if conf.Status != core.UnlockStatusNotSupported {
		t.Errorf("status = %s, want NotSupported", conf.Status)
	}
}

func TestUpstreamRelay_WithDownstream(t *testing.T) {
	relay := NewUpstreamRelay(newFakeConfigRepo(), discardedLogger())
	cmdr := &stubDownstream{}
	sessions := &stubSessionRepo{}
	relay.WithDownstream(cmdr, sessions)

	if relay.commander == nil {
		t.Error("commander not set")
	}
	if relay.sessions == nil {
		t.Error("sessions not set")
	}
}
