package mqtt

import (
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"github.com/chiabcc/panya-charge-oss/internal/domain/charger"
)

type Publisher struct {
	client    mqtt.Client
	baseTopic string
	topics    map[string]string
	logger    *slog.Logger
}

func NewPublisher(broker, clientID, username, password, baseTopic string, topics map[string]string, logger *slog.Logger) (*Publisher, error) {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(broker)
	opts.SetClientID(clientID)
	if username != "" {
		opts.SetUsername(username)
		opts.SetPassword(password)
	}
	opts.SetAutoReconnect(true)
	opts.SetCleanSession(true)

	p := &Publisher{
		baseTopic: baseTopic,
		topics:    topics,
		logger:    logger,
	}

	opts.SetOnConnectHandler(func(c mqtt.Client) {
		p.logger.Info("mqtt connected", "broker", broker)
	})
	opts.SetConnectionLostHandler(func(c mqtt.Client, err error) {
		p.logger.Warn("mqtt connection lost", "err", err)
	})

	p.client = mqtt.NewClient(opts)
	if token := p.client.Connect(); token.Wait() && token.Error() != nil {
		return nil, fmt.Errorf("mqtt connect %s: %w", broker, token.Error())
	}

	logger.Info("mqtt publisher initialized", "broker", broker, "base_topic", baseTopic)
	return p, nil
}

func (p *Publisher) publish(topic string, payload string, retained bool) {
	fullTopic := fmt.Sprintf("%s/%s", p.baseTopic, topic)
	p.client.Publish(fullTopic, 1, retained, payload)
}

func (p *Publisher) PublishChargerStatus(chargerID string, status charger.ConnectorStatus) {
	p.publish(fmt.Sprintf("charge/%s/status", chargerID), string(status), false)
}

func (p *Publisher) PublishChargerPower(chargerID string, powerKW float64) {
	p.publish(fmt.Sprintf("charge/%s/power", chargerID), fmt.Sprintf("%.3f", powerKW), false)
}

func (p *Publisher) PublishSessionEnergy(chargerID string, energyKWh float64) {
	p.publish(fmt.Sprintf("charge/%s/energy", chargerID), fmt.Sprintf("%.3f", energyKWh), false)
}

func (p *Publisher) PublishChargerOnline(chargerID string, online bool) {
	val := "offline"
	if online {
		val = "online"
	}
	p.publish(fmt.Sprintf("charge/%s/online", chargerID), val, true)
}

func (p *Publisher) PublishChargerCurrent(chargerID string, amps int) {
	p.publish(fmt.Sprintf("charge/%s/current", chargerID), strconv.Itoa(amps), true)
}

func (p *Publisher) PublishChargingState(chargerID string, charging bool) {
	val := "0"
	if charging {
		val = "1"
	}
	p.publish(fmt.Sprintf("charge/%s/charging_state", chargerID), val, false)
}

func (p *Publisher) PublishProxyState(chargerID string, connected bool) {
	val := "OFF"
	if connected {
		val = "ON"
	}
	p.publish(fmt.Sprintf("charge/%s/proxy_connected", chargerID), val, true)
}

func (p *Publisher) Subscribe(topic string, handler mqtt.MessageHandler) error {
	fullTopic := fmt.Sprintf("%s/%s", p.baseTopic, topic)
	if token := p.client.Subscribe(fullTopic, 1, handler); token.Wait() && token.Error() != nil {
		return fmt.Errorf("subscribe %s: %w", fullTopic, token.Error())
	}
	p.logger.Info("mqtt subscribed", "topic", fullTopic)
	return nil
}

func (p *Publisher) Close() {
	p.client.Disconnect(500)
}

// IsConnected reports whether the MQTT client has an active broker connection.
func (p *Publisher) IsConnected() bool {
	return p.client.IsConnected()
}

// EnergyTracker tracks grid, solar, and consumption power readings from MQTT.
// It implements ports.EnergySource. Each source is tracked independently with
// per-source staleness detection.
type EnergyTracker struct {
	grid          atomic.Int64
	gridAt        time.Time
	solar         atomic.Int64
	solarAt       time.Time
	consumption   atomic.Int64
	consumptionAt time.Time
	mu            sync.RWMutex
}

func NewEnergyTracker() *EnergyTracker {
	return &EnergyTracker{}
}

func (e *EnergyTracker) UpdateGrid(powerW float64) {
	e.mu.Lock()
	e.gridAt = time.Now()
	e.mu.Unlock()
	e.grid.Store(int64(powerW))
}

func (e *EnergyTracker) UpdateSolar(powerW float64) {
	e.mu.Lock()
	e.solarAt = time.Now()
	e.mu.Unlock()
	e.solar.Store(int64(powerW))
}

func (e *EnergyTracker) UpdateConsumption(powerW float64) {
	e.mu.Lock()
	e.consumptionAt = time.Now()
	e.mu.Unlock()
	e.consumption.Store(int64(powerW))
}

func (e *EnergyTracker) GetGridPowerW() float64 {
	return float64(e.grid.Load())
}

func (e *EnergyTracker) GetSolarPowerW() float64 {
	return float64(e.solar.Load())
}

func (e *EnergyTracker) GetConsumptionPowerW() float64 {
	return float64(e.consumption.Load())
}

func (e *EnergyTracker) IsStale(threshold time.Duration) bool {
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

func (e *EnergyTracker) IsGridStale(threshold time.Duration) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return time.Since(e.gridAt) > threshold
}

func (e *EnergyTracker) IsSolarAvailable(threshold time.Duration) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return !e.solarAt.IsZero() && time.Since(e.solarAt) <= threshold
}

func (e *EnergyTracker) IsConsumptionAvailable(threshold time.Duration) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return !e.consumptionAt.IsZero() && time.Since(e.consumptionAt) <= threshold
}

// PublishGridPower publishes a grid power reading (W). Negative = surplus/exporting.
func (p *Publisher) PublishGridPower(watts float64) {
	topic := "grid/power"
	if t, ok := p.topics["grid_power"]; ok && t != "" {
		topic = t
	}
	p.publish(topic, strconv.FormatFloat(watts, 'f', 0, 64), false)
}

// PublishSimSetAmps publishes a set_amps command for a specific charger.
func (p *Publisher) PublishSimSetAmps(chargerID string, amps int) {
	p.publish(fmt.Sprintf("charge/%s/command/set_amps", chargerID), strconv.Itoa(amps), false)
}

// PublishSimSetState publishes a start/stop command for a specific charger.
func (p *Publisher) PublishSimSetState(chargerID string, charging bool) {
	val := "stop"
	if charging {
		val = "start"
	}
	p.publish(fmt.Sprintf("charge/%s/command/state", chargerID), val, false)
}
