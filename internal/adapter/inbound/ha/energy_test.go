package ha

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func newMockHASServer(t *testing.T, handler func(http.ResponseWriter, *http.Request)) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(handler))
	t.Cleanup(ts.Close)
	return ts
}

func newEntityState(state string) []byte {
	body, _ := json.Marshal(map[string]string{"state": state})
	return body
}

func newEntityStateWithUnit(state, unit string) []byte {
	body, _ := json.Marshal(map[string]any{
		"state": state,
		"attributes": map[string]string{
			"unit_of_measurement": unit,
		},
	})
	return body
}

func TestPoll_SuccessfulResponse(t *testing.T) {
	var hitCount atomic.Int32

	ts := newMockHASServer(t, func(w http.ResponseWriter, r *http.Request) {
		hitCount.Add(1)
		_, _ = w.Write(newEntityState("2779"))
	})

	es := NewEnergySource(
		HASSConfig{GridEntityID: "sensor.grid"},
		ts.URL,
		"test-token",
		nil,
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	es.Start(ctx)

	deadline := time.After(11 * time.Second)
	for {
		select {
		case <-ctx.Done():
			t.Fatal("context cancelled before first poll")
		case <-deadline:
			t.Fatal("timed out waiting for first poll")
		default:
		}
		if hitCount.Load() > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	got := es.GetGridPowerW()
	if got != 2779.0 {
		t.Errorf("GetGridPowerW() = %f, want 2779.0", got)
	}

	if es.IsSolarAvailable(60 * time.Second) {
		t.Error("expected solar to be unavailable (no solar entity configured)")
	}
}

func TestPoll_EntityNotFound_404(t *testing.T) {
	var callCount atomic.Int32

	ts := newMockHASServer(t, func(w http.ResponseWriter, r *http.Request) {
		n := callCount.Add(1)
		if n == 1 {
			_, _ = w.Write(newEntityState("1000"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	es := NewEnergySource(
		HASSConfig{GridEntityID: "sensor.grid"},
		ts.URL,
		"test-token",
		nil,
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	es.Start(ctx)

	// Wait for first successful poll (up to 11s).
	deadline := time.After(11 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for first poll")
		default:
		}
		if es.GetGridPowerW() == 1000.0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Wait for second poll cycle (returns 404).
	time.Sleep(12 * time.Second)

	if got := es.GetGridPowerW(); got != 1000.0 {
		t.Errorf("GetGridPowerW() after 404 = %f, want 1000.0", got)
	}

	if es.IsGridStale(30 * time.Second) {
		t.Error("timestamp changed after 404 — expected last known timestamp")
	}
}

func TestPoll_TokenExpired_401(t *testing.T) {
	ts := newMockHASServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})

	es := NewEnergySource(
		HASSConfig{GridEntityID: "sensor.grid"},
		ts.URL,
		"expired-token",
		nil,
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	es.Start(ctx)

	time.Sleep(11 * time.Second)

	got := es.GetGridPowerW()
	if got != 0.0 {
		t.Errorf("GetGridPowerW() = %f, want 0.0 after 401", got)
	}

	if !es.IsGridStale(1*time.Second) {
		t.Error("expected grid to be stale after only 401 responses")
	}
}

func TestPoll_HARestarting_503(t *testing.T) {
	ts := newMockHASServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})

	es := NewEnergySource(
		HASSConfig{GridEntityID: "sensor.grid"},
		ts.URL,
		"test-token",
		nil,
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	es.Start(ctx)

	time.Sleep(11 * time.Second)

	got := es.GetGridPowerW()
	if got != 0.0 {
		t.Errorf("GetGridPowerW() = %f, want 0.0 after 503", got)
	}

	if !es.IsStale(1 * time.Second) {
		t.Error("expected overall state to be stale after only 503 responses")
	}
}

func TestPoll_Timeout(t *testing.T) {
	ts := newMockHASServer(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Second)
		_, _ = w.Write(newEntityState("9999"))
	})

	es := NewEnergySource(
		HASSConfig{GridEntityID: "sensor.grid"},
		ts.URL,
		"test-token",
		nil,
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	es.Start(ctx)

	time.Sleep(11 * time.Second)

	got := es.GetGridPowerW()
	if got != 0.0 {
		t.Errorf("GetGridPowerW() = %f, want 0.0 after timeout", got)
	}

	if !es.IsGridStale(1*time.Second) {
		t.Error("expected grid to be stale after timeout")
	}
}

func TestPoll_UnavailableState(t *testing.T) {
	ts := newMockHASServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(newEntityState("unavailable"))
	})

	es := NewEnergySource(
		HASSConfig{GridEntityID: "sensor.grid"},
		ts.URL,
		"test-token",
		nil,
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	es.Start(ctx)

	time.Sleep(11 * time.Second)

	got := es.GetGridPowerW()
	if got != 0.0 {
		t.Errorf("GetGridPowerW() = %f, want 0.0 for unavailable state", got)
	}

	if !es.IsGridStale(1*time.Second) {
		t.Error("expected grid to be stale for unavailable state")
	}
}

func TestPoll_NonNumericState(t *testing.T) {
	ts := newMockHASServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(newEntityState("on"))
	})

	es := NewEnergySource(
		HASSConfig{GridEntityID: "sensor.grid"},
		ts.URL,
		"test-token",
		nil,
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	es.Start(ctx)

	time.Sleep(11 * time.Second)

	got := es.GetGridPowerW()
	if got != 0.0 {
		t.Errorf("GetGridPowerW() = %f, want 0.0 for non-numeric state", got)
	}

	if !es.IsGridStale(1*time.Second) {
		t.Error("expected grid to be stale for non-numeric state")
	}
}

func TestStaleness_InitialState(t *testing.T) {
	es := NewEnergySource(
		HASSConfig{GridEntityID: "sensor.grid"},
		"http://example.com/api",
		"token",
		nil,
	)

	if !es.IsStale(60 * time.Second) {
		t.Error("IsStale(60s) = false, want true for fresh adapter")
	}

	if !es.IsGridStale(60 * time.Second) {
		t.Error("IsGridStale(60s) = false, want true for fresh adapter")
	}

	if es.IsSolarAvailable(60 * time.Second) {
		t.Error("IsSolarAvailable(60s) = true, want false for fresh adapter")
	}

	if es.IsConsumptionAvailable(60 * time.Second) {
		t.Error("IsConsumptionAvailable(60s) = true, want false for fresh adapter")
	}
}

func TestStop_CancelsPoller(t *testing.T) {
	ts := newMockHASServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(newEntityState("42"))
	})

	es := NewEnergySource(
		HASSConfig{GridEntityID: "sensor.grid"},
		ts.URL,
		"test-token",
		nil,
	)

	ctx, cancel := context.WithCancel(context.Background())
	es.Start(ctx)

	time.Sleep(11 * time.Second)

	es.Stop()
	cancel()

	// Read values after stop to exercise the race detector under -race.
	v := es.GetGridPowerW()
	_ = v

	for i := 0; i < 100; i++ {
		es.GetGridPowerW()
		es.GetSolarPowerW()
		es.GetConsumptionPowerW()
		es.IsStale(60 * time.Second)
		es.IsGridStale(60 * time.Second)
		es.IsSolarAvailable(60 * time.Second)
		es.IsConsumptionAvailable(60 * time.Second)
	}

	time.Sleep(100 * time.Millisecond)
}

func TestConcurrentAccess(t *testing.T) {
	ts := newMockHASServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(newEntityState("1000"))
	})

	es := NewEnergySource(
		HASSConfig{
			GridEntityID:        "sensor.grid",
			SolarEntityID:       "sensor.solar",
			ConsumptionEntityID: "sensor.consumption",
		},
		ts.URL,
		"test-token",
		nil,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	es.Start(ctx)

	done := make(chan struct{})

	go func() {
		defer close(done)
		for i := 0; i < 200; i++ {
			es.GetGridPowerW()
			es.GetSolarPowerW()
			es.GetConsumptionPowerW()
			es.IsStale(60 * time.Second)
			es.IsGridStale(60 * time.Second)
			es.IsSolarAvailable(60 * time.Second)
			es.IsConsumptionAvailable(60 * time.Second)
			time.Sleep(10 * time.Millisecond)
		}
	}()

	time.Sleep(12 * time.Second)
	<-done

	es.Stop()
}

