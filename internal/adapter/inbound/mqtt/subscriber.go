package mqtt

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	outmq "github.com/chiabcc/panya-charge-oss/internal/adapter/outbound/mqtt"
)

type CommandHandler interface {
	OnSetAmps(chargerID string, amps int)
	OnSetState(chargerID string, charging bool)
	OnSetSmartCharging(enabled bool)
}

type Subscriber struct {
	client     mqtt.Client
	baseTopic  string
	topics     map[string]string
	energy     *outmq.EnergyTracker
	cmdHandler CommandHandler
	logger     *slog.Logger
}

func NewSubscriber(broker, clientID, username, password, baseTopic string, topics map[string]string, energy *outmq.EnergyTracker, cmdHandler CommandHandler, logger *slog.Logger) (*Subscriber, error) {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(broker)
	opts.SetClientID(clientID + "-sub")
	if username != "" {
		opts.SetUsername(username)
		opts.SetPassword(password)
	}
	opts.SetAutoReconnect(true)
	opts.SetCleanSession(true)

	s := &Subscriber{
		baseTopic:  baseTopic,
		topics:     topics,
		energy:     energy,
		cmdHandler: cmdHandler,
		logger:     logger,
	}

	opts.SetOnConnectHandler(func(c mqtt.Client) {
		s.logger.Info("mqtt subscriber connected", "broker", broker)
		s.resubscribe(c)
	})
	opts.SetConnectionLostHandler(func(c mqtt.Client, err error) {
		s.logger.Warn("mqtt subscriber connection lost", "err", err)
	})

	s.client = mqtt.NewClient(opts)
	if token := s.client.Connect(); token.Wait() && token.Error() != nil {
		return nil, fmt.Errorf("subscriber mqtt connect %s: %w", broker, token.Error())
	}

	if err := s.setupHandlers(); err != nil {
		s.client.Disconnect(500)
		return nil, err
	}

	logger.Info("mqtt subscriber initialized", "broker", broker)
	return s, nil
}

func (s *Subscriber) setupHandlers() error {
	if err := s.subscribe(s.client); err != nil {
		return err
	}
	s.logger.Info("mqtt subscriber handlers set up")
	return nil
}

func (s *Subscriber) resubscribe(c mqtt.Client) {
	_ = s.subscribe(c)
}

func (s *Subscriber) subscribe(c mqtt.Client) error {
	subs := []struct {
		topic   string
		handler mqtt.MessageHandler
	}{
		{s.fullTopic(s.topics["grid_power"]), s.handleGridPower},
		{s.fullTopic(s.topics["command_set_amps"]), s.handleSetAmpsGlobal},
		{s.fullTopic(s.topics["command_state"]), s.handleSetStateGlobal},
		{s.fullTopic(s.topics["smart_charging_command"]), s.handleSmartChargingCommand},
		{s.baseTopic + "/charge/+/command/set_amps", s.handleSetAmpsPerCharger},
		{s.baseTopic + "/charge/+/command/state", s.handleSetStatePerCharger},
	}

	if solarTopic := s.topics["solar_power"]; solarTopic != "" {
		subs = append(subs, struct {
			topic   string
			handler mqtt.MessageHandler
		}{s.fullTopic(solarTopic), s.handleSolarPower})
	}
	if consumptionTopic := s.topics["consumption_power"]; consumptionTopic != "" {
		subs = append(subs, struct {
			topic   string
			handler mqtt.MessageHandler
		}{s.fullTopic(consumptionTopic), s.handleConsumptionPower})
	}

	for _, sub := range subs {
		if token := c.Subscribe(sub.topic, 1, sub.handler); token.Wait() && token.Error() != nil {
			return fmt.Errorf("subscribe %s: %w", sub.topic, token.Error())
		}
		s.logger.Info("subscribed", "topic", sub.topic)
	}
	return nil
}

func (s *Subscriber) fullTopic(suffix string) string {
	return fmt.Sprintf("%s/%s", s.baseTopic, suffix)
}

func (s *Subscriber) handleGridPower(_ mqtt.Client, msg mqtt.Message) {
	val, err := parsePowerPayload(msg.Payload())
	if err != nil {
		s.logger.Debug("ignoring non-numeric grid power payload", "payload", string(msg.Payload()), "err", err)
		return
	}
	s.energy.UpdateGrid(val)
	s.logger.Debug("grid power updated", "watts", val)
}

func (s *Subscriber) handleSolarPower(_ mqtt.Client, msg mqtt.Message) {
	val, err := parsePowerPayload(msg.Payload())
	if err != nil {
		s.logger.Debug("ignoring non-numeric solar power payload", "payload", string(msg.Payload()), "err", err)
		return
	}
	s.energy.UpdateSolar(val)
	s.logger.Debug("solar power updated", "watts", val)
}

func (s *Subscriber) handleConsumptionPower(_ mqtt.Client, msg mqtt.Message) {
	val, err := parsePowerPayload(msg.Payload())
	if err != nil {
		s.logger.Debug("ignoring non-numeric consumption power payload", "payload", string(msg.Payload()), "err", err)
		return
	}
	s.energy.UpdateConsumption(val)
	s.logger.Debug("consumption power updated", "watts", val)
}

func parsePowerPayload(payload []byte) (float64, error) {
	val, err := strconv.ParseFloat(string(payload), 64)
	if err != nil {
		var obj map[string]any
		if jsonErr := json.Unmarshal(payload, &obj); jsonErr == nil {
			if v, ok := obj["power"].(float64); ok {
				return v, nil
			}
		}
		return 0, err
	}
	return val, nil
}

func (s *Subscriber) handleSetAmpsGlobal(_ mqtt.Client, msg mqtt.Message) {
	amps, err := strconv.Atoi(string(msg.Payload()))
	if err != nil {
		s.logger.Warn("invalid set_amps payload", "payload", string(msg.Payload()), "err", err)
		return
	}
	if s.cmdHandler != nil {
		s.cmdHandler.OnSetAmps("", amps)
	}
}

func (s *Subscriber) handleSetStateGlobal(_ mqtt.Client, msg mqtt.Message) {
	payload := string(msg.Payload())
	charging := payload == "1" || payload == "true" || payload == "ON" || payload == "start"
	if s.cmdHandler != nil {
		s.cmdHandler.OnSetState("", charging)
	}
}

func (s *Subscriber) handleSmartChargingCommand(_ mqtt.Client, msg mqtt.Message) {
	payload := string(msg.Payload())
	enabled := payload == "1" || payload == "true" || payload == "ON"
	if s.cmdHandler != nil {
		s.cmdHandler.OnSetSmartCharging(enabled)
	}
}

func (s *Subscriber) handleSetAmpsPerCharger(_ mqtt.Client, msg mqtt.Message) {
	chargerID := extractChargerID(msg.Topic(), s.baseTopic)
	if chargerID == "" {
		return
	}
	amps, err := strconv.Atoi(string(msg.Payload()))
	if err != nil {
		s.logger.Warn("invalid per-charger set_amps payload", "payload", string(msg.Payload()), "err", err)
		return
	}
	if s.cmdHandler != nil {
		s.cmdHandler.OnSetAmps(chargerID, amps)
	}
}

func (s *Subscriber) handleSetStatePerCharger(_ mqtt.Client, msg mqtt.Message) {
	chargerID := extractChargerID(msg.Topic(), s.baseTopic)
	if chargerID == "" {
		return
	}
	payload := string(msg.Payload())
	charging := payload == "1" || payload == "true" || payload == "ON" || payload == "start"
	if s.cmdHandler != nil {
		s.cmdHandler.OnSetState(chargerID, charging)
	}
}

func extractChargerID(fullTopic, baseTopic string) string {
	prefix := baseTopic + "/charge/"
	if !strings.HasPrefix(fullTopic, prefix) {
		return ""
	}
	rest := strings.TrimPrefix(fullTopic, prefix)
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) < 1 {
		return ""
	}
	return parts[0]
}

func (s *Subscriber) Close() {
	s.client.Disconnect(500)
}

// IsConnected reports whether the subscriber's MQTT client has an active broker connection.
func (s *Subscriber) IsConnected() bool {
	return s.client.IsConnected()
}
