package mqtt

import (
	"fmt"
	"log/slog"
	"strconv"
	"sync/atomic"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"github.com/chiabcc/panya-charge-oss/internal/domain/charger"
)

type Publisher struct {
	client      mqtt.Client
	baseTopic   string
	topics      map[string]string
	broker      string
	onReconnect func()
	connected   atomic.Bool
	logger      *slog.Logger
}

func NewPublisher(broker, clientID, username, password, baseTopic string, topics map[string]string, logger *slog.Logger) (*Publisher, error) {
	if logger == nil {
		return nil, fmt.Errorf("publisher: logger must not be nil")
	}

	opts := mqtt.NewClientOptions()
	opts.AddBroker(broker)
	opts.SetClientID(clientID)
	if username != "" {
		opts.SetUsername(username)
		opts.SetPassword(password)
	}
	opts.SetAutoReconnect(true)
	opts.SetCleanSession(true)
	opts.SetConnectRetry(true)
	opts.SetConnectRetryInterval(10 * time.Second)

	p := &Publisher{
		broker:    broker,
		baseTopic: baseTopic,
		topics:    topics,
		logger:    logger,
	}

	opts.SetOnConnectHandler(func(c mqtt.Client) {
		p.connected.Store(true)
		p.logger.Info("mqtt connected", "broker", broker)
		if p.onReconnect != nil {
			p.onReconnect()
		}
	})
	opts.SetConnectionLostHandler(func(c mqtt.Client, err error) {
		p.connected.Store(false)
		p.logger.Warn("mqtt connection lost", "err", err)
	})

	p.client = mqtt.NewClient(opts)
	_ = p.client.Connect()

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

func (p *Publisher) PublishChargerMode(chargerID, mode string) {
	p.publish(fmt.Sprintf("charge/%s/mode", chargerID), mode, true)
}

func (p *Publisher) PublishSmartChargingEnabled(enabled bool) {
	val := "OFF"
	if enabled {
		val = "ON"
	}
	p.publish("smart_charging/state", val, true)
}

func (p *Publisher) Subscribe(topic string, handler mqtt.MessageHandler) error {
	fullTopic := fmt.Sprintf("%s/%s", p.baseTopic, topic)
	if token := p.client.Subscribe(fullTopic, 1, handler); token.Wait() && token.Error() != nil {
		return fmt.Errorf("subscribe %s: %w", fullTopic, token.Error())
	}
	p.logger.Info("mqtt subscribed", "topic", fullTopic)
	return nil
}

// SetOnReconnect sets a callback invoked each time the MQTT client connects
// (including the initial connect and all auto-reconnects).
func (p *Publisher) SetOnReconnect(fn func()) {
	p.onReconnect = fn
}

func (p *Publisher) Close() {
	p.client.Disconnect(500)
}

// IsConnected reports whether the MQTT client has an active broker connection.
func (p *Publisher) IsConnected() bool {
	return p.client.IsConnected()
}

// Status returns the MQTT connection state and broker address.
func (p *Publisher) Status() (connected bool, broker string) {
	return p.connected.Load(), p.broker
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
