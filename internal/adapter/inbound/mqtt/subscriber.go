package mqtt

import (
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type CommandHandler interface {
	OnSetAmps(chargerID string, amps int)
	OnSetState(chargerID string, charging bool)
	OnSetChargingMode(chargerID string, manual bool)
	OnSetSmartCharging(enabled bool)
	OnSetSolarThreshold(chargerID string, watts int)
}

type Subscriber struct {
	client     mqtt.Client
	baseTopic  string
	topics     map[string]string
	cmdHandler CommandHandler
	logger     *slog.Logger
}

func NewSubscriber(broker, clientID, username, password, baseTopic string, topics map[string]string, cmdHandler CommandHandler, logger *slog.Logger) (*Subscriber, error) {
	if logger == nil {
		return nil, fmt.Errorf("subscriber: logger must not be nil")
	}

	opts := mqtt.NewClientOptions()
	opts.AddBroker(broker)
	opts.SetClientID(clientID + "-sub")
	if username != "" {
		opts.SetUsername(username)
		opts.SetPassword(password)
	}
	opts.SetAutoReconnect(true)
	opts.SetCleanSession(true)
	opts.SetConnectRetry(true)
	opts.SetConnectRetryInterval(10 * time.Second)

	s := &Subscriber{
		baseTopic:  baseTopic,
		topics:     topics,
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
	_ = s.client.Connect()

	logger.Info("mqtt subscriber initialized", "broker", broker)
	return s, nil
}

func (s *Subscriber) resubscribe(c mqtt.Client) {
	_ = s.subscribe(c)
}

func (s *Subscriber) subscribe(c mqtt.Client) error {
	subs := []struct {
		topic   string
		handler mqtt.MessageHandler
	}{
		{s.fullTopic(s.topics["command_set_amps"]), s.handleSetAmpsGlobal},
		{s.fullTopic(s.topics["command_state"]), s.handleSetStateGlobal},
		{s.fullTopic(s.topics["smart_charging_command"]), s.handleSmartChargingCommand},
		{s.baseTopic + "/charge/+/command/set_amps", s.handleSetAmpsPerCharger},
		{s.baseTopic + "/charge/+/command/state", s.handleSetStatePerCharger},
		{s.baseTopic + "/charge/+/command/mode", s.handleModePerCharger},
		{s.baseTopic + "/charge/+/command/solar_threshold", s.handleSolarThresholdPerCharger},
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

func (s *Subscriber) handleModePerCharger(_ mqtt.Client, msg mqtt.Message) {
	chargerID := extractChargerID(msg.Topic(), s.baseTopic)
	if chargerID == "" {
		return
	}
	payload := strings.TrimSpace(string(msg.Payload()))
	switch payload {
	case "manual":
		if s.cmdHandler != nil {
			s.cmdHandler.OnSetChargingMode(chargerID, true)
		}
	case "auto":
		if s.cmdHandler != nil {
			s.cmdHandler.OnSetChargingMode(chargerID, false)
		}
	default:
		s.logger.Warn("invalid mode payload", "charger", chargerID, "payload", payload)
	}
}

func (s *Subscriber) handleSolarThresholdPerCharger(_ mqtt.Client, msg mqtt.Message) {
	chargerID := extractChargerID(msg.Topic(), s.baseTopic)
	if chargerID == "" {
		return
	}
	watts, err := strconv.Atoi(string(msg.Payload()))
	if err != nil {
		s.logger.Warn("invalid solar_threshold payload", "payload", string(msg.Payload()), "err", err)
		return
	}
	if s.cmdHandler != nil {
		s.cmdHandler.OnSetSolarThreshold(chargerID, watts)
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
