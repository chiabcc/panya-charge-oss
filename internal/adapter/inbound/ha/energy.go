package ha

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chiabcc/panya-charge-oss/internal/domain/ports"
)

const pollInterval = 10 * time.Second

type HASSConfig struct {
	GridEntityID        string
	SolarEntityID       string
	ConsumptionEntityID string
	Token               string
}

type stateResponse struct {
	State string `json:"state"`
	Attributes struct {
		UnitOfMeasurement string `json:"unit_of_measurement"`
	} `json:"attributes"`
}

// normalizeToWatts scales W/kW/MW to watts. Unknown or empty units pass through
// unchanged (assumed already watts) to preserve backward compatibility.
func normalizeToWatts(value float64, unit string) float64 {
	switch strings.ToLower(strings.TrimSpace(unit)) {
	case "", "w":
		return value
	case "kw":
		return value * 1_000
	case "mw":
		return value * 1_000_000
	default:
		return value
	}
}

type EnergySource struct {
	grid          atomic.Int64
	gridAt        time.Time
	solar         atomic.Int64
	solarAt       time.Time
	consumption   atomic.Int64
	consumptionAt time.Time
	mu            sync.RWMutex
	cancel        context.CancelFunc
	client        *http.Client
	baseURL       string
	token         string
	cfg           HASSConfig
	logger        *slog.Logger
}

func NewEnergySource(cfg HASSConfig, baseURL, token string, logger *slog.Logger) *EnergySource {
	if logger == nil {
		logger = slog.Default()
	}
	return &EnergySource{
		client:  &http.Client{Timeout: 5 * time.Second},
		baseURL: baseURL,
		token:   token,
		cfg:     cfg,
		logger:  logger,
	}
}

func (e *EnergySource) Start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	e.cancel = cancel
	go e.pollLoop(ctx)
}

func (e *EnergySource) Stop() {
	if e.cancel != nil {
		e.cancel()
	}
}

func (e *EnergySource) pollLoop(ctx context.Context) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.pollOnce()
		}
	}
}

func (e *EnergySource) pollOnce() {
	var wg sync.WaitGroup

	if e.cfg.GridEntityID != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			e.pollEntity(e.cfg.GridEntityID, "grid")
		}()
	}
	if e.cfg.SolarEntityID != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			e.pollEntity(e.cfg.SolarEntityID, "solar")
		}()
	}
	if e.cfg.ConsumptionEntityID != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			e.pollEntity(e.cfg.ConsumptionEntityID, "consumption")
		}()
	}

	wg.Wait()
}

func (e *EnergySource) pollEntity(entityID, entityType string) {
	url := e.baseURL + "/api/states/" + entityID

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		e.logger.Warn("ha energy: failed to create request", "entity_id", entityID, "error", err)
		return
	}
	req.Header.Set("Authorization", "Bearer "+e.token)

	resp, err := e.client.Do(req)
	if err != nil {
		e.logger.Debug("ha energy: failed to poll entity", "entity_id", entityID, "error", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		e.logger.Debug("ha energy: non-200 response for entity",
			"entity_id", entityID, "status", resp.StatusCode)
		return
	}

	var state stateResponse
	if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
		e.logger.Debug("ha energy: failed to decode response", "entity_id", entityID, "error", err)
		return
	}

	if state.State == "unavailable" || state.State == "unknown" {
		e.logger.Debug("ha energy: entity state unavailable, skipping",
			"entity_id", entityID, "state", state.State)
		return
	}

	powerW, err := strconv.ParseFloat(state.State, 64)
	if err != nil {
		e.logger.Debug("ha energy: non-numeric state value",
			"entity_id", entityID, "state", state.State)
		return
	}

	if norm := normalizeToWatts(powerW, state.Attributes.UnitOfMeasurement); norm != powerW {
		e.logger.Debug("ha energy: normalized unit to watts",
			"entity_id", entityID,
			"state", state.State,
			"unit", state.Attributes.UnitOfMeasurement,
			"watts", norm,
		)
		powerW = norm
	}

	switch entityType {
	case "grid":
		e.mu.Lock()
		e.gridAt = time.Now()
		e.mu.Unlock()
		e.grid.Store(int64(powerW))
	case "solar":
		e.mu.Lock()
		e.solarAt = time.Now()
		e.mu.Unlock()
		e.solar.Store(int64(powerW))
	case "consumption":
		e.mu.Lock()
		e.consumptionAt = time.Now()
		e.mu.Unlock()
		e.consumption.Store(int64(powerW))
	}
}

func (e *EnergySource) GetGridPowerW() float64 {
	return float64(e.grid.Load())
}

func (e *EnergySource) GetSolarPowerW() float64 {
	return float64(e.solar.Load())
}

func (e *EnergySource) GetConsumptionPowerW() float64 {
	return float64(e.consumption.Load())
}

func (e *EnergySource) IsStale(threshold time.Duration) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	latest := e.gridAt
	if e.solarAt.After(latest) {
		latest = e.solarAt
	}
	if e.consumptionAt.After(latest) {
		latest = e.consumptionAt
	}
	return time.Since(latest) > threshold
}

func (e *EnergySource) IsGridStale(threshold time.Duration) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return time.Since(e.gridAt) > threshold
}

func (e *EnergySource) IsSolarAvailable(threshold time.Duration) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return !e.solarAt.IsZero() && time.Since(e.solarAt) <= threshold
}

func (e *EnergySource) IsConsumptionAvailable(threshold time.Duration) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return !e.consumptionAt.IsZero() && time.Since(e.consumptionAt) <= threshold
}

// Compile-time interface compliance.
var _ ports.EnergySource = (*EnergySource)(nil)