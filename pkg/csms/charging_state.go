package csms

// ChargingState is a point-in-time snapshot of the smart charging runtime state.
type ChargingState struct {
	CurrentAmps    int32
	GridPowerW     float64
	SolarPowerW    float64
	ConsumptionW   float64
	Enabled        bool
}