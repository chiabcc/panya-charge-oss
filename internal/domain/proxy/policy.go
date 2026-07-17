// Package proxy defines the OCPP message routing policy for the proxy/forwarding
// feature (issue #9). The policy decides for each OCPP 1.6 action whether it
// should be handled locally, forwarded upstream, both, or dropped.
package proxy

// RouteDecision expresses what the router should do with a message.
type RouteDecision int

const (
	// DecisionLocalOnly handles the message in the local CSMS only.
	// The upstream vendor cloud must never see it.
	// Example: SetChargingProfile (solar surplus logic).
	DecisionLocalOnly RouteDecision = iota

	// DecisionUpstreamOnly forwards the message to the upstream vendor cloud
	// without any local processing beyond logging.
	// Example: Authorize (vendor does idTag validation).
	DecisionUpstreamOnly

	// DecisionBoth processes the message locally AND forwards upstream.
	// Example: BootNotification, StartTransaction.
	DecisionBoth

	// DecisionDrop silently discards the message.
	// Reserved for throttled or rate-limited messages.
	DecisionDrop
)

// Direction indicates whether a message flows from charger to CSMS (inbound)
// or from CSMS to charger (outbound).
type Direction int

const (
	// DirectionInbound: Charge Point → Central System (e.g. BootNotification).
	DirectionInbound Direction = iota

	// DirectionOutbound: Central System → Charge Point (e.g. RemoteStartTransaction).
	DirectionOutbound
)

// ShouldForward reports whether the decision requires upstream forwarding.
func (d RouteDecision) ShouldForward() bool {
	return d == DecisionUpstreamOnly || d == DecisionBoth
}

// ShouldHandleLocally reports whether the decision requires local processing.
func (d RouteDecision) ShouldHandleLocally() bool {
	return d == DecisionLocalOnly || d == DecisionBoth
}

// String returns a human-readable name for logging.
func (d RouteDecision) String() string {
	switch d {
	case DecisionLocalOnly:
		return "local_only"
	case DecisionUpstreamOnly:
		return "upstream_only"
	case DecisionBoth:
		return "both"
	case DecisionDrop:
		return "drop"
	default:
		return "unknown"
	}
}

// OCPP 1.6 action identifiers. These mirror the action strings used in the
// OCPP-J protocol and the ocpp-go library.
const (
	// Inbound actions (CP → CSMS)
	ActionBootNotification              = "BootNotification"
	ActionAuthorize                     = "Authorize"
	ActionDataTransfer                  = "DataTransfer"
	ActionHeartbeat                     = "Heartbeat"
	ActionMeterValues                   = "MeterValues"
	ActionStatusNotification            = "StatusNotification"
	ActionStartTransaction              = "StartTransaction"
	ActionStopTransaction               = "StopTransaction"
	ActionFirmwareStatusNotification    = "FirmwareStatusNotification"
	ActionDiagnosticsStatusNotification = "DiagnosticsStatusNotification"

	// Outbound actions (CSMS → CP)
	ActionRemoteStartTransaction = "RemoteStartTransaction"
	ActionRemoteStopTransaction  = "RemoteStopTransaction"
	ActionSetChargingProfile     = "SetChargingProfile"
	ActionClearChargingProfile   = "ClearChargingProfile"
	ActionGetConfiguration       = "GetConfiguration"
	ActionChangeConfiguration    = "ChangeConfiguration"
	ActionTriggerMessage         = "TriggerMessage"
	ActionReserveNow             = "ReserveNow"
	ActionCancelReservation      = "CancelReservation"
	ActionReset                  = "Reset"
	ActionUnlockConnector        = "UnlockConnector"
	ActionGetLog                 = "GetLog"
	ActionGetDiagnostics         = "GetDiagnostics"
	ActionUpdateFirmware         = "UpdateFirmware"
	ActionClearCache             = "ClearCache"
)

// Policy holds the routing decisions for inbound and outbound OCPP messages.
// The zero value is an empty policy — use DefaultPolicy() for production defaults.
type Policy struct {
	inbound  map[string]RouteDecision
	outbound map[string]RouteDecision
}

