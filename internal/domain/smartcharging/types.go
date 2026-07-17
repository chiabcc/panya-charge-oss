// Package smartcharging contains domain types and pure-Go logic for
// computing dynamic OCPP 1.6 ChargingProfile limits based on solar
// surplus and grid import/export data.
package smartcharging

// MeterSample represents a real-time power reading used by the charging
// profile calculator.
type MeterSample struct {
	// GridPowerW is the current grid power. Negative = exporting (surplus),
	// positive = importing. Optional when solar + consumption are available.
	GridPowerW float64

	// SolarPowerW is the current solar production (always >= 0).
	SolarPowerW float64

	// ConsumptionPowerW is the current total home consumption (always >= 0),
	// excluding EV charging power.
	ConsumptionPowerW float64
}

// ChargingProfileRequest is the output of the smart charging calculator.
// It represents the desired charging profile to send via SetChargingProfile.
type ChargingProfileRequest struct {
	// LimitAmps is the computed current limit (A) to enforce.
	LimitAmps int

	// Reason explains why this limit was chosen (for logging/debugging).
	Reason string

	// ShouldStop indicates the charger should pause charging entirely
	// (no solar surplus available, grid import would be required).
	ShouldStop bool
}
