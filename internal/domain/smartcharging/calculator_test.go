package smartcharging

import (
	"testing"
)

func TestCalculator_Compute(t *testing.T) {
	// minAmps=6, maxAmps=32, gridVolt=230 → 1A per 230W
	c := NewCalculator(6, 32, 230.0)

	tests := []struct {
		name      string
		chargerID string
		sample    MeterSample
		wantAmps  int
		wantStop  bool
	}{
		{
			name:      "large solar surplus — full amps",
			chargerID: "ABB-001",
			sample: MeterSample{
				GridPowerW: -8000, // 8kW export
			},
			wantAmps: 32, // 8000/230=34.7 → capped at 32
			wantStop: false,
		},
		{
			name:      "moderate solar surplus",
			chargerID: "ABB-001",
			sample: MeterSample{
				GridPowerW: -3450, // 3.45kW export
			},
			wantAmps: 15, // 3450/230=15
			wantStop: false,
		},
		{
			name:      "small surplus below minimum — holds previous while debouncing",
			chargerID: "ABB-001",
			sample: MeterSample{
				GridPowerW: -500, // 500W export → 500/230≈2.17A < 6A minimum
			},
			wantAmps: 15, // holds previous limit during debounce
			wantStop: false,
		},
		{
			name:      "grid importing — holds previous while debouncing",
			chargerID: "ABB-001",
			sample: MeterSample{
				GridPowerW: 2000, // importing 2kW
			},
			wantAmps: 15, // still debouncing
			wantStop: false,
		},
		{
			name:      "grid neutral — third below-threshold tick triggers stop",
			chargerID: "ABB-001",
			sample: MeterSample{
				GridPowerW: 0,
			},
			wantAmps: 6,
			wantStop: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := c.Compute(tt.chargerID, tt.sample)
			if result.LimitAmps != tt.wantAmps {
				t.Errorf("LimitAmps = %d, want %d (reason: %s)", result.LimitAmps, tt.wantAmps, result.Reason)
			}
			if result.ShouldStop != tt.wantStop {
				t.Errorf("ShouldStop = %v, want %v", result.ShouldStop, tt.wantStop)
			}
		})
	}
}

// TestCalculator_StopStartDebounce exercises the full state machine:
// 3 consecutive below-threshold ticks to stop, 2 above-threshold to resume.
func TestCalculator_StopStartDebounce(t *testing.T) {
	c := NewCalculator(6, 32, 230.0)
	const id = "ABB-001"

	// Establish a running state at 13A (3000W surplus).
	r := c.Compute(id, MeterSample{GridPowerW: -3000})
	if r.LimitAmps != 13 || r.ShouldStop {
		t.Fatalf("setup: got amps=%d stop=%v, want 13/false (%s)", r.LimitAmps, r.ShouldStop, r.Reason)
	}

	// Tick 1 below threshold: hold at 13, not stopped.
	r = c.Compute(id, MeterSample{GridPowerW: -500})
	if r.LimitAmps != 13 || r.ShouldStop {
		t.Errorf("below tick 1: got amps=%d stop=%v, want 13/false (%s)", r.LimitAmps, r.ShouldStop, r.Reason)
	}

	// Tick 2 below threshold: still debouncing.
	r = c.Compute(id, MeterSample{GridPowerW: -500})
	if r.LimitAmps != 13 || r.ShouldStop {
		t.Errorf("below tick 2: got amps=%d stop=%v, want 13/false (%s)", r.LimitAmps, r.ShouldStop, r.Reason)
	}

	// Tick 3 below threshold: transition to stopped.
	r = c.Compute(id, MeterSample{GridPowerW: -500})
	if r.LimitAmps != 6 || !r.ShouldStop {
		t.Errorf("below tick 3: got amps=%d stop=%v, want 6/true (%s)", r.LimitAmps, r.ShouldStop, r.Reason)
	}

	// Tick 1 above threshold: still stopped (need 2 to resume).
	r = c.Compute(id, MeterSample{GridPowerW: -3000})
	if r.LimitAmps != 6 || !r.ShouldStop {
		t.Errorf("above tick 1: got amps=%d stop=%v, want 6/true (still debouncing) (%s)", r.LimitAmps, r.ShouldStop, r.Reason)
	}

	// Tick 2 above threshold: resume.
	r = c.Compute(id, MeterSample{GridPowerW: -3000})
	if r.ShouldStop {
		t.Errorf("above tick 2: got stop=%v, want false (resumed) (%s)", r.ShouldStop, r.Reason)
	}
	if r.LimitAmps != 13 {
		t.Errorf("above tick 2: got amps=%d, want 13 (%s)", r.LimitAmps, r.Reason)
	}
}

