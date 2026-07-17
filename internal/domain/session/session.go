// Package session contains domain types for charging sessions.
package session

import "time"

// Session represents a single charging session from start to stop.
type Session struct {
	// ID is a unique session identifier (UUID).
	ID string

	// TransactionID is the OCPP transaction ID assigned by the CSMS.
	TransactionID int

	// ChargerID is the OCPP chargePointIdentity.
	ChargerID string

	// ConnectorID is the physical connector number (typically 1).
	ConnectorID int

	// IDTag is the RFID/idTag used to authorize the session.
	IDTag string

	// MeterStartWh is the energy meter reading (Wh) at session start.
	MeterStartWh float64

	// MeterStopWh is the energy meter reading (Wh) at session stop.
	MeterStopWh *float64

	// StartedAt is the session start timestamp.
	StartedAt time.Time

	// StoppedAt is the session stop timestamp, or nil if ongoing.
	StoppedAt *time.Time

	// StopReason is the OCPP stop reason (e.g. "Local", "Remote", "EVDisconnected").
	StopReason string
}

// IsActive returns true if the session is ongoing.
func (s Session) IsActive() bool {
	return s.StoppedAt == nil
}

// EnergyKWh returns the energy delivered in kWh for a completed session.
func (s Session) EnergyKWh() *float64 {
	if s.MeterStopWh == nil {
		return nil
	}
	kwh := (*s.MeterStopWh - s.MeterStartWh) / 1000.0
	return &kwh
}
