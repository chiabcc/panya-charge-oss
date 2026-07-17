package ports

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/chiabcc/panya-charge-oss/internal/domain/charger"
	"github.com/chiabcc/panya-charge-oss/internal/domain/proxy"
	"github.com/chiabcc/panya-charge-oss/internal/domain/session"
)

// InMemoryChargerRepository implements ChargerRepository with an in-memory store.
type InMemoryChargerRepository struct {
	mu         sync.RWMutex
	chargers   map[string]charger.Charger
	connectors map[string][]charger.Connector
}

func NewInMemoryChargerRepository() *InMemoryChargerRepository {
	return &InMemoryChargerRepository{
		chargers:   make(map[string]charger.Charger),
		connectors: make(map[string][]charger.Connector),
	}
}

func (r *InMemoryChargerRepository) UpsertCharger(_ context.Context, c charger.Charger) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.chargers[c.ID] = c
	return nil
}

func (r *InMemoryChargerRepository) GetCharger(_ context.Context, id string) (*charger.Charger, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.chargers[id]
	if !ok {
		return nil, fmt.Errorf("charger not found: %s", id)
	}
	return &c, nil
}

func (r *InMemoryChargerRepository) ListChargers(_ context.Context) ([]charger.Charger, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := make([]charger.Charger, 0, len(r.chargers))
	for _, c := range r.chargers {
		list = append(list, c)
	}
	return list, nil
}

func (r *InMemoryChargerRepository) MarkOnline(_ context.Context, id string, online bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.chargers[id]
	if !ok {
		return fmt.Errorf("charger not found: %s", id)
	}
	c.Online = online
	r.chargers[id] = c
	return nil
}

func (r *InMemoryChargerRepository) UpsertConnector(_ context.Context, conn charger.Connector) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	conns := r.connectors[conn.ChargerID]
	found := false
	for i, c := range conns {
		if c.ConnectorID == conn.ConnectorID {
			conns[i] = conn
			found = true
			break
		}
	}
	if !found {
		conns = append(conns, conn)
	}
	r.connectors[conn.ChargerID] = conns
	return nil
}

func (r *InMemoryChargerRepository) GetConnector(_ context.Context, chargerID string, connectorID int) (*charger.Connector, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	conns := r.connectors[chargerID]
	for _, c := range conns {
		if c.ConnectorID == connectorID {
			return &c, nil
		}
	}
	return nil, fmt.Errorf("connector not found: charger=%s, connector=%d", chargerID, connectorID)
}

func (r *InMemoryChargerRepository) ListConnectors(_ context.Context, chargerID string) ([]charger.Connector, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	conns := r.connectors[chargerID]
	if conns == nil {
		return []charger.Connector{}, nil
	}
	result := make([]charger.Connector, len(conns))
	copy(result, conns)
	return result, nil
}

// InMemorySessionRepository implements SessionRepository with an in-memory store.
type InMemorySessionRepository struct {
	mu       sync.RWMutex
	sessions map[string]session.Session
}

func NewInMemorySessionRepository() *InMemorySessionRepository {
	return &InMemorySessionRepository{
		sessions: make(map[string]session.Session),
	}
}

func (r *InMemorySessionRepository) CreateSession(_ context.Context, s session.Session) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s.ID == "" {
		s.ID = fmt.Sprintf("sess-%d", time.Now().UnixNano())
	}
	r.sessions[s.ID] = s
	return nil
}

func (r *InMemorySessionRepository) UpdateSession(_ context.Context, s session.Session) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.sessions[s.ID]; !ok {
		return fmt.Errorf("session not found: %s", s.ID)
	}
	r.sessions[s.ID] = s
	return nil
}

func (r *InMemorySessionRepository) GetActiveSession(_ context.Context, chargerID string, connectorID int) (*session.Session, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, s := range r.sessions {
		if s.ChargerID == chargerID && s.ConnectorID == connectorID && s.StoppedAt == nil {
			return &s, nil
		}
	}
	return nil, nil
}

func (r *InMemorySessionRepository) GetSessionByTransactionID(_ context.Context, chargerID string, txID int) (*session.Session, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, s := range r.sessions {
		if s.ChargerID == chargerID && s.TransactionID == txID {
			return &s, nil
		}
	}
	return nil, nil
}

func (r *InMemorySessionRepository) GetSession(_ context.Context, id string) (*session.Session, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.sessions[id]
	if !ok {
		return nil, fmt.Errorf("session not found: %s", id)
	}
	return &s, nil
}

func (r *InMemorySessionRepository) ListSessions(_ context.Context, limit, offset int) ([]session.Session, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	all := make([]session.Session, 0, len(r.sessions))
	for _, s := range r.sessions {
		all = append(all, s)
	}
	if offset >= len(all) {
		return []session.Session{}, nil
	}
	end := offset + limit
	if end > len(all) {
		end = len(all)
	}
	return all[offset:end], nil
}

// InMemoryMeterRepository implements MeterRepository with an in-memory store.
type InMemoryMeterRepository struct {
	mu     sync.RWMutex
	values []MeterValue
}

func NewInMemoryMeterRepository() *InMemoryMeterRepository {
	return &InMemoryMeterRepository{
		values: make([]MeterValue, 0),
	}
}

func (r *InMemoryMeterRepository) StoreMeterValue(_ context.Context, mv MeterValue) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.values = append(r.values, mv)
	return nil
}

func (r *InMemoryMeterRepository) StoreMeterValues(_ context.Context, mvs []MeterValue) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.values = append(r.values, mvs...)
	return nil
}

func (r *InMemoryMeterRepository) GetMeterValues(_ context.Context, chargerID string, from, to time.Time) ([]MeterValue, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []MeterValue
	for _, mv := range r.values {
		if mv.ChargerID == chargerID && !mv.Timestamp.Before(from) && !mv.Timestamp.After(to) {
			result = append(result, mv)
		}
	}
	return result, nil
}

func (r *InMemoryMeterRepository) GetMeterValuesBySession(_ context.Context, sessionID string) ([]MeterValue, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []MeterValue
	for _, mv := range r.values {
		if mv.SessionID == sessionID {
			result = append(result, mv)
		}
	}
	return result, nil
}

func (r *InMemoryMeterRepository) PurgeOlderThan(_ context.Context, before time.Time) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	kept := make([]MeterValue, 0, len(r.values))
	purged := int64(0)
	for _, mv := range r.values {
		if !mv.Timestamp.Before(before) {
			kept = append(kept, mv)
		} else {
			purged++
		}
	}
	r.values = kept
	return purged, nil
}

// InMemoryProxyConfigRepository implements ProxyConfigRepository with an in-memory store.
type InMemoryProxyConfigRepository struct {
	mu      sync.RWMutex
	configs map[string]proxy.ProxyConfig
}

func NewInMemoryProxyConfigRepository() *InMemoryProxyConfigRepository {
	return &InMemoryProxyConfigRepository{
		configs: make(map[string]proxy.ProxyConfig),
	}
}

func (r *InMemoryProxyConfigRepository) GetProxyConfig(_ context.Context, chargerID string) (*proxy.ProxyConfig, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	cfg, ok := r.configs[chargerID]
	if !ok {
		return nil, nil
	}
	return &cfg, nil
}

func (r *InMemoryProxyConfigRepository) UpsertProxyConfig(_ context.Context, cfg proxy.ProxyConfig) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.configs[cfg.ChargerID] = cfg
	return nil
}

// Compile-time assertions that in-memory implementations satisfy the ports.
var (
	_ ChargerRepository        = (*InMemoryChargerRepository)(nil)
	_ SessionRepository        = (*InMemorySessionRepository)(nil)
	_ MeterRepository          = (*InMemoryMeterRepository)(nil)
	_ ProxyConfigRepository    = (*InMemoryProxyConfigRepository)(nil)
)
