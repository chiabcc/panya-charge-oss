// Package csms defines domain events emitted by the OCPP CSMS core.
// Each event type carries a timestamp and the fields relevant to that
// charging-station lifecycle event.  These types are pure Go — no I/O
// dependencies.
package csms

import "time"

// Event is the common interface for all CSMS domain events.
// Each concrete event implements the event() marker method.
type Event interface {
	event()
}

// ChargerConnected is emitted when a charge point establishes its OCPP
// WebSocket session and becomes available for management.
type ChargerConnected struct {
	Timestamp      time.Time
	ChargerID      string
	Vendor         string
	Model          string
	FirmwareVersion string
	SerialNumber   string
}

// ChargerDisconnected is emitted when a charge point's OCPP session
// terminates (graceful close, network loss, or timeout).
type ChargerDisconnected struct {
	Timestamp time.Time
	ChargerID string
}

// TransactionStarted is emitted when a charging session begins on a
// connector (Authorize + StartTransaction flow).
type TransactionStarted struct {
	Timestamp   time.Time
	TxID        int
	ChargerID   string
	IDTag       string
	ConnectorID int
	MeterStartWh float64
}

// TransactionStopped is emitted when a charging session ends (StopTransaction
// or remote stop). The Reason field carries the OCPP StoppedReason value.
type TransactionStopped struct {
	Timestamp   time.Time
	TxID        int
	ChargerID   string
	Reason      string
	MeterStopWh float64
}

// MeterValue is emitted for periodic energy readings during an active
// transaction. EnergyWh is cumulative (not instantaneous power).
type MeterValue struct {
	Timestamp   time.Time
	TxID        int
	ChargerID   string
	ConnectorID int
	EnergyWh    float64 // cumulative, NOT instantaneous power
}

// StatusChanged is emitted when a connector's operational status changes
// (Available, Occupied, Faulted, etc.) or an error code is reported.
type StatusChanged struct {
	Timestamp   time.Time
	ChargerID   string
	ConnectorID int
	Status      string // charger.ConnectorStatus as string
	ErrorCode   string
}

// ChargingProfileUpdated is emitted when the active charging profile for
// a charger changes. ShouldStop distinguishes surplus/solar mode (false)
// from fallback/grid mode (true).
type ChargingProfileUpdated struct {
	Timestamp    time.Time
	ChargerID    string
	LimitAmps    int
	ShouldStop   bool   // false = surplus/solar mode, true = fallback/grid mode
	Reason       string
}

// event() marker implementations

func (ChargerConnected) event()        {}
func (ChargerDisconnected) event()     {}
func (TransactionStarted) event()      {}
func (TransactionStopped) event()      {}
func (MeterValue) event()              {}
func (StatusChanged) event()           {}
func (ChargingProfileUpdated) event()  {}