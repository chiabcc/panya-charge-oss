// Package ports defines the hexagonal architecture port interfaces.
// Domain defines these; adapters implement them.
//
// Domain depends only on these interfaces — never on concrete adapters.
package ports

import (
	"context"
	"time"

	"github.com/chiabcc/panya-charge-oss/internal/domain/charger"
	"github.com/chiabcc/panya-charge-oss/internal/domain/proxy"
	"github.com/chiabcc/panya-charge-oss/internal/domain/session"
)

// ChargerRepository persists charger registrations and connector states.
type ChargerRepository interface {
	UpsertCharger(ctx context.Context, c charger.Charger) error
	GetCharger(ctx context.Context, id string) (*charger.Charger, error)
	ListChargers(ctx context.Context) ([]charger.Charger, error)
	MarkOnline(ctx context.Context, id string, online bool) error

	UpsertConnector(ctx context.Context, conn charger.Connector) error
	GetConnector(ctx context.Context, chargerID string, connectorID int) (*charger.Connector, error)
	ListConnectors(ctx context.Context, chargerID string) ([]charger.Connector, error)
}

// SessionRepository persists charging sessions.
type SessionRepository interface {
	CreateSession(ctx context.Context, s session.Session) error
	UpdateSession(ctx context.Context, s session.Session) error
	GetActiveSession(ctx context.Context, chargerID string, connectorID int) (*session.Session, error)
	GetSessionByTransactionID(ctx context.Context, chargerID string, txID int) (*session.Session, error)
	GetSession(ctx context.Context, id string) (*session.Session, error)
	ListSessions(ctx context.Context, limit, offset int) ([]session.Session, error)
}

// MeterRepository persists time-series meter values.
type MeterRepository interface {
	StoreMeterValue(ctx context.Context, mv MeterValue) error
	StoreMeterValues(ctx context.Context, mvs []MeterValue) error
	GetMeterValues(ctx context.Context, chargerID string, from, to time.Time) ([]MeterValue, error)
	GetMeterValuesBySession(ctx context.Context, sessionID string) ([]MeterValue, error)
	PurgeOlderThan(ctx context.Context, before time.Time) (int64, error)
}

type MeterValue struct {
	ChargerID   string
	ConnectorID int
	SessionID   string
	Timestamp   time.Time
	Measurand   string
	Value       float64
	Unit        string
	Phase       string
}

// EventPublisher is the outbound port for publishing domain events to MQTT.
// The MQTT adapter implements this.
type EventPublisher interface {
	PublishChargerStatus(chargerID string, status charger.ConnectorStatus)
	PublishChargerPower(chargerID string, powerKW float64)
	PublishSessionEnergy(chargerID string, energyKWh float64)
	PublishChargingEnergy(chargerID string, energyKWh float64)
	PublishChargerOnline(chargerID string, online bool)
	PublishChargerCurrent(chargerID string, amps int)
	PublishChargingState(chargerID string, charging bool)
	PublishProxyState(chargerID string, connected bool)
	PublishChargerMode(chargerID string, mode string)
	PublishSmartChargingEnabled(enabled bool)
	PublishSolarThreshold(chargerID string, watts int)
}

// DiscoveryPublisher publishes Home Assistant MQTT auto-discovery payloads.
type DiscoveryPublisher interface {
	PublishDiscovery(c charger.Charger, minAmps, maxAmps int, proxyEnabled bool)
}

// CommandReceiver is the inbound port for receiving commands from HA (via MQTT).
// The MQTT adapter calls these when commands arrive on command topics.
type CommandReceiver interface {
	OnSetAmps(chargerID string, amps int)
	OnSetState(chargerID string, charging bool)
	OnSetChargingMode(chargerID string, manual bool)
	OnSetSmartCharging(enabled bool)
	OnSetSolarThreshold(chargerID string, watts int)
}

// EnergySource provides real-time power readings from grid, solar, and
// consumption sensors (via HA/MQTT). Adapters implement this; the smart
// charging controller consumes it.
type EnergySource interface {
	// Start launches the adapter's polling/subscription loop. Must be safe
	// to call exactly once over the lifetime of the source. The context
	// cancels background work on shutdown.
	Start(ctx context.Context)

	// Stop tears down background work started by Start. Safe to call even
	// if Start was never invoked.
	Stop()

	GetGridPowerW() float64
	GetSolarPowerW() float64
	GetConsumptionPowerW() float64

	// IsStale returns true if no energy data (any source) has been received
	// within the threshold duration.
	IsStale(threshold time.Duration) bool

	// IsGridStale returns true if only grid-specific data is stale.
	IsGridStale(threshold time.Duration) bool

	// IsSolarAvailable returns true if solar data has been received and is fresh.
	IsSolarAvailable(threshold time.Duration) bool

	// IsConsumptionAvailable returns true if consumption data has been received and is fresh.
	IsConsumptionAvailable(threshold time.Duration) bool
}

// NoOpEnergySource implements EnergySource with always-stale, always-zero values.
// Used as the zero-value default so the smart charging controller can consume
// an EnergySource without a nil check before a real adapter is wired up.
type NoOpEnergySource struct{}

var _ EnergySource = (*NoOpEnergySource)(nil)

func (NoOpEnergySource) Start(context.Context)    {}
func (NoOpEnergySource) Stop()                    {}
func (NoOpEnergySource) GetGridPowerW() float64          { return 0 }
func (NoOpEnergySource) GetSolarPowerW() float64         { return 0 }
func (NoOpEnergySource) GetConsumptionPowerW() float64   { return 0 }
func (NoOpEnergySource) IsStale(time.Duration) bool      { return true }
func (NoOpEnergySource) IsGridStale(time.Duration) bool  { return true }
func (NoOpEnergySource) IsSolarAvailable(time.Duration) bool { return false }
func (NoOpEnergySource) IsConsumptionAvailable(time.Duration) bool { return false }

// ChargerCommander sends OCPP commands to a charger.
// The OCPP adapter implements this.
type ChargerCommander interface {
	// SetChargingProfile sends a TxDefaultProfile with the given amp limit.
	SetChargingProfile(chargerID string, connectorID int, limitAmps int) error
	// RemoteStartTransaction starts a transaction on the given connector.
	RemoteStartTransaction(chargerID string, connectorID int, idTag string) error
	// RemoteStopTransaction stops a transaction by OCPP transaction ID.
	RemoteStopTransaction(chargerID string, transactionID int) error
	// ClearChargingProfile removes charging profiles from the charger.
	ClearChargingProfile(chargerID string, connectorID int) error
}

// ProxyConfigRepository persists per-charger upstream relay configuration.
type ProxyConfigRepository interface {
	GetProxyConfig(ctx context.Context, chargerID string) (*proxy.ProxyConfig, error)
	UpsertProxyConfig(ctx context.Context, cfg proxy.ProxyConfig) error
}
