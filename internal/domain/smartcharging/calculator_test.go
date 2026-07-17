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
			name:      "small surplus below minimum — should stop",
			chargerID: "ABB-001",
			sample: MeterSample{
				GridPowerW: -500, // 500W export → 500/230≈2.17A < 6A minimum
			},
			wantAmps: 6,
			wantStop: true,
		},
		{
			name:      "grid importing — should stop",
			chargerID: "ABB-001",
			sample: MeterSample{
				GridPowerW: 2000, // importing 2kW
			},
			wantAmps: 6,
			wantStop: true,
		},
		{
			name:      "grid neutral — should stop",
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