// DefaultPolicy returns the production routing rules as specified in issue #9.
//
// Key rules:
//   - SetChargingProfile and ClearChargingProfile are ALWAYS LocalOnly — the
//     upstream vendor cloud must never see our solar surplus adjustments.
//   - Authorize is UpstreamOnly — the vendor cloud owns idTag validation.
//   - BootNotification, Heartbeat, Start/StopTransaction, StatusNotification
//     are Both — local needs them AND the vendor cloud expects them.
//   - Firmware/Diagnostics status notifications are UpstreamOnly — vendor
//     owns firmware and support flows.
func DefaultPolicy() Policy {
	return Policy{
		inbound: map[string]RouteDecision{
			ActionBootNotification:              DecisionBoth,
			ActionHeartbeat:                     DecisionBoth,
			ActionAuthorize:                     DecisionUpstreamOnly,
			ActionStartTransaction:              DecisionBoth,
			ActionStopTransaction:               DecisionBoth,
			ActionStatusNotification:            DecisionBoth,
			ActionMeterValues:                   DecisionBoth,
			ActionDataTransfer:                  DecisionUpstreamOnly,
			ActionFirmwareStatusNotification:    DecisionUpstreamOnly,
			ActionDiagnosticsStatusNotification: DecisionUpstreamOnly,
		},
		outbound: map[string]RouteDecision{
			ActionSetChargingProfile:     DecisionLocalOnly,
			ActionClearChargingProfile:   DecisionLocalOnly,
			ActionRemoteStartTransaction: DecisionBoth,
			ActionRemoteStopTransaction:  DecisionBoth,
			ActionTriggerMessage:         DecisionBoth,
			ActionGetConfiguration:       DecisionBoth,
			ActionChangeConfiguration:    DecisionBoth,
			ActionReset:                  DecisionBoth,
			ActionUnlockConnector:        DecisionBoth,
			ActionGetLog:                 DecisionBoth,
			ActionGetDiagnostics:         DecisionBoth,
			ActionReserveNow:             DecisionUpstreamOnly,
			ActionCancelReservation:      DecisionUpstreamOnly,
			ActionUpdateFirmware:         DecisionUpstreamOnly,
			ActionClearCache:             DecisionUpstreamOnly,
		},
	}
}

// Decide returns the routing decision for a given direction and OCPP action.
// Unknown actions default to DecisionLocalOnly (safe default — never accidentally
// leak an unknown message to the upstream vendor cloud).
func (p Policy) Decide(dir Direction, action string) RouteDecision {
	switch dir {
	case DirectionInbound:
		if d, ok := p.inbound[action]; ok {
			return d
		}
	case DirectionOutbound:
		if d, ok := p.outbound[action]; ok {
			return d
		}
	}
	return DecisionLocalOnly
}

// Merge applies per-charger overrides on top of this policy. The override map
// uses the same action strings as keys and RouteDecision values. Only actions
// present in the override map are changed; all others keep their default decision.
//
// A nil or empty override returns the policy unchanged.
func (p Policy) Merge(overrides map[string]OverrideDecision) Policy {
	if len(overrides) == 0 {
		return p
	}

	merged := Policy{
		inbound:  make(map[string]RouteDecision, len(p.inbound)),
		outbound: make(map[string]RouteDecision, len(p.outbound)),
	}
	for k, v := range p.inbound {
		merged.inbound[k] = v
	}
	for k, v := range p.outbound {
		merged.outbound[k] = v
	}

	for action, od := range overrides {
		d := RouteDecision(od)
		// Apply to whichever map(s) contain this action. If neither contains
		// it, treat it as an inbound action (most user overrides are inbound).
		if _, isInbound := p.inbound[action]; isInbound {
			merged.inbound[action] = d
		} else if _, isOutbound := p.outbound[action]; isOutbound {
			merged.outbound[action] = d
		} else {
			merged.inbound[action] = d
		}
	}

	return merged
}

// OverrideDecision is a JSON-serializable RouteDecision for storage in the
// chargers table's proxy_policy_json column.
type OverrideDecision int

// MarshalJSON encodes the decision as a lowercase string for JSON storage.
func (o OverrideDecision) MarshalJSON() ([]byte, error) {
	return []byte(`"` + RouteDecision(o).String() + `"`), nil
}

// UnmarshalJSON decodes a decision from a lowercase string.
func (o *OverrideDecision) UnmarshalJSON(data []byte) error {
	s := string(data)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
	}
	switch s {
	case "local_only":
		*o = OverrideDecision(DecisionLocalOnly)
	case "upstream_only":
		*o = OverrideDecision(DecisionUpstreamOnly)
	case "both":
		*o = OverrideDecision(DecisionBoth)
	case "drop":
		*o = OverrideDecision(DecisionDrop)
	default:
		*o = OverrideDecision(DecisionLocalOnly)
	}
	return nil
}