// TestCalculator_StopDebounceResetsOnAboveTick ensures a single above-threshold
// reading during the stop-debounce window resets the counter.
func TestCalculator_StopDebounceResetsOnAboveTick(t *testing.T) {
	c := NewCalculator(6, 32, 230.0)
	const id = "ABB-001"

	// Running at 13A.
	_ = c.Compute(id, MeterSample{GridPowerW: -3000})

	// Two below-threshold ticks (1 more would trigger stop).
	_ = c.Compute(id, MeterSample{GridPowerW: -500})
	_ = c.Compute(id, MeterSample{GridPowerW: -500})

	// Single above-threshold tick resets the counter.
	_ = c.Compute(id, MeterSample{GridPowerW: -3000})

	// One below-threshold tick should NOT trigger stop (counter restarted).
	r := c.Compute(id, MeterSample{GridPowerW: -500})
	if r.ShouldStop {
		t.Errorf("after reset: single below tick triggered stop, want continued running (%s)", r.Reason)
	}
}

func TestCalculator_Hysteresis(t *testing.T) {
	c := NewCalculator(6, 32, 230.0)

	// First call: 3000W surplus → 13A
	r1 := c.Compute("ABB-001", MeterSample{GridPowerW: -3000})
	if r1.LimitAmps != 13 {
		t.Fatalf("first compute: LimitAmps = %d, want 13", r1.LimitAmps)
	}

	// Small change (3000→3100W, 13→13.47A) → within hysteresis band of 2A → should hold at 13A
	r2 := c.Compute("ABB-001", MeterSample{GridPowerW: -3100})
	if r2.LimitAmps != 13 {
		t.Errorf("hysteresis hold: LimitAmps = %d, want 13 (reason: %s)", r2.LimitAmps, r2.Reason)
	}

	// Large change (3000→4000W, 13→17A) → exceeds hysteresis → should update to 17A
	r3 := c.Compute("ABB-001", MeterSample{GridPowerW: -4000})
	if r3.LimitAmps != 17 {
		t.Errorf("hysteresis break: LimitAmps = %d, want 17 (reason: %s)", r3.LimitAmps, r3.Reason)
	}
}

func TestCalculator_PerChargerHysteresis(t *testing.T) {
	c := NewCalculator(6, 32, 230.0)

	// Charger A: 3000W → 13A
	rA1 := c.Compute("ABB-A", MeterSample{GridPowerW: -3000})
	if rA1.LimitAmps != 13 {
		t.Fatalf("charger A first: LimitAmps = %d, want 13", rA1.LimitAmps)
	}

	// Charger B (different charger): 5000W → 21A — should NOT be affected by charger A's history
	rB1 := c.Compute("ABB-B", MeterSample{GridPowerW: -5000})
	if rB1.LimitAmps != 21 {
		t.Fatalf("charger B first: LimitAmps = %d, want 21", rB1.LimitAmps)
	}

	// Charger A again with small change — should hold at 13A (its own hysteresis state)
	rA2 := c.Compute("ABB-A", MeterSample{GridPowerW: -3100})
	if rA2.LimitAmps != 13 {
		t.Errorf("charger A hysteresis: LimitAmps = %d, want 13", rA2.LimitAmps)
	}

	// Charger B again with small change — should hold at 21A (its own hysteresis state)
	rB2 := c.Compute("ABB-B", MeterSample{GridPowerW: -5100})
	if rB2.LimitAmps != 21 {
		t.Errorf("charger B hysteresis: LimitAmps = %d, want 21", rB2.LimitAmps)
	}
}

func TestCalculator_CappedAtMaxAmps(t *testing.T) {
	c := NewCalculator(6, 16, 230.0) // maxAmps=16

	r := c.Compute("ABB-001", MeterSample{GridPowerW: -10000})
	if r.LimitAmps != 16 {
		t.Errorf("LimitAmps = %d, want 16 (capped at maxAmps)", r.LimitAmps)
	}
}

func TestCompute_SolarSurplusPrimary(t *testing.T) {
	c := NewCalculator(6, 32, 230.0)

	r := c.Compute("ABB-001", MeterSample{
		SolarPowerW:       5000,
		ConsumptionPowerW: 1000,
		GridPowerW:        0,
	})
	if r.LimitAmps != 17 {
		t.Errorf("Compute(5000-1000) = %d, want 17 (4000/230)", r.LimitAmps)
	}
	if r.ShouldStop {
		t.Errorf("ShouldStop = true, want false")
	}
}

func TestCompute_GridFallback(t *testing.T) {
	c := NewCalculator(6, 32, 230.0)

	r := c.Compute("ABB-001", MeterSample{
		GridPowerW: -4000,
	})
	if r.LimitAmps != 17 {
		t.Errorf("Compute(grid=-4000) = %d, want 17", r.LimitAmps)
	}
	if r.ShouldStop {
		t.Errorf("ShouldStop = true, want false")
	}
}

func TestCompute_SolarOnly_PartialSource(t *testing.T) {
	c := NewCalculator(6, 32, 230.0)

	r := c.Compute("ABB-001", MeterSample{
		SolarPowerW: 3000,
	})
	if r.LimitAmps != 13 {
		t.Errorf("Compute(3000-0) = %d, want 13 (3000/230)", r.LimitAmps)
	}
}

func TestCrossValidationDrift_Agreement(t *testing.T) {
	sample := MeterSample{
		SolarPowerW:       5000,
		ConsumptionPowerW: 1000,
		GridPowerW:        -4000,
	}
	drift := CrossValidationDrift(sample)
	if drift != 0 {
		t.Errorf("CrossValidationDrift() = %.0f, want 0 (solar-consumption=4000, grid=4000)", drift)
	}
}

func TestCrossValidationDrift_SensorDrift(t *testing.T) {
	sample := MeterSample{
		SolarPowerW:       5000,
		ConsumptionPowerW: 1000,
		GridPowerW:        1000,
	}
	drift := CrossValidationDrift(sample)
	if drift <= 500 {
		t.Errorf("CrossValidationDrift() = %.0f, want > 500 (solar-consumption=4000, grid=-1000)", drift)
	}
}

func TestHasSensorDrift_True(t *testing.T) {
	sample := MeterSample{
		SolarPowerW:       8000,
		ConsumptionPowerW: 2000,
		GridPowerW:        3000,
	}
	if !HasSensorDrift(sample) {
		t.Error("HasSensorDrift() = false, want true (drift = 9000 > 500)")
	}
}

func TestHasSensorDrift_False(t *testing.T) {
	tests := []struct {
		name   string
		sample MeterSample
	}{
		{
			name: "sources agree",
			sample: MeterSample{
				SolarPowerW:       4000,
				ConsumptionPowerW: 1000,
				GridPowerW:        -3000,
			},
		},
		{
			name: "only grid available",
			sample: MeterSample{
				GridPowerW: -4000,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if HasSensorDrift(tt.sample) {
				t.Errorf("HasSensorDrift() = true, want false")
			}
		})
	}
}

func TestCalculator_SetLimits_RaceFree(t *testing.T) {
	c := NewCalculator(6, 32, 230.0)
	done := make(chan struct{})

	sample := MeterSample{GridPowerW: -3000}

	go func() {
		defer close(done)
		for i := 0; i < 200; i++ {
			_ = c.Compute("CHG-001", sample)
		}
	}()

	for i := 0; i < 200; i++ {
		c.SetLimits(6+i%4, 32-i%4)
	}

	<-done
}

func TestCalculator_SetLimits_HotApplied(t *testing.T) {
	c := NewCalculator(6, 32, 230.0)

	r1 := c.Compute("CHG-001", MeterSample{GridPowerW: -10000})
	if r1.LimitAmps != 32 {
		t.Errorf("before: LimitAmps = %d, want 32", r1.LimitAmps)
	}

	c.SetLimits(6, 16)

	r2 := c.Compute("CHG-002", MeterSample{GridPowerW: -10000})
	if r2.LimitAmps != 16 {
		t.Errorf("after: LimitAmps = %d, want 16", r2.LimitAmps)
	}
}
