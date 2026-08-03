package ocpp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/xBlaz3kx/ocpp-go/ocpp1.6/core"
	"github.com/xBlaz3kx/ocpp-go/ocpp1.6/types"

	"github.com/chiabcc/panya-charge-oss/internal/domain/charger"
	"github.com/chiabcc/panya-charge-oss/internal/domain/ports"
	"github.com/chiabcc/panya-charge-oss/internal/domain/session"
	"github.com/chiabcc/panya-charge-oss/pkg/csms"
)

type ChargerRelayHook interface {
	Connect(ctx context.Context, chargerID string) error
	Disconnect(chargerID string)
}

// OverrideClearer exposes manual-override clearing for the controller.
type OverrideClearer interface {
	ClearManualOverride(chargerID string)
}

type LiveBroadcaster interface {
	UpdateMeter(chargerID string, powerW, energyKWh, currentA float64)
	PublishStatus(chargerID, status string)
}

type MetricsRecorder interface {
	RecordInbound(msgType string, duration time.Duration, err error)
	RecordSessionStart()
	RecordSessionStop()
	RecordChargerConnected()
	RecordChargerDisconnected()
	RecordChargePowerWatts(chargerID string, watts float64)
}

type OcppMessageBroker interface {
	Add(msg OcppMessage)
}

type OcppMessage struct {
	Time      time.Time
	ChargerID string
	Direction string
	Action    string
	Payload   map[string]any
}

// EventEmitter multicasts CSMS domain events to subscribers. It is satisfied
// by *csms.Emitter and is optional on the Handler/Controller — when nil, emit
// calls are no-ops so existing tests and standalone deployments are unaffected.
type EventEmitter interface {
	Emit(ev csms.Event)
}

type Handler struct {
	router          *Router
	chargerRepo     ports.ChargerRepository
	sessionRepo     ports.SessionRepository
	meterRepo       ports.MeterRepository
	proxyConfigRepo ports.ProxyConfigRepository
	publisher       ports.EventPublisher
	discovery       ports.DiscoveryPublisher
	relayHook       ChargerRelayHook
	broadcaster     LiveBroadcaster
	ocppBroker      OcppMessageBroker
	emitter         EventEmitter
	overrideClearer OverrideClearer
	ampMu           sync.RWMutex
	minAmps         int
	maxAmps         int
	logger          *slog.Logger
	metrics         MetricsRecorder
	activeConns     atomic.Int32
}

func NewHandler(
	router *Router,
	cr ports.ChargerRepository,
	sr ports.SessionRepository,
	mr ports.MeterRepository,
	pr ports.ProxyConfigRepository,
	pub ports.EventPublisher,
	discovery ports.DiscoveryPublisher,
	relayHook ChargerRelayHook,
	minAmps, maxAmps int,
	logger *slog.Logger,
	metrics MetricsRecorder,
) *Handler {
	return &Handler{
		router:          router,
		chargerRepo:     cr,
		sessionRepo:     sr,
		meterRepo:       mr,
		proxyConfigRepo: pr,
		publisher:       pub,
		discovery:       discovery,
		relayHook:       relayHook,
		minAmps:         minAmps,
		maxAmps:         maxAmps,
		logger:          logger,
		metrics:         metrics,
	}
}

// SetOverrideClearer wires the controller's override clearing into the handler
// so that StopTransaction and Disconnect events can lift manual overrides.
func (h *Handler) SetOverrideClearer(oc OverrideClearer) {
	h.overrideClearer = oc
}

func (h *Handler) SetLiveBroadcaster(b LiveBroadcaster) {
	h.broadcaster = b
}

func (h *Handler) SetOcppBroker(b OcppMessageBroker) {
	h.ocppBroker = b
}

// SetMinMax updates the min/max amps bounds for discovery publishing.
func (h *Handler) SetMinMax(minAmps, maxAmps int) {
	h.ampMu.Lock()
	defer h.ampMu.Unlock()
	h.minAmps = minAmps
	h.maxAmps = maxAmps
}

func (h *Handler) getAmpBounds() (min, max int) {
	h.ampMu.RLock()
	defer h.ampMu.RUnlock()
	return h.minAmps, h.maxAmps
}

