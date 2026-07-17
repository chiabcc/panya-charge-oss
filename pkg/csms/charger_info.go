package csms

// ChargerInfo is a DTO returned by the Facade for read-only charger
// consumption.  It contains a point-in-time snapshot of a registered
// charger's state.
type ChargerInfo struct {
	ID            string  // OCPP ChargePoint ID
	Vendor        string
	Model         string
	Firmware      string
	SerialNumber  string
	Status        string // e.g. "Available", "Charging", "Faulted"
	ConnectorID   int
	TxID          int   // active transaction ID, 0 if idle
	LimitAmps     int   // current charging limit in amps
	ChargingPower float64 // instantaneous power in watts
}