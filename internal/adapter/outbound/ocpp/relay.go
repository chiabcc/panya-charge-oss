package ocpp

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	ocpp16 "github.com/xBlaz3kx/ocpp-go/ocpp1.6"
	"github.com/xBlaz3kx/ocpp-go/ocpp1.6/core"
	"github.com/xBlaz3kx/ocpp-go/ocpp1.6/firmware"
	"github.com/xBlaz3kx/ocpp-go/ocpp1.6/types"
	"github.com/xBlaz3kx/ocpp-go/ws"

	"github.com/chiabcc/panya-charge-oss/internal/domain/proxy"
	"github.com/chiabcc/panya-charge-oss/internal/domain/session"
)

type chargePointClient interface {
	BootNotification(model, vendor string, props ...func(*core.BootNotificationRequest)) (*core.BootNotificationConfirmation, error)
	Heartbeat(props ...func(*core.HeartbeatRequest)) (*core.HeartbeatConfirmation, error)
	Authorize(idTag string, props ...func(*core.AuthorizeRequest)) (*core.AuthorizeConfirmation, error)
	StatusNotification(connectorId int, errorCode core.ChargePointErrorCode, status core.ChargePointStatus, props ...func(*core.StatusNotificationRequest)) (*core.StatusNotificationConfirmation, error)
	MeterValues(connectorId int, meterValues []types.MeterValue, props ...func(*core.MeterValuesRequest)) (*core.MeterValuesConfirmation, error)
	StartTransaction(connectorId int, idTag string, meterStart int, timestamp *types.DateTime, props ...func(*core.StartTransactionRequest)) (*core.StartTransactionConfirmation, error)
	StopTransaction(meterStop int, timestamp *types.DateTime, transactionId int, props ...func(*core.StopTransactionRequest)) (*core.StopTransactionConfirmation, error)
	DataTransfer(vendorId string, props ...func(*core.DataTransferRequest)) (*core.DataTransferConfirmation, error)
	FirmwareStatusNotification(status firmware.FirmwareStatus, props ...func(*firmware.FirmwareStatusNotificationRequest)) (*firmware.FirmwareStatusNotificationConfirmation, error)
	DiagnosticsStatusNotification(status firmware.DiagnosticsStatus, props ...func(*firmware.DiagnosticsStatusNotificationRequest)) (*firmware.DiagnosticsStatusNotificationConfirmation, error)
	IsConnected() bool
}

var _ chargePointClient = (ocpp16.ChargePoint)(nil)

type relayEntry struct {
	cp      chargePointClient
	stopOne sync.Once
	stop    func()
}

type DownstreamCommander interface {
	RemoteStartTransaction(chargerID string, connectorID int, idTag string) error
	RemoteStopTransaction(chargerID string, transactionID int) error
}

type activeSessionRepo interface {
	GetActiveSession(ctx context.Context, chargerID string, connectorID int) (*session.Session, error)
}

type UpstreamRelay struct {
	mu            sync.RWMutex
	clients       map[string]*relayEntry
	repo          proxyConfigReader
	commander     DownstreamCommander
	sessions      activeSessionRepo
	logger        *slog.Logger
	mvMu          sync.Mutex
	mvLastSent    map[string]time.Time
	OnStateChange func(chargerID string, connected bool)
}

type proxyConfigReader interface {
	GetProxyConfig(ctx context.Context, chargerID string) (*proxy.ProxyConfig, error)
}

func NewUpstreamRelay(repo proxyConfigReader, logger *slog.Logger) *UpstreamRelay {
	return &UpstreamRelay{
		clients:    make(map[string]*relayEntry),
		mvLastSent: make(map[string]time.Time),
		repo:       repo,
		logger:     logger,
	}
}

func (r *UpstreamRelay) WithDownstream(cmdr DownstreamCommander, sessions activeSessionRepo) *UpstreamRelay {
	r.commander = cmdr
	r.sessions = sessions
	return r
}

func (r *UpstreamRelay) Connect(ctx context.Context, chargerID string) error {
	cfg, err := r.repo.GetProxyConfig(ctx, chargerID)
	if err != nil {
		return fmt.Errorf("relay: get proxy config for %s: %w", chargerID, err)
	}
	if cfg == nil || !cfg.ProxyEnabled || cfg.UpstreamURL == "" {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.clients[chargerID]; exists {
		return nil
	}

	wsClient := ws.NewClient()
	wsClient.SetTimeoutConfig(ws.ClientTimeoutConfig{
		RetryBackOffRepeatTimes: 0,
		RetryBackOffRandomRange: 5,
		RetryBackOffWaitMinimum: 30 * time.Second,
	})
	if cfg.UpstreamUser != "" {
		wsClient.SetBasicAuth(cfg.UpstreamUser, string(cfg.UpstreamPasswordEnc))
	}
	wsClient.SetDisconnectedHandler(func(err error) {
		r.logger.Warn("relay: upstream disconnected", "charger", chargerID, "err", err)
		if r.OnStateChange != nil {
			r.OnStateChange(chargerID, false)
		}
	})
	wsClient.SetReconnectedHandler(func() {
		r.logger.Info("relay: upstream reconnected", "charger", chargerID)
		if r.OnStateChange != nil {
			r.OnStateChange(chargerID, true)
		}
	})

	cp, err := ocpp16.NewChargePoint(chargerID, nil, wsClient, nil)
	if err != nil {
		return fmt.Errorf("relay: create charge point for %s: %w", chargerID, err)
	}

	if r.commander != nil {
		handler := &relayCoreHandler{
			chargerID: chargerID,
			commander: r.commander,
			sessions:  r.sessions,
			logger:    r.logger,
		}
		cp.SetCoreHandler(handler)
	}

	if err := cp.Start(cfg.UpstreamURL); err != nil {
		return fmt.Errorf("relay: start upstream connection for %s: %w", chargerID, err)
	}

	entry := &relayEntry{cp: cp, stop: cp.Stop}
	r.clients[chargerID] = entry
	r.logger.Info("relay: upstream connected",
		"charger", chargerID,
		"upstream_url", cfg.UpstreamURL,
	)
	if r.OnStateChange != nil {
		r.OnStateChange(chargerID, true)
	}
	return nil
}

func (r *UpstreamRelay) Disconnect(chargerID string) {
	r.mu.Lock()
	entry, ok := r.clients[chargerID]
	if ok {
		delete(r.clients, chargerID)
	}
	r.mu.Unlock()

	if !ok {
		return
	}

	entry.stopOne.Do(func() {
		entry.stop()
	})

	r.logger.Info("relay: upstream disconnected", "charger", chargerID)
	if r.OnStateChange != nil {
		r.OnStateChange(chargerID, false)
	}
}

func (r *UpstreamRelay) IsConnected(chargerID string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entry, ok := r.clients[chargerID]
	if !ok {
		return false
	}
	return entry.cp.IsConnected()
}

func (r *UpstreamRelay) Close() {
	r.mu.Lock()
	ids := make([]string, 0, len(r.clients))
	for id := range r.clients {
		ids = append(ids, id)
	}
	r.mu.Unlock()

	for _, id := range ids {
		r.Disconnect(id)
	}
}

func (r *UpstreamRelay) Forward(ctx context.Context, chargerID string, action string, payload any) error {
	r.mu.RLock()
	entry, ok := r.clients[chargerID]
	r.mu.RUnlock()

	if !ok {
		return fmt.Errorf("relay: no upstream connection for %s", chargerID)
	}

	if action == "MeterValues" && r.shouldThrottleMV(ctx, chargerID) {
		r.logger.Debug("relay: meter values throttled", "charger", chargerID)
		return nil
	}

	err := r.forward(entry.cp, action, payload)

	if err != nil {
		return fmt.Errorf("relay: forward %s for %s: %w", action, chargerID, err)
	}
	return nil
}

func (r *UpstreamRelay) shouldThrottleMV(ctx context.Context, chargerID string) bool {
	cfg, err := r.repo.GetProxyConfig(ctx, chargerID)
	if err != nil || cfg == nil || cfg.UpstreamThrottleMVPerMin <= 0 {
		return false
	}

	minInterval := time.Duration(60/cfg.UpstreamThrottleMVPerMin) * time.Second

	r.mvMu.Lock()
	defer r.mvMu.Unlock()

	last, exists := r.mvLastSent[chargerID]
	now := time.Now()
	if exists && now.Sub(last) < minInterval {
		return true
	}
	r.mvLastSent[chargerID] = now
	return false
}

func (r *UpstreamRelay) forward(cp chargePointClient, action string, payload any) error {
	if payload == nil {
		return fmt.Errorf("relay: nil payload for %s", action)
	}

	switch req := payload.(type) {
	case *core.BootNotificationRequest:
		_, err := cp.BootNotification(req.ChargePointModel, req.ChargePointVendor,
			func(r *core.BootNotificationRequest) { *r = *req },
		)
		return err
	case *core.HeartbeatRequest:
		_, err := cp.Heartbeat()
		return err
	case *core.AuthorizeRequest:
		_, err := cp.Authorize(req.IdTag,
			func(r *core.AuthorizeRequest) { *r = *req },
		)
		return err
	case *core.StatusNotificationRequest:
		_, err := cp.StatusNotification(req.ConnectorId, req.ErrorCode, req.Status,
			func(r *core.StatusNotificationRequest) { *r = *req },
		)
		return err
	case *core.MeterValuesRequest:
		_, err := cp.MeterValues(req.ConnectorId, req.MeterValue,
			func(r *core.MeterValuesRequest) { *r = *req },
		)
		return err
	case *core.StartTransactionRequest:
		ts := req.Timestamp
		if ts == nil {
			ts = types.NewDateTime(time.Now())
		}
		_, err := cp.StartTransaction(req.ConnectorId, req.IdTag, req.MeterStart, ts,
			func(r *core.StartTransactionRequest) { *r = *req },
		)
		return err
	case *core.StopTransactionRequest:
		ts := req.Timestamp
		if ts == nil {
			ts = types.NewDateTime(time.Now())
		}
		_, err := cp.StopTransaction(req.MeterStop, ts, req.TransactionId,
			func(r *core.StopTransactionRequest) { *r = *req },
		)
		return err
	case *core.DataTransferRequest:
		_, err := cp.DataTransfer(req.VendorId,
			func(r *core.DataTransferRequest) { *r = *req },
		)
		return err
	case *firmware.FirmwareStatusNotificationRequest:
		_, err := cp.FirmwareStatusNotification(req.Status,
			func(r *firmware.FirmwareStatusNotificationRequest) { *r = *req },
		)
		return err
	case *firmware.DiagnosticsStatusNotificationRequest:
		_, err := cp.DiagnosticsStatusNotification(req.Status,
			func(r *firmware.DiagnosticsStatusNotificationRequest) { *r = *req },
		)
		return err
	default:
		return fmt.Errorf("relay: unsupported payload type %T for action %s", payload, action)
	}
}

type relayCoreHandler struct {
	chargerID string
	commander DownstreamCommander
	sessions  activeSessionRepo
	logger    *slog.Logger
}

func (h *relayCoreHandler) OnRemoteStartTransaction(req *core.RemoteStartTransactionRequest) (*core.RemoteStartTransactionConfirmation, error) {
	connectorID := 1
	if req.ConnectorId != nil {
		connectorID = *req.ConnectorId
	}
	idTag := req.IdTag
	if idTag == "" {
		idTag = "upstream"
	}

	if err := h.commander.RemoteStartTransaction(h.chargerID, connectorID, idTag); err != nil {
		h.logger.Error("relay: downstream remote start failed", "charger", h.chargerID, "err", err)
		return core.NewRemoteStartTransactionConfirmation(types.RemoteStartStopStatusRejected), nil
	}
	h.logger.Info("relay: forwarded remote start downstream", "charger", h.chargerID, "connector", connectorID)
	return core.NewRemoteStartTransactionConfirmation(types.RemoteStartStopStatusAccepted), nil
}

func (h *relayCoreHandler) OnRemoteStopTransaction(req *core.RemoteStopTransactionRequest) (*core.RemoteStopTransactionConfirmation, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	active, err := h.sessions.GetActiveSession(ctx, h.chargerID, 1)
	if err != nil || active == nil {
		h.logger.Warn("relay: no active session for downstream remote stop", "charger", h.chargerID, "err", err)
		return core.NewRemoteStopTransactionConfirmation(types.RemoteStartStopStatusRejected), nil
	}

	if err := h.commander.RemoteStopTransaction(h.chargerID, active.TransactionID); err != nil {
		h.logger.Error("relay: downstream remote stop failed", "charger", h.chargerID, "err", err)
		return core.NewRemoteStopTransactionConfirmation(types.RemoteStartStopStatusRejected), nil
	}
	h.logger.Info("relay: forwarded remote stop downstream", "charger", h.chargerID, "tx", active.TransactionID)
	return core.NewRemoteStopTransactionConfirmation(types.RemoteStartStopStatusAccepted), nil
}

func (h *relayCoreHandler) OnChangeAvailability(req *core.ChangeAvailabilityRequest) (*core.ChangeAvailabilityConfirmation, error) {
	return core.NewChangeAvailabilityConfirmation(core.AvailabilityStatusAccepted), nil
}

func (h *relayCoreHandler) OnChangeConfiguration(req *core.ChangeConfigurationRequest) (*core.ChangeConfigurationConfirmation, error) {
	return core.NewChangeConfigurationConfirmation(core.ConfigurationStatusAccepted), nil
}

func (h *relayCoreHandler) OnClearCache(req *core.ClearCacheRequest) (*core.ClearCacheConfirmation, error) {
	return core.NewClearCacheConfirmation(core.ClearCacheStatusAccepted), nil
}

func (h *relayCoreHandler) OnDataTransfer(req *core.DataTransferRequest) (*core.DataTransferConfirmation, error) {
	return core.NewDataTransferConfirmation(core.DataTransferStatusAccepted), nil
}

func (h *relayCoreHandler) OnGetConfiguration(req *core.GetConfigurationRequest) (*core.GetConfigurationConfirmation, error) {
	return core.NewGetConfigurationConfirmation(nil), nil
}

func (h *relayCoreHandler) OnReset(req *core.ResetRequest) (*core.ResetConfirmation, error) {
	h.logger.Info("relay: reset requested from upstream", "charger", h.chargerID, "type", req.Type)
	return core.NewResetConfirmation(core.ResetStatusAccepted), nil
}

func (h *relayCoreHandler) OnUnlockConnector(req *core.UnlockConnectorRequest) (*core.UnlockConnectorConfirmation, error) {
	return core.NewUnlockConnectorConfirmation(core.UnlockStatusNotSupported), nil
}