func (h *Handler) SetEmitter(e EventEmitter) {
	h.emitter = e
}

func (h *Handler) emit(ev csms.Event) {
	if h.emitter == nil {
		return
	}
	h.emitter.Emit(ev)
}

func (h *Handler) recordInbound(msgType string, start time.Time, err error) {
	if h.metrics == nil {
		return
	}
	h.metrics.RecordInbound(msgType, time.Since(start), err)
}

func (h *Handler) publishOcppMessage(chargerID, direction, action string, connector int) {
	if h.ocppBroker == nil {
		return
	}
	payload := map[string]any{}
	if connector > 0 {
		payload["connector_id"] = connector
	}
	h.ocppBroker.Add(OcppMessage{
		Time:      time.Now(),
		ChargerID: chargerID,
		Direction: direction,
		Action:    action,
		Payload:   payload,
	})
}

func (h *Handler) OnBootNotification(chargePointId string, req *core.BootNotificationRequest) (*core.BootNotificationConfirmation, error) {
	start := time.Now()
	h.router.RouteBootNotification(chargePointId, req)
	h.publishOcppMessage(chargePointId, "inbound", "BootNotification", 0)
	h.logger.Info("boot notification",
		"charger", chargePointId,
		"vendor", req.ChargePointVendor,
		"model", req.ChargePointModel,
		"firmware", req.FirmwareVersion,
		"serial", req.ChargePointSerialNumber,
	)

	c := charger.Charger{
		ID:              chargePointId,
		Vendor:          req.ChargePointVendor,
		Model:           req.ChargePointModel,
		FirmwareVersion: req.FirmwareVersion,
		SerialNumber:    req.ChargePointSerialNumber,
		Online:          true,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := h.chargerRepo.UpsertCharger(ctx, c); err != nil {
		h.logger.Error("failed to upsert charger on boot", "err", err, "charger", chargePointId)
	}

	h.detectHardwareProfile(chargePointId, req.ChargePointVendor, req.ChargePointModel, req.FirmwareVersion)

	if h.discovery != nil {
		proxyEnabled := h.isProxyEnabled(ctx, chargePointId)
		min, max := h.getAmpBounds()
		h.discovery.PublishDiscovery(c, min, max, proxyEnabled)
	}

	h.recordInbound("BootNotification", start, nil)
	h.emit(csms.ChargerConnected{
		Timestamp:       time.Now(),
		ChargerID:       chargePointId,
		Vendor:          req.ChargePointVendor,
		Model:           req.ChargePointModel,
		FirmwareVersion: req.FirmwareVersion,
		SerialNumber:    req.ChargePointSerialNumber,
	})
	return core.NewBootNotificationConfirmation(types.NewDateTime(time.Now()), 60, core.RegistrationStatusAccepted), nil
}

func (h *Handler) OnAuthorize(chargePointId string, req *core.AuthorizeRequest) (*core.AuthorizeConfirmation, error) {
	start := time.Now()
	h.router.RouteAuthorize(chargePointId, req)
	h.publishOcppMessage(chargePointId, "inbound", "Authorize", 0)
	h.logger.Debug("authorize", "charger", chargePointId, "id_tag", req.IdTag)
	h.recordInbound("Authorize", start, nil)
	return core.NewAuthorizationConfirmation(types.NewIdTagInfo(types.AuthorizationStatusAccepted)), nil
}

func (h *Handler) OnDataTransfer(chargePointId string, req *core.DataTransferRequest) (*core.DataTransferConfirmation, error) {
	start := time.Now()
	h.router.RouteDataTransfer(chargePointId, req)
	h.publishOcppMessage(chargePointId, "inbound", "DataTransfer", 0)
	h.logger.Debug("data transfer", "charger", chargePointId, "vendor", req.VendorId)
	h.recordInbound("DataTransfer", start, nil)
	return core.NewDataTransferConfirmation(core.DataTransferStatusAccepted), nil
}

func (h *Handler) OnHeartbeat(chargePointId string, req *core.HeartbeatRequest) (*core.HeartbeatConfirmation, error) {
	start := time.Now()
	h.router.RouteHeartbeat(chargePointId, req)
	h.publishOcppMessage(chargePointId, "inbound", "Heartbeat", 0)
	h.recordInbound("Heartbeat", start, nil)
	return core.NewHeartbeatConfirmation(types.NewDateTime(time.Now())), nil
}

func (h *Handler) OnStatusNotification(chargePointId string, req *core.StatusNotificationRequest) (*core.StatusNotificationConfirmation, error) {
	start := time.Now()
	h.router.RouteStatusNotification(chargePointId, req)
	h.publishOcppMessage(chargePointId, "inbound", "StatusNotification", req.ConnectorId)
	status := charger.ConnectorStatus(string(req.Status))

	h.logger.Info("status notification",
		"charger", chargePointId,
		"connector", req.ConnectorId,
		"status", status,
		"error", req.ErrorCode,
	)

	conn := charger.Connector{
		ChargerID:   chargePointId,
		ConnectorID: req.ConnectorId,
		Status:      status,
		ErrorCode:   string(req.ErrorCode),
		Info:        req.Info,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := h.chargerRepo.UpsertConnector(ctx, conn); err != nil {
		h.logger.Error("failed to upsert connector status", "err", err)
	}

	h.publisher.PublishChargerStatus(chargePointId, status)

	if h.broadcaster != nil {
		h.broadcaster.PublishStatus(chargePointId, string(status))
	}

	charging := status == charger.StatusCharging
	h.publisher.PublishChargingState(chargePointId, charging)

	h.recordInbound("StatusNotification", start, nil)
	h.emit(csms.StatusChanged{
		Timestamp:   time.Now(),
		ChargerID:   chargePointId,
		ConnectorID: req.ConnectorId,
		Status:      string(status),
		ErrorCode:   string(req.ErrorCode),
	})
	return core.NewStatusNotificationConfirmation(), nil
}

func (h *Handler) OnMeterValues(chargePointId string, req *core.MeterValuesRequest) (*core.MeterValuesConfirmation, error) {
	start := time.Now()
	h.router.RouteMeterValues(chargePointId, req)
	h.publishOcppMessage(chargePointId, "inbound", "MeterValues", req.ConnectorId)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var sessionID string
	if req.TransactionId != nil {
		if sid, err := h.findSessionUUID(ctx, chargePointId, *req.TransactionId); err == nil {
			sessionID = sid
		}
	}

	// Session recovery: if MeterValues carries a TransactionId but no session
	// matched (e.g. after a CSMS restart mid-transaction), reconstruct the
	// session from the charger's state. The charger emits MeterValues every ~10s
	// during charging, each carrying the active TransactionId.
	if sessionID == "" && req.TransactionId != nil {
		existing, err := h.sessionRepo.GetSessionByTransactionID(ctx, chargePointId, *req.TransactionId)
		if err == nil && existing != nil {
			sessionID = existing.ID
		} else if existing == nil {
			// No session found — recover it.
			recoveredID := uuid.NewString()
			recovered := session.Session{
				ID:            recoveredID,
				TransactionID: *req.TransactionId,
				ChargerID:     chargePointId,
				ConnectorID:   req.ConnectorId,
				IDTag:         "recovered",
				StartedAt:     time.Now(),
			}
			if err := h.sessionRepo.CreateSession(ctx, recovered); err != nil {
				h.logger.Error("failed to create recovered session", "err", err, "charger", chargePointId, "tx_id", *req.TransactionId)
			} else {
				sessionID = recoveredID
				h.logger.Info("recovered active session after CSMS restart",
					"charger", chargePointId,
					"connector", req.ConnectorId,
					"tx_id", *req.TransactionId,
					"session", recoveredID,
				)
				conn := charger.Connector{
					ChargerID:   chargePointId,
					ConnectorID: req.ConnectorId,
					Status:      charger.StatusCharging,
				}
				if err := h.chargerRepo.UpsertConnector(ctx, conn); err != nil {
					h.logger.Error("failed to upsert connector status on recovery", "err", err)
				}
				h.publisher.PublishChargerStatus(chargePointId, charger.StatusCharging)
				h.publisher.PublishChargingState(chargePointId, true)
				h.emit(csms.TransactionStarted{
					Timestamp:   time.Now(),
					TxID:        *req.TransactionId,
					ChargerID:   chargePointId,
					IDTag:       "recovered",
					ConnectorID: req.ConnectorId,
				})
			}
		}
	}

	var meterValues []ports.MeterValue
	var powerKW, energyWh, currentA float64

	for _, mv := range req.MeterValue {
		ts := time.Now()
		if mv.Timestamp != nil {
			ts = mv.Timestamp.Time
		}
		for _, sv := range mv.SampledValue {
			mvals, pwr, enrg, amps := h.parseSampledValue(sv, chargePointId, req.ConnectorId, sessionID, ts)
			meterValues = append(meterValues, mvals...)
			if pwr > 0 {
				powerKW = pwr
			}
			if enrg > 0 {
				energyWh = enrg
			}
			if amps > 0 {
				currentA = amps
			}
		}
	}

	if len(meterValues) > 0 {
		if err := h.meterRepo.StoreMeterValues(ctx, meterValues); err != nil {
			h.logger.Error("failed to store meter values", "err", err, "charger", chargePointId)
		}
	}

	if powerKW > 0 || energyWh > 0 {
		h.publisher.PublishChargerPower(chargePointId, powerKW)
	}
	if energyWh > 0 {
		h.publisher.PublishSessionEnergy(chargePointId, energyWh/1000.0)
	}

	if h.metrics != nil && powerKW > 0 {
		h.metrics.RecordChargePowerWatts(chargePointId, powerKW*1000)
	}

	if h.broadcaster != nil && (powerKW > 0 || energyWh > 0 || currentA > 0) {
		h.broadcaster.UpdateMeter(chargePointId, powerKW*1000, energyWh/1000.0, currentA)
	}

	h.recordInbound("MeterValues", start, nil)
	if energyWh > 0 {
		txID := 0
		if req.TransactionId != nil {
			txID = *req.TransactionId
		}
		h.emit(csms.MeterValue{
			Timestamp:   time.Now(),
			TxID:        txID,
			ChargerID:   chargePointId,
			ConnectorID: req.ConnectorId,
			EnergyWh:    energyWh,
		})
	}
	return core.NewMeterValuesConfirmation(), nil
}

func (h *Handler) parseSampledValue(sv types.SampledValue, chargerID string, connectorID int, sessionID string, ts time.Time) ([]ports.MeterValue, float64, float64, float64) {
	val, err := strconv.ParseFloat(sv.Value, 64)
	if err != nil {
		return nil, 0, 0, 0
	}

	measurand := string(sv.Measurand)
	if measurand == "" {
		measurand = "Energy.Active.Import.Register"
	}

	mv := ports.MeterValue{
		ChargerID:   chargerID,
		ConnectorID: connectorID,
		SessionID:   sessionID,
		Timestamp:   ts,
		Measurand:   measurand,
		Value:       val,
		Unit:        string(sv.Unit),
		Phase:       string(sv.Phase),
	}

	var powerKW, energyWh, currentA float64
	if measurand == "Power.Active.Import" {
		if string(sv.Unit) == "W" || sv.Unit == "" {
			powerKW = val / 1000.0
		} else if string(sv.Unit) == "kW" {
			powerKW = val
		}
	}
	if measurand == "Energy.Active.Import.Register" {
		if string(sv.Unit) == "Wh" || sv.Unit == "" {
			energyWh = val
		} else if string(sv.Unit) == "kWh" {
			energyWh = val * 1000.0
		}
	}
	if measurand == "Current.Import" {
		currentA = val
	}

	return []ports.MeterValue{mv}, powerKW, energyWh, currentA
}

func (h *Handler) OnStartTransaction(chargePointId string, req *core.StartTransactionRequest) (*core.StartTransactionConfirmation, error) {
	start := time.Now()
	h.router.RouteStartTransaction(chargePointId, req)
	h.publishOcppMessage(chargePointId, "inbound", "StartTransaction", req.ConnectorId)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	txID := generateTxID()
	sessionUUID := uuid.NewString()

	s := session.Session{
		ID:            sessionUUID,
		TransactionID: txID,
		ChargerID:     chargePointId,
		ConnectorID:   req.ConnectorId,
		IDTag:         req.IdTag,
		MeterStartWh:  float64(req.MeterStart),
		StartedAt:     time.Now(),
	}

	if req.Timestamp != nil {
		s.StartedAt = req.Timestamp.Time
	}

	if err := h.sessionRepo.CreateSession(ctx, s); err != nil {
		h.logger.Error("failed to create session", "err", err, "charger", chargePointId)
	} else {
		h.logger.Info("transaction started",
			"charger", chargePointId,
			"connector", req.ConnectorId,
			"tx_id", txID,
			"session", sessionUUID,
			"meter_start_wh", req.MeterStart,
		)
		if h.metrics != nil {
			h.metrics.RecordSessionStart()
		}
	}

	h.recordInbound("StartTransaction", start, nil)
	h.emit(csms.TransactionStarted{
		Timestamp:    time.Now(),
		TxID:         txID,
		ChargerID:    chargePointId,
		IDTag:        req.IdTag,
		ConnectorID:  req.ConnectorId,
		MeterStartWh: float64(req.MeterStart),
	})
	return core.NewStartTransactionConfirmation(
		types.NewIdTagInfo(types.AuthorizationStatusAccepted),
		txID,
	), nil
}

func (h *Handler) OnStopTransaction(chargePointId string, req *core.StopTransactionRequest) (*core.StopTransactionConfirmation, error) {
	start := time.Now()
	h.router.RouteStopTransaction(chargePointId, req)
	h.publishOcppMessage(chargePointId, "inbound", "StopTransaction", 0)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stopTime := time.Now()
	if req.Timestamp != nil {
		stopTime = req.Timestamp.Time
	}

	active, err := h.sessionRepo.GetSessionByTransactionID(ctx, chargePointId, req.TransactionId)
	if err != nil {
		h.logger.Error("failed to find session for stop", "err", err, "charger", chargePointId, "tx_id", req.TransactionId)
	} else if active != nil {
		meterStop := float64(req.MeterStop)
		active.MeterStopWh = &meterStop
		active.StoppedAt = &stopTime
		active.StopReason = string(req.Reason)

		if err := h.sessionRepo.UpdateSession(ctx, *active); err != nil {
			h.logger.Error("failed to update session on stop", "err", err)
		} else {
			h.logger.Info("transaction stopped",
				"charger", chargePointId,
				"tx_id", req.TransactionId,
				"session", active.ID,
				"meter_stop_wh", req.MeterStop,
				"reason", req.Reason,
			)
			if h.metrics != nil {
				h.metrics.RecordSessionStop()
			}
		}
	}

	h.recordInbound("StopTransaction", start, nil)
	if h.overrideClearer != nil {
		h.overrideClearer.ClearManualOverride(chargePointId)
	}
	h.emit(csms.TransactionStopped{
		Timestamp:   time.Now(),
		TxID:        req.TransactionId,
		ChargerID:   chargePointId,
		Reason:      string(req.Reason),
		MeterStopWh: float64(req.MeterStop),
	})
	return core.NewStopTransactionConfirmation(), nil
}

func (h *Handler) OnConnect(chargePointID string) {
	h.activeConns.Add(1)
	h.logger.Info("charger connected", "charger", chargePointID)
	if h.metrics != nil {
		h.metrics.RecordChargerConnected()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := h.chargerRepo.MarkOnline(ctx, chargePointID, true); err != nil {
		h.logger.Debug("creating charger placeholder on first connect", "charger", chargePointID)
		if err := h.chargerRepo.UpsertCharger(ctx, charger.Charger{
			ID:     chargePointID,
			Online: true,
		}); err != nil {
			h.logger.Error("failed to create charger placeholder", "err", err, "charger", chargePointID)
		}
	}
	h.publisher.PublishChargerOnline(chargePointID, true)

	if h.relayHook != nil {
		if err := h.relayHook.Connect(ctx, chargePointID); err != nil {
			h.logger.Error("relay connect failed", "charger", chargePointID, "err", err)
		}
	}

	if h.discovery != nil {
		if c, err := h.chargerRepo.GetCharger(ctx, chargePointID); err == nil && c != nil {
			proxyEnabled := h.isProxyEnabled(ctx, chargePointID)
			min, max := h.getAmpBounds()
			h.discovery.PublishDiscovery(*c, min, max, proxyEnabled)
		}
	}

	h.emit(csms.ChargerConnected{
		Timestamp: time.Now(),
		ChargerID: chargePointID,
	})
}

func (h *Handler) OnDisconnect(chargePointID string) {
	defer h.activeConns.Add(-1)
	h.logger.Warn("charger disconnected", "charger", chargePointID)
	if h.metrics != nil {
		h.metrics.RecordChargerDisconnected()
	}

	if h.overrideClearer != nil {
		h.overrideClearer.ClearManualOverride(chargePointID)
	}

	if h.relayHook != nil {
		h.relayHook.Disconnect(chargePointID)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := h.chargerRepo.MarkOnline(ctx, chargePointID, false); err != nil {
		h.logger.Error("failed to mark charger offline", "err", err, "charger", chargePointID)
	}
	h.publisher.PublishChargerOnline(chargePointID, false)

	h.emit(csms.ChargerDisconnected{
		Timestamp: time.Now(),
		ChargerID: chargePointID,
	})
}

func (h *Handler) WaitForIdle(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if h.activeConns.Load() == 0 {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return h.activeConns.Load() == 0
}

func (h *Handler) isProxyEnabled(ctx context.Context, chargerID string) bool {
	if h.proxyConfigRepo == nil {
		return false
	}
	cfg, err := h.proxyConfigRepo.GetProxyConfig(ctx, chargerID)
	if err != nil || cfg == nil {
		return false
	}
	return cfg.ProxyEnabled
}

func (h *Handler) findSessionUUID(ctx context.Context, chargerID string, txID int) (string, error) {
	active, err := h.sessionRepo.GetSessionByTransactionID(ctx, chargerID, txID)
	if err != nil || active == nil {
		return "", fmt.Errorf("no session for tx %d", txID)
	}
	return active.ID, nil
}

func (h *Handler) detectHardwareProfile(chargerID, vendor, model, firmware string) {
	if strings.EqualFold(vendor, "ABB") {
		h.logger.Info("detected ABB charger — applying Terra AC charging profile quirks",
			"charger", chargerID,
			"model", model,
			"firmware", firmware,
		)
		h.warnOldFirmware(chargerID, firmware)
	} else {
		h.logger.Warn("unverified charger vendor — using ABB Terra AC profile defaults",
			"charger", chargerID,
			"vendor", vendor,
			"model", model,
			"firmware", firmware,
		)
	}
}

func (h *Handler) warnOldFirmware(chargerID, firmware string) {
	if !isFirmwareBelowMin(firmware, 1, 8, 32) {
		return
	}
	h.logger.Warn("ABB firmware below minimum — OCPP 1.6-J may not work correctly",
		"charger", chargerID,
		"firmware", firmware,
		"minimum_required", "1.8.32",
	)
}

func isFirmwareBelowMin(version string, minMajor, minMinor, minPatch int) bool {
	major, minor, patch, ok := parseSemver(version)
	if !ok {
		return false
	}
	if major != minMajor {
		return major < minMajor
	}
	if minor != minMinor {
		return minor < minMinor
	}
	return patch < minPatch
}

func parseSemver(version string) (major, minor, patch int, ok bool) {
	parts := strings.SplitN(version, ".", 4)
	if len(parts) < 3 {
		return 0, 0, 0, false
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, 0, false
	}
	minor, err = strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, 0, false
	}
	patchStr := parts[2]
	for i, c := range patchStr {
		if c < '0' || c > '9' {
			patchStr = patchStr[:i]
			break
		}
	}
	patch, err = strconv.Atoi(patchStr)
	if err != nil {
		return 0, 0, 0, false
	}
	return major, minor, patch, true
}

func generateTxID() int {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	v, _ := strconv.ParseInt(hex.EncodeToString(b), 16, 64)
	if v < 0 {
		v = -v
	}
	if v == 0 {
		v = 1
	}
	if v > 2147483647 {
		v = v % 2147483647
	}
	return int(v)
}