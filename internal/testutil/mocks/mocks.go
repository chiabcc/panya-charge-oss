// Package mocks provides hand-written mock implementations for all hexagonal
// port interfaces in internal/domain/ports. Use these in integration tests
// for the OCPP handler, controller, and MQTT adapter.
package mocks

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/chiabcc/panya-charge-oss/internal/domain/charger"
	"github.com/chiabcc/panya-charge-oss/internal/domain/ports"
	"github.com/chiabcc/panya-charge-oss/internal/domain/proxy"
	"github.com/chiabcc/panya-charge-oss/internal/domain/session"
)

// MockChargerRepository implements ports.ChargerRepository.
type MockChargerRepository struct {
	mu                 sync.Mutex
	Chargers           map[string]charger.Charger
	Connectors         map[string][]charger.Connector
	UpsertChargerErr   error
	GetChargerErr      error
	ListChargersErr    error
	MarkOnlineErr      error
	UpsertConnectorErr error
}

func NewMockChargerRepository() *MockChargerRepository {
	return &MockChargerRepository{
		Chargers:   make(map[string]charger.Charger),
		Connectors: make(map[string][]charger.Connector),
	}
}

func (m *MockChargerRepository) UpsertCharger(_ context.Context, c charger.Charger) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.UpsertChargerErr != nil {
		return m.UpsertChargerErr
	}
	m.Chargers[c.ID] = c
	return nil
}

func (m *MockChargerRepository) GetCharger(_ context.Context, id string) (*charger.Charger, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.GetChargerErr != nil {
		return nil, m.GetChargerErr
	}
	c, ok := m.Chargers[id]
	if !ok {
		return nil, errors.New("charger not found")
	}
	return &c, nil
}

func (m *MockChargerRepository) ListChargers(_ context.Context) ([]charger.Charger, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.ListChargersErr != nil {
		return nil, m.ListChargersErr
	}
	result := make([]charger.Charger, 0, len(m.Chargers))
	for _, c := range m.Chargers {
		result = append(result, c)
	}
	return result, nil
}

func (m *MockChargerRepository) MarkOnline(_ context.Context, id string, online bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.MarkOnlineErr != nil {
		return m.MarkOnlineErr
	}
	c, ok := m.Chargers[id]
	if !ok {
		return errors.New("charger not found")
	}
	c.Online = online
	m.Chargers[id] = c
	return nil
}

func (m *MockChargerRepository) UpsertConnector(_ context.Context, conn charger.Connector) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.UpsertConnectorErr != nil {
		return m.UpsertConnectorErr
	}
	conns, ok := m.Connectors[conn.ChargerID]
	if !ok {
		conns = []charger.Connector{}
	}
	for i, c := range conns {
		if c.ConnectorID == conn.ConnectorID {
			conns[i] = conn
			m.Connectors[conn.ChargerID] = conns
			return nil
		}
	}
	conns = append(conns, conn)
	m.Connectors[conn.ChargerID] = conns
	return nil
}

func (m *MockChargerRepository) GetConnector(_ context.Context, chargerID string, connectorID int) (*charger.Connector, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	conns, ok := m.Connectors[chargerID]
	if !ok {
		return nil, errors.New("connector not found")
	}
	for _, c := range conns {
		if c.ConnectorID == connectorID {
			return &c, nil
		}
	}
	return nil, errors.New("connector not found")
}

func (m *MockChargerRepository) ListConnectors(_ context.Context, chargerID string) ([]charger.Connector, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	conns, ok := m.Connectors[chargerID]
	if !ok {
		return nil, nil
	}
	result := make([]charger.Connector, len(conns))
	copy(result, conns)
	return result, nil
}

// MockSessionRepository implements ports.SessionRepository.
type MockSessionRepository struct {
	mu                  sync.Mutex
	Sessions            map[string]session.Session
	CreateSessionErr    error
	UpdateSessionErr    error
	GetActiveSessionErr error
}

func NewMockSessionRepository() *MockSessionRepository {
	return &MockSessionRepository{
		Sessions: make(map[string]session.Session),
	}
}

func (m *MockSessionRepository) CreateSession(_ context.Context, s session.Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.CreateSessionErr != nil {
		return m.CreateSessionErr
	}
	m.Sessions[s.ID] = s
	return nil
}

func (m *MockSessionRepository) UpdateSession(_ context.Context, s session.Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.UpdateSessionErr != nil {
		return m.UpdateSessionErr
	}
	m.Sessions[s.ID] = s
	return nil
}

func (m *MockSessionRepository) GetActiveSession(_ context.Context, chargerID string, connectorID int) (*session.Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.GetActiveSessionErr != nil {
		return nil, m.GetActiveSessionErr
	}
	for _, s := range m.Sessions {
		if s.ChargerID == chargerID && s.ConnectorID == connectorID && s.IsActive() {
			return &s, nil
		}
	}
	return nil, errors.New("no active session")
}

func (m *MockSessionRepository) GetSessionByTransactionID(_ context.Context, chargerID string, txID int) (*session.Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range m.Sessions {
		if s.ChargerID == chargerID && s.TransactionID == txID {
			return &s, nil
		}
	}
	return nil, errors.New("session not found")
}

func (m *MockSessionRepository) GetSession(_ context.Context, id string) (*session.Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.Sessions[id]
	if !ok {
		return nil, errors.New("session not found")
	}
	return &s, nil
}

func (m *MockSessionRepository) ListSessions(_ context.Context, limit, offset int) ([]session.Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	all := make([]session.Session, 0, len(m.Sessions))
	for _, s := range m.Sessions {
		all = append(all, s)
	}
	if offset > 0 && offset < len(all) {
		all = all[offset:]
	}
	if limit > 0 && limit < len(all) {
		all = all[:limit]
	}
	return all, nil
}

func (m *MockSessionRepository) Count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.Sessions)
}

// MockMeterRepository implements ports.MeterRepository.
type MockMeterRepository struct {
	mu                  sync.Mutex
	Values              []ports.MeterValue
	StoreMeterValueErr  error
	StoreMeterValuesErr error
	GetMeterValuesErr   error
}

func NewMockMeterRepository() *MockMeterRepository {
	return &MockMeterRepository{
		Values: make([]ports.MeterValue, 0),
	}
}

func (m *MockMeterRepository) StoreMeterValue(_ context.Context, mv ports.MeterValue) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.StoreMeterValueErr != nil {
		return m.StoreMeterValueErr
	}
	m.Values = append(m.Values, mv)
	return nil
}

func (m *MockMeterRepository) StoreMeterValues(_ context.Context, mvs []ports.MeterValue) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.StoreMeterValuesErr != nil {
		return m.StoreMeterValuesErr
	}
	m.Values = append(m.Values, mvs...)
	return nil
}

func (m *MockMeterRepository) GetMeterValues(_ context.Context, chargerID string, from, to time.Time) ([]ports.MeterValue, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.GetMeterValuesErr != nil {
		return nil, m.GetMeterValuesErr
	}
	var result []ports.MeterValue
	for _, mv := range m.Values {
		if mv.ChargerID == chargerID && !mv.Timestamp.Before(from) && !mv.Timestamp.After(to) {
			result = append(result, mv)
		}
	}
	return result, nil
}

func (m *MockMeterRepository) GetMeterValuesBySession(_ context.Context, sessionID string) ([]ports.MeterValue, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.GetMeterValuesErr != nil {
		return nil, m.GetMeterValuesErr
	}
	var result []ports.MeterValue
	for _, mv := range m.Values {
		if mv.SessionID == sessionID {
			result = append(result, mv)
		}
	}
	return result, nil
}

func (m *MockMeterRepository) PurgeOlderThan(_ context.Context, before time.Time) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	kept := make([]ports.MeterValue, 0, len(m.Values))
	purged := int64(0)
	for _, mv := range m.Values {
		if mv.Timestamp.Before(before) {
			purged++
		} else {
			kept = append(kept, mv)
		}
	}
	m.Values = kept
	return purged, nil
}

func (m *MockMeterRepository) Count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.Values)
}

// StatusPublish captures a PublishChargerStatus call.
type StatusPublish struct {
	ChargerID string
	Status    charger.ConnectorStatus
}

// PowerPublish captures a PublishChargerPower call.
type PowerPublish struct {
	ChargerID string
	PowerKW   float64
}

// EnergyPublish captures a PublishSessionEnergy call.
type EnergyPublish struct {
	ChargerID string
	EnergyKWh float64
}

// OnlinePublish captures a PublishChargerOnline call.
type OnlinePublish struct {
	ChargerID string
	Online    bool
}

// CurrentPublish captures a PublishChargerCurrent call.
type CurrentPublish struct {
	ChargerID string
	Amps      int
}

// ChargingPublish captures a PublishChargingState call.
type ChargingPublish struct {
	ChargerID string
	Charging  bool
}

// ProxyStatePublish captures a PublishProxyState call.
type ProxyStatePublish struct {
	ChargerID string
	Connected bool
}

// SmartChargingPublish captures a PublishSmartChargingEnabled call.
type SmartChargingPublish struct {
	Enabled bool
}

// MockEventPublisher implements ports.EventPublisher.
type MockEventPublisher struct {
	mu                       sync.Mutex
	StatusPublished          []StatusPublish
	PowerPublished           []PowerPublish
	EnergyPublished          []EnergyPublish
	OnlinePublished          []OnlinePublish
	CurrentPublished         []CurrentPublish
	ChargingPublished        []ChargingPublish
	ProxyPublished           []ProxyStatePublish
	SmartChargingPublished   []SmartChargingPublish
}

func NewMockEventPublisher() *MockEventPublisher {
	return &MockEventPublisher{
		StatusPublished:        make([]StatusPublish, 0),
		PowerPublished:         make([]PowerPublish, 0),
		EnergyPublished:        make([]EnergyPublish, 0),
		OnlinePublished:        make([]OnlinePublish, 0),
		CurrentPublished:       make([]CurrentPublish, 0),
		ChargingPublished:      make([]ChargingPublish, 0),
		ProxyPublished:         make([]ProxyStatePublish, 0),
		SmartChargingPublished: make([]SmartChargingPublish, 0),
	}
}

func (m *MockEventPublisher) PublishChargerStatus(chargerID string, status charger.ConnectorStatus) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.StatusPublished = append(m.StatusPublished, StatusPublish{ChargerID: chargerID, Status: status})
}

func (m *MockEventPublisher) PublishChargerPower(chargerID string, powerKW float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.PowerPublished = append(m.PowerPublished, PowerPublish{ChargerID: chargerID, PowerKW: powerKW})
}

func (m *MockEventPublisher) PublishSessionEnergy(chargerID string, energyKWh float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.EnergyPublished = append(m.EnergyPublished, EnergyPublish{ChargerID: chargerID, EnergyKWh: energyKWh})
}

func (m *MockEventPublisher) PublishChargerOnline(chargerID string, online bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.OnlinePublished = append(m.OnlinePublished, OnlinePublish{ChargerID: chargerID, Online: online})
}

func (m *MockEventPublisher) PublishChargerCurrent(chargerID string, amps int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.CurrentPublished = append(m.CurrentPublished, CurrentPublish{ChargerID: chargerID, Amps: amps})
}

func (m *MockEventPublisher) PublishChargingState(chargerID string, charging bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ChargingPublished = append(m.ChargingPublished, ChargingPublish{ChargerID: chargerID, Charging: charging})
}

func (m *MockEventPublisher) PublishProxyState(chargerID string, connected bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ProxyPublished = append(m.ProxyPublished, ProxyStatePublish{ChargerID: chargerID, Connected: connected})
}

func (m *MockEventPublisher) PublishSmartChargingEnabled(enabled bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.SmartChargingPublished = append(m.SmartChargingPublished, SmartChargingPublish{Enabled: enabled})
}

func (m *MockEventPublisher) LastStatus(chargerID string) (charger.ConnectorStatus, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := len(m.StatusPublished) - 1; i >= 0; i-- {
		if m.StatusPublished[i].ChargerID == chargerID {
			return m.StatusPublished[i].Status, true
		}
	}
	return "", false
}

func (m *MockEventPublisher) LastPower(chargerID string) (float64, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := len(m.PowerPublished) - 1; i >= 0; i-- {
		if m.PowerPublished[i].ChargerID == chargerID {
			return m.PowerPublished[i].PowerKW, true
		}
	}
	return 0, false
}

func (m *MockEventPublisher) LastOnline(chargerID string) (bool, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := len(m.OnlinePublished) - 1; i >= 0; i-- {
		if m.OnlinePublished[i].ChargerID == chargerID {
			return m.OnlinePublished[i].Online, true
		}
	}
	return false, false
}

func (m *MockEventPublisher) LastCurrent(chargerID string) (int, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := len(m.CurrentPublished) - 1; i >= 0; i-- {
		if m.CurrentPublished[i].ChargerID == chargerID {
			return m.CurrentPublished[i].Amps, true
		}
	}
	return 0, false
}

func (m *MockEventPublisher) LastCharging(chargerID string) (bool, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := len(m.ChargingPublished) - 1; i >= 0; i-- {
		if m.ChargingPublished[i].ChargerID == chargerID {
			return m.ChargingPublished[i].Charging, true
		}
	}
	return false, false
}

func (m *MockEventPublisher) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.StatusPublished = m.StatusPublished[:0]
	m.PowerPublished = m.PowerPublished[:0]
	m.EnergyPublished = m.EnergyPublished[:0]
	m.OnlinePublished = m.OnlinePublished[:0]
	m.CurrentPublished = m.CurrentPublished[:0]
	m.ChargingPublished = m.ChargingPublished[:0]
	m.ProxyPublished = m.ProxyPublished[:0]
	m.SmartChargingPublished = m.SmartChargingPublished[:0]
}

// DiscoveryCall captures a PublishDiscovery call.
type DiscoveryCall struct {
	Charger      charger.Charger
	MinAmps      int
	MaxAmps      int
	ProxyEnabled bool
}

// MockDiscoveryPublisher implements ports.DiscoveryPublisher.
type MockDiscoveryPublisher struct {
	mu             sync.Mutex
	DiscoveryCalls []DiscoveryCall
}

func NewMockDiscoveryPublisher() *MockDiscoveryPublisher {
	return &MockDiscoveryPublisher{
		DiscoveryCalls: make([]DiscoveryCall, 0),
	}
}

func (m *MockDiscoveryPublisher) PublishDiscovery(c charger.Charger, minAmps, maxAmps int, proxyEnabled bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.DiscoveryCalls = append(m.DiscoveryCalls, DiscoveryCall{
		Charger:      c,
		MinAmps:      minAmps,
		MaxAmps:      maxAmps,
		ProxyEnabled: proxyEnabled,
	})
}

func (m *MockDiscoveryPublisher) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.DiscoveryCalls)
}

func (m *MockDiscoveryPublisher) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.DiscoveryCalls = m.DiscoveryCalls[:0]
}

// MockEnergySource implements ports.EnergySource.
type MockEnergySource struct {
	GridPowerW        float64
	SolarPowerW       float64
	ConsumptionPowerW float64
	Stale             bool
	GridStale         bool
	SolarAvail        bool
	ConsumptionAvail  bool
}

func NewMockEnergySource() *MockEnergySource {
	return &MockEnergySource{}
}

func (m *MockEnergySource) GetGridPowerW() float64                      { return m.GridPowerW }
func (m *MockEnergySource) GetSolarPowerW() float64                     { return m.SolarPowerW }
func (m *MockEnergySource) GetConsumptionPowerW() float64               { return m.ConsumptionPowerW }
func (m *MockEnergySource) IsStale(_ time.Duration) bool                { return m.Stale }
func (m *MockEnergySource) IsGridStale(_ time.Duration) bool            { return m.GridStale }
func (m *MockEnergySource) IsSolarAvailable(_ time.Duration) bool       { return m.SolarAvail }
func (m *MockEnergySource) IsConsumptionAvailable(_ time.Duration) bool { return m.ConsumptionAvail }

// SetChargingProfileCall records a SetChargingProfile call.
type SetChargingProfileCall struct {
	ChargerID   string
	ConnectorID int
	LimitAmps   int
}

// RemoteStartCall records a RemoteStartTransaction call.
type RemoteStartCall struct {
	ChargerID   string
	ConnectorID int
	IDTag       string
}

// RemoteStopCall records a RemoteStopTransaction call.
type RemoteStopCall struct {
	ChargerID     string
	TransactionID int
}

// ClearChargingProfileCall records a ClearChargingProfile call.
type ClearChargingProfileCall struct {
	ChargerID   string
	ConnectorID int
}

// MockChargerCommander implements ports.ChargerCommander.
type MockChargerCommander struct {
	mu                        sync.Mutex
	SetChargingProfileCalls   []SetChargingProfileCall
	RemoteStartCalls          []RemoteStartCall
	RemoteStopCalls           []RemoteStopCall
	ClearChargingProfileCalls []ClearChargingProfileCall
	SetChargingProfileErr     error
	RemoteStartErr            error
	RemoteStopErr             error
	ClearChargingProfileErr   error
}

func NewMockChargerCommander() *MockChargerCommander {
	return &MockChargerCommander{
		SetChargingProfileCalls:   make([]SetChargingProfileCall, 0),
		RemoteStartCalls:          make([]RemoteStartCall, 0),
		RemoteStopCalls:           make([]RemoteStopCall, 0),
		ClearChargingProfileCalls: make([]ClearChargingProfileCall, 0),
	}
}

func (m *MockChargerCommander) SetChargingProfile(chargerID string, connectorID int, limitAmps int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.SetChargingProfileCalls = append(m.SetChargingProfileCalls, SetChargingProfileCall{
		ChargerID:   chargerID,
		ConnectorID: connectorID,
		LimitAmps:   limitAmps,
	})
	return m.SetChargingProfileErr
}

func (m *MockChargerCommander) RemoteStartTransaction(chargerID string, connectorID int, idTag string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.RemoteStartCalls = append(m.RemoteStartCalls, RemoteStartCall{
		ChargerID:   chargerID,
		ConnectorID: connectorID,
		IDTag:       idTag,
	})
	return m.RemoteStartErr
}

func (m *MockChargerCommander) RemoteStopTransaction(chargerID string, transactionID int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.RemoteStopCalls = append(m.RemoteStopCalls, RemoteStopCall{
		ChargerID:     chargerID,
		TransactionID: transactionID,
	})
	return m.RemoteStopErr
}

func (m *MockChargerCommander) ClearChargingProfile(chargerID string, connectorID int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ClearChargingProfileCalls = append(m.ClearChargingProfileCalls, ClearChargingProfileCall{
		ChargerID:   chargerID,
		ConnectorID: connectorID,
	})
	return m.ClearChargingProfileErr
}

func (m *MockChargerCommander) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.SetChargingProfileCalls = m.SetChargingProfileCalls[:0]
	m.RemoteStartCalls = m.RemoteStartCalls[:0]
	m.RemoteStopCalls = m.RemoteStopCalls[:0]
	m.ClearChargingProfileCalls = m.ClearChargingProfileCalls[:0]
	m.SetChargingProfileErr = nil
	m.RemoteStartErr = nil
	m.RemoteStopErr = nil
	m.ClearChargingProfileErr = nil
}

// SetAmpsCall records an OnSetAmps call.
type SetAmpsCall struct {
	ChargerID string
	Amps      int
}

// SetStateCall records an OnSetState call.
type SetStateCall struct {
	ChargerID string
	Charging  bool
}

// SetSmartChargingCall records an OnSetSmartCharging call.
type SetSmartChargingCall struct {
	Enabled bool
}