func TestPollOnce_UnitConversion(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		state string
		unit  string
		want  float64
	}{
		{"watts", "2779", "W", 2779},
		{"kilowatts", "3.953", "kW", 3953},
		{"kilowatts_lowercase", "3", "kw", 3000},
		{"kilowatts_padded", "3", " kW ", 3000},
		{"megawatts", "0.005", "MW", 5000},
		{"no_unit", "1500", "", 1500},
		{"unknown_unit_passthrough", "1500", "horses", 1500},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ts := newMockHASServer(t, func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write(newEntityStateWithUnit(tc.state, tc.unit))
			})

			es := NewEnergySource(
				HASSConfig{
					GridEntityID:        "sensor.grid",
					SolarEntityID:       "sensor.solar",
					ConsumptionEntityID: "sensor.consumption",
				},
				ts.URL,
				"test-token",
				nil,
			)
			es.pollOnce()

			if got := es.GetGridPowerW(); got != tc.want {
				t.Errorf("GetGridPowerW() = %f, want %f (state=%q unit=%q)", got, tc.want, tc.state, tc.unit)
			}
			if got := es.GetSolarPowerW(); got != tc.want {
				t.Errorf("GetSolarPowerW() = %f, want %f", got, tc.want)
			}
			if got := es.GetConsumptionPowerW(); got != tc.want {
				t.Errorf("GetConsumptionPowerW() = %f, want %f", got, tc.want)
			}
		})
	}
}

func TestNormalizeToWatts(t *testing.T) {
	t.Parallel()

	cases := []struct {
		value float64
		unit  string
		want  float64
	}{
		{2779, "W", 2779},
		{2779, "w", 2779},
		{3.953, "kW", 3953},
		{3.953, "KW", 3953},
		{3.953, " kW ", 3953},
		{0.005, "MW", 5000},
		{1500, "", 1500},
		{1500, "horses", 1500},
		{-0.001, "kW", -1},
		{0, "kW", 0},
	}

	for _, tc := range cases {
		if got := normalizeToWatts(tc.value, tc.unit); got != tc.want {
			t.Errorf("normalizeToWatts(%v, %q) = %v, want %v", tc.value, tc.unit, got, tc.want)
		}
	}
}

// TestPollOnce_URLOnlySingleAPI guards against the double-/api/ bug that caused
// the add-on to call /core/api/api/states/... instead of /core/api/states/...
// (which returned 404 from HA Core after the Supervisor auth middleware let it through).
func TestPollOnce_URLOnlySingleAPI(t *testing.T) {
	t.Parallel()

	var capturedPath string
	ts := newMockHASServer(t, func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		_, _ = w.Write(newEntityState("1000"))
	})

	es := NewEnergySource(
		HASSConfig{GridEntityID: "sensor.grid"},
		ts.URL, // simulate baseURL root like "http://supervisor/core"
		"test-token",
		nil,
	)
	es.pollOnce()

	const want = "/api/states/sensor.grid"
	if capturedPath != want {
		t.Errorf("request path = %q, want %q (no double /api/)", capturedPath, want)
	}
}
