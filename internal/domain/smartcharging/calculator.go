package smartcharging

import (
	"math"
	"sync"
)

const crossValidationToleranceW = 500.0

const (
	stopConfirmTicks = 3
	runConfirmTicks  = 2
)

type chargerStopState struct {
	isStopped  bool
	belowCount int
	aboveCount int
}

type Calculator struct {
	mu         sync.RWMutex
	minAmps    int
	maxAmps    int
	gridVolt   float64
	hysteresis int
	lastLimit  map[string]int
	stopState  map[string]*chargerStopState
}

func NewCalculator(minAmps, maxAmps int, gridVolt float64) *Calculator {
	return &Calculator{
		minAmps:    minAmps,
		maxAmps:    maxAmps,
		gridVolt:   gridVolt,
		hysteresis: 2,
		lastLimit:  make(map[string]int),
		stopState:  make(map[string]*chargerStopState),
	}
}

func (c *Calculator) SetLimits(minAmps, maxAmps int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.minAmps = minAmps
	c.maxAmps = maxAmps
}

func (c *Calculator) Compute(chargerID string, sample MeterSample) ChargingProfileRequest {
	surplusW := computeSurplus(sample)

	availableW := surplusW
	if availableW < 0 {
		availableW = 0
	}

	idealAmps := int(math.Floor(availableW / c.gridVolt))

	c.mu.Lock()
	defer c.mu.Unlock()

	state := c.stopState[chargerID]
	if state == nil {
		state = &chargerStopState{}
		c.stopState[chargerID] = state
	}

	wouldStop := idealAmps < c.minAmps

	if state.isStopped {
		if wouldStop {
			state.aboveCount = 0
		} else {
			state.aboveCount++
			if state.aboveCount >= runConfirmTicks {
				state.isStopped = false
				state.belowCount = 0
			}
		}
	} else {
		if wouldStop {
			state.belowCount++
			if state.belowCount >= stopConfirmTicks {
				state.isStopped = true
				state.aboveCount = 0
			}
		} else {
			state.belowCount = 0
		}
	}

	prevLimit, hasPrev := c.lastLimit[chargerID]

	if state.isStopped {
		c.lastLimit[chargerID] = c.minAmps
		return ChargingProfileRequest{
			LimitAmps:  c.minAmps,
			Reason:     "insufficient surplus — falling back to minimum",
			ShouldStop: true,
		}
	}

	if wouldStop {
		holdAmps := c.minAmps
		if hasPrev && prevLimit >= c.minAmps {
			holdAmps = prevLimit
		}
		c.lastLimit[chargerID] = holdAmps
		return ChargingProfileRequest{
			LimitAmps: holdAmps,
			Reason:    "below threshold — holding while stop debouncing",
		}
	}

	if idealAmps > c.maxAmps {
		idealAmps = c.maxAmps
	}

	if hasPrev && prevLimit > 0 && abs(idealAmps-prevLimit) < c.hysteresis {
		return ChargingProfileRequest{
			LimitAmps: prevLimit,
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