// MockCommandReceiver implements ports.CommandReceiver.
type MockCommandReceiver struct {
	mu                   sync.Mutex
	SetAmpsCalls         []SetAmpsCall
	SetStateCalls        []SetStateCall
	SetSmartChargingCalls []SetSmartChargingCall
}

func NewMockCommandReceiver() *MockCommandReceiver {
	return &MockCommandReceiver{
		SetAmpsCalls:          make([]SetAmpsCall, 0),
		SetStateCalls:         make([]SetStateCall, 0),
		SetSmartChargingCalls: make([]SetSmartChargingCall, 0),
	}
}

func (m *MockCommandReceiver) OnSetAmps(chargerID string, amps int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.SetAmpsCalls = append(m.SetAmpsCalls, SetAmpsCall{ChargerID: chargerID, Amps: amps})
}

func (m *MockCommandReceiver) OnSetState(chargerID string, charging bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.SetStateCalls = append(m.SetStateCalls, SetStateCall{ChargerID: chargerID, Charging: charging})
}

func (m *MockCommandReceiver) OnSetSmartCharging(enabled bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.SetSmartChargingCalls = append(m.SetSmartChargingCalls, SetSmartChargingCall{Enabled: enabled})
}

func (m *MockCommandReceiver) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.SetAmpsCalls = m.SetAmpsCalls[:0]
	m.SetStateCalls = m.SetStateCalls[:0]
	m.SetSmartChargingCalls = m.SetSmartChargingCalls[:0]
}

type MockProxyConfigRepo struct {
	mu      sync.Mutex
	configs map[string]proxy.ProxyConfig
	err     error
}

func NewMockProxyConfigRepo() *MockProxyConfigRepo {
	return &MockProxyConfigRepo{
		configs: make(map[string]proxy.ProxyConfig),
	}
}

func (m *MockProxyConfigRepo) GetProxyConfig(_ context.Context, chargerID string) (*proxy.ProxyConfig, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return nil, m.err
	}
	cfg, ok := m.configs[chargerID]
	if !ok {
		return nil, nil
	}
	return &cfg, nil
}

func (m *MockProxyConfigRepo) UpsertProxyConfig(_ context.Context, cfg proxy.ProxyConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return m.err
	}
	m.configs[cfg.ChargerID] = cfg
	return nil
}

func (m *MockProxyConfigRepo) SetError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.err = err
}

type MockProxyRelay struct {
	mu              sync.Mutex
	Connected       map[string]bool
	ConnectCalls    []string
	DisconnectCalls []string
	ConnectErr      error
}

func NewMockProxyRelay() *MockProxyRelay {
	return &MockProxyRelay{
		Connected:       make(map[string]bool),
		ConnectCalls:    make([]string, 0),
		DisconnectCalls: make([]string, 0),
	}
}

func (m *MockProxyRelay) Connect(_ context.Context, chargerID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ConnectCalls = append(m.ConnectCalls, chargerID)
	if m.ConnectErr != nil {
		return m.ConnectErr
	}
	m.Connected[chargerID] = true
	return nil
}

func (m *MockProxyRelay) Disconnect(chargerID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.DisconnectCalls = append(m.DisconnectCalls, chargerID)
	m.Connected[chargerID] = false
}

func (m *MockProxyRelay) IsConnected(chargerID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.Connected[chargerID]
}

// Compile-time interface assertions.
var (
	_ ports.ChargerRepository     = (*MockChargerRepository)(nil)
	_ ports.SessionRepository     = (*MockSessionRepository)(nil)
	_ ports.MeterRepository       = (*MockMeterRepository)(nil)
	_ ports.EventPublisher        = (*MockEventPublisher)(nil)
	_ ports.DiscoveryPublisher    = (*MockDiscoveryPublisher)(nil)
	_ ports.EnergySource          = (*MockEnergySource)(nil)
	_ ports.ChargerCommander      = (*MockChargerCommander)(nil)
	_ ports.CommandReceiver       = (*MockCommandReceiver)(nil)
	_ ports.ProxyConfigRepository = (*MockProxyConfigRepo)(nil)
)
