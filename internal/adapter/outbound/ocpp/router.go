package ocpp

import (
	"context"
	"log/slog"

	"github.com/xBlaz3kx/ocpp-go/ocpp1.6/core"
	"github.com/xBlaz3kx/ocpp-go/ocpp1.6/firmware"
	"github.com/xBlaz3kx/ocpp-go/ocpp1.6/smartcharging"

	"github.com/chiabcc/panya-charge-oss/internal/domain/proxy"
)

type Relay interface {
	Forward(ctx context.Context, chargerID string, action string, payload any) error
	IsConnected(chargerID string) bool
}

type NoopRelay struct {
	logger *slog.Logger
}

func NewNoopRelay(logger *slog.Logger) *NoopRelay {
	return &NoopRelay{logger: logger}
}

func (n *NoopRelay) Forward(_ context.Context, chargerID string, action string, _ any) error {
	n.logger.Debug("relay: forward (stub)", "charger", chargerID, "action", action)
	return nil
}

func (n *NoopRelay) IsConnected(_ string) bool {
	return false
}

type Router struct {
	policy proxy.Policy
	relay  Relay
	logger *slog.Logger
}

func NewRouter(policy proxy.Policy, relay Relay, logger *slog.Logger) *Router {
	return &Router{
		policy: policy,
		relay:  relay,
		logger: logger,
	}
}

func (r *Router) decide(chargePointID string, dir proxy.Direction, action string) proxy.RouteDecision {
	d := r.policy.Decide(dir, action)
	r.logger.LogAttrs(context.Background(), slog.LevelDebug, "router: route",
		slog.String("charger", chargePointID),
		slog.String("action", action),
		slog.String("direction", dirString(dir)),
		slog.String("decision", d.String()),
	)
	return d
}

func dirString(dir proxy.Direction) string {
	if dir == proxy.DirectionOutbound {
		return "outbound"
	}
	return "inbound"
}

func (r *Router) forwardAsync(chargePointID string, action string, req any) {
	if err := r.relay.Forward(context.Background(), chargePointID, action, req); err != nil {
		r.logger.Error("router: upstream forward failed",
			"charger", chargePointID,
			"action", action,
			"err", err,
		)
	}
}

func (r *Router) RouteBootNotification(chargePointID string, req *core.BootNotificationRequest) proxy.RouteDecision {
	d := r.decide(chargePointID, proxy.DirectionInbound, proxy.ActionBootNotification)
	if d.ShouldForward() {
		r.forwardAsync(chargePointID, proxy.ActionBootNotification, req)
	}
	return d
}

func (r *Router) RouteAuthorize(chargePointID string, req *core.AuthorizeRequest) proxy.RouteDecision {
	d := r.decide(chargePointID, proxy.DirectionInbound, proxy.ActionAuthorize)
	if d.ShouldForward() {
		r.forwardAsync(chargePointID, proxy.ActionAuthorize, req)
	}
	return d
}

func (r *Router) RouteDataTransfer(chargePointID string, req *core.DataTransferRequest) proxy.RouteDecision {
	d := r.decide(chargePointID, proxy.DirectionInbound, proxy.ActionDataTransfer)
	if d.ShouldForward() {
		r.forwardAsync(chargePointID, proxy.ActionDataTransfer, req)
	}
	return d
}

func (r *Router) RouteHeartbeat(chargePointID string, req *core.HeartbeatRequest) proxy.RouteDecision {
	d := r.decide(chargePointID, proxy.DirectionInbound, proxy.ActionHeartbeat)
	if d.ShouldForward() {
		r.forwardAsync(chargePointID, proxy.ActionHeartbeat, req)
	}
	return d
}

func (r *Router) RouteStatusNotification(chargePointID string, req *core.StatusNotificationRequest) proxy.RouteDecision {
	d := r.decide(chargePointID, proxy.DirectionInbound, proxy.ActionStatusNotification)
	if d.ShouldForward() {
		r.forwardAsync(chargePointID, proxy.ActionStatusNotification, req)
	}
	return d
}

func (r *Router) RouteMeterValues(chargePointID string, req *core.MeterValuesRequest) proxy.RouteDecision {
	d := r.decide(chargePointID, proxy.DirectionInbound, proxy.ActionMeterValues)
	if d.ShouldForward() {
		r.forwardAsync(chargePointID, proxy.ActionMeterValues, req)
	}
	return d
}

func (r *Router) RouteStartTransaction(chargePointID string, req *core.StartTransactionRequest) proxy.RouteDecision {
	d := r.decide(chargePointID, proxy.DirectionInbound, proxy.ActionStartTransaction)
	if d.ShouldForward() {
		r.forwardAsync(chargePointID, proxy.ActionStartTransaction, req)
	}
	return d
}

func (r *Router) RouteStopTransaction(chargePointID string, req *core.StopTransactionRequest) proxy.RouteDecision {
	d := r.decide(chargePointID, proxy.DirectionInbound, proxy.ActionStopTransaction)
	if d.ShouldForward() {
		r.forwardAsync(chargePointID, proxy.ActionStopTransaction, req)
	}
	return d
}

func (r *Router) RouteFirmwareStatusNotification(chargePointID string, req *firmware.FirmwareStatusNotificationRequest) proxy.RouteDecision {
	d := r.decide(chargePointID, proxy.DirectionInbound, proxy.ActionFirmwareStatusNotification)
	if d.ShouldForward() {
		r.forwardAsync(chargePointID, proxy.ActionFirmwareStatusNotification, req)
	}
	return d
}

func (r *Router) RouteDiagnosticsStatusNotification(chargePointID string, req *firmware.DiagnosticsStatusNotificationRequest) proxy.RouteDecision {
	d := r.decide(chargePointID, proxy.DirectionInbound, proxy.ActionDiagnosticsStatusNotification)
	if d.ShouldForward() {
		r.forwardAsync(chargePointID, proxy.ActionDiagnosticsStatusNotification, req)
	}
	return d
}

func (r *Router) RouteSetChargingProfile(chargePointID string, req *smartcharging.SetChargingProfileRequest) proxy.RouteDecision {
	d := r.policy.Decide(proxy.DirectionOutbound, proxy.ActionSetChargingProfile)
	r.logger.Debug("router: outbound route",
		"charger", chargePointID,
		"action", proxy.ActionSetChargingProfile,
		"decision", d.String(),
	)
	return d
}

func (r *Router) RouteClearChargingProfile(chargePointID string, req *smartcharging.ClearChargingProfileRequest) proxy.RouteDecision {
	d := r.policy.Decide(proxy.DirectionOutbound, proxy.ActionClearChargingProfile)
	r.logger.Debug("router: outbound route",
		"charger", chargePointID,
		"action", proxy.ActionClearChargingProfile,
		"decision", d.String(),
	)
	return d
}

func (r *Router) RouteRemoteStartTransaction(chargePointID string, req *core.RemoteStartTransactionRequest) proxy.RouteDecision {
	d := r.decide(chargePointID, proxy.DirectionOutbound, proxy.ActionRemoteStartTransaction)
	return d
}

func (r *Router) RouteRemoteStopTransaction(chargePointID string, req *core.RemoteStopTransactionRequest) proxy.RouteDecision {
	d := r.decide(chargePointID, proxy.DirectionOutbound, proxy.ActionRemoteStopTransaction)
	return d
}
