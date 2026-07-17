// Package charger contains domain types representing an EV charger,
// its connectors, and connector state.
//
// This package is pure Go — no imports of database, HTTP, or MQTT.
// Adapters in internal/adapter implement the ports defined in internal/domain/ports.
package charger

// Charger represents a registered EV charge point.
type Charger struct {
	// ID is the OCPP chargePointIdentity (e.g. "ABB-001").
	ID string

	// Vendor is the chargePointVendor from BootNotification (e.g. "ABB").
	Vendor string

	// Model is the chargePointModel from BootNotification (e.g. "Terra AC W22-G5-R").
	Model string

	// FirmwareVersion from BootNotification.
	FirmwareVersion string

	// SerialNumber from BootNotification.
	SerialNumber string

	// Online indicates whether the charger currently has an active OCPP WebSocket connection.
	Online bool
}

// ChargerInfo holds a point-in-time snapshot of a registered charger's state.
// This type is returned by the CSMS Facade for read-only consumption.
type ChargerInfo struct {
	ID            string // OCPP Connector ID / ChargePoint ID
	Vendor        string
	Model         string
	Firmware      string
	SerialNumber  string
	Status        string // e.g. "Available", "Charging", "Faulted"
	ConnectorID   int
	TxID          int    // active transaction ID, 0 if idle
	LimitAmps     int    // current charging limit in amps
	ChargingPower float64 // instantaneous power in watts
}

// ConnectorStatus is the OCPP 1.6 charge point status.
type ConnectorStatus string

const (
	StatusAvailable     ConnectorStatus = "Available"
	StatusPreparing     ConnectorStatus = "Preparing"
	StatusCharging      ConnectorStatus = "Charging"
	StatusSuspendedEV   ConnectorStatus = "SuspendedEV"
	StatusSuspendedEVSE ConnectorStatus = "SuspendedEVSE"
	StatusFinishing     ConnectorStatus = "Finishing"
	StatusReserved      ConnectorStatus = "Reserved"
	StatusUnavailable   ConnectorStatus = "Unavailable"
	StatusFaulted       ConnectorStatus = "Faulted"
)

// Connector represents the state of a single physical connector on a charger.
type Connector struct {
	ChargerID   string
	ConnectorID int
	Status      ConnectorStatus
	ErrorCode   string // OCPP error code, empty if no error
	Info        string // Additional info
}
