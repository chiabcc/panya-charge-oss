package smartcharging

import (
	"math"
)

const crossValidationToleranceW = 500.0

// Calculator computes the optimal charging current limit based on
// real-time solar surplus and grid power data.
//
// All logic is pure Go — no I/O, no side effects.
// The OCPP adapter translates the output into SetChargingProfile calls.
type Calculator struct {
	minAmps    int
	maxAmps    int
	gridVolt   float64
	hysteresis int
	lastLimit  map[string]int
}

func NewCalculator(minAmps, maxAmps int, gridVolt float64) *Calculator {
	return &Calculator{
		minAmps:    minAmps,
		maxAmps:    maxAmps,
		gridVolt:   gridVolt,
		hysteresis: 2,
		lastLimit:  make(map[string]int),
	}
}

// Compute determines the ideal charging profile from the latest meter data.
//
// Surplus strategy (v1.5 multi-source):
//   - Primary:   surplus = solar - consumption  (when both available)
//   - Fallback:  surplus = -grid                (grid-only, v0.1.0 compat)
//   - Cross-val: warn if |solar - consumption + grid| > tolerance (sensor drift)
//
// surplus > 0 (exporting): ramp charging up
// surplus < 0 (importing): ramp charging down
// below minimum: stop or fall back to minimum
func (c *Calculator) Compute(chargerID string, sample MeterSample) ChargingProfileRequest {
	surplusW := computeSurplus(sample)

	availableW := surplusW
	if availableW < 0 {
		availableW = 0
	}

	idealAmps := int(math.Floor(availableW / c.gridVolt))

	if idealAmps < c.minAmps {
		c.lastLimit[chargerID] = c.minAmps
		return ChargingProfileRequest{
			LimitAmps:  c.minAmps,
			Reason:     "insufficient surplus — falling back to minimum",
			ShouldStop: true,
		}
	}

	if idealAmps > c.maxAmps {
		idealAmps = c.maxAmps
	}

	prev, hasPrev := c.lastLimit[chargerID]
	if hasPrev && prev > 0 && abs(idealAmps-prev) < c.hysteresis {
		return ChargingProfileRequest{
			LimitAmps: prev,
			Reason:    "within hysteresis band — holding previous limit",
		}
	}

	c.lastLimit[chargerID] = idealAmps
	return ChargingProfileRequest{
		LimitAmps: idealAmps,
		Reason:    surplusMethod(sample),
	}
}

// computeSurplus determines the available solar surplus in watts.
//
// When solar and consumption readings are present (> 0), they provide a
// more accurate surplus than grid alone: surplus = solar - consumption.
// Otherwise, falls back to the negated grid power (v0.1.0 behavior).
func computeSurplus(sample MeterSample) float64 {
	if sample.SolarPowerW > 0 || sample.ConsumptionPowerW > 0 {
		return sample.SolarPowerW - sample.ConsumptionPowerW
	}
	return sample.GridPowerW * -1
}

// surplusMethod returns a human-readable label for the surplus source used.
func surplusMethod(sample MeterSample) string {
	if sample.SolarPowerW > 0 || sample.ConsumptionPowerW > 0 {
		return "solar surplus adjusted (solar-consumption)"
	}
	return "solar surplus adjusted (grid fallback)"
}

// CrossValidationDrift returns the absolute difference between the
// solar-consumption surplus and the negated grid reading. A large value
// (> crossValidationToleranceW) indicates sensor drift or mismatched
// readings. Returns 0 if either source is unavailable.
func CrossValidationDrift(sample MeterSample) float64 {
	if sample.SolarPowerW <= 0 && sample.ConsumptionPowerW <= 0 {
		return 0
	}
	if sample.GridPowerW == 0 {
		return 0
	}
	solarSurplus := sample.SolarPowerW - sample.ConsumptionPowerW
	gridSurplus := sample.GridPowerW * -1
	return math.Abs(solarSurplus - gridSurplus)
}

// HasSensorDrift reports whether the solar/consumption readings disagree
// with the grid reading beyond the tolerance threshold.
func HasSensorDrift(sample MeterSample) bool {
	return CrossValidationDrift(sample) > crossValidationToleranceW
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
