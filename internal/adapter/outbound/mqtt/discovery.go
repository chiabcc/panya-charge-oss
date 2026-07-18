package mqtt

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/chiabcc/panya-charge-oss/internal/domain/charger"
)

// HA MQTT Discovery spec: https://www.home-assistant.io/integrations/mqtt/#mqtt-discovery

type haDevice struct {
	Identifiers  []string `json:"identifiers"`
	Manufacturer string   `json:"manufacturer"`
	Model        string   `json:"model"`
	Name         string   `json:"name"`
	SWVersion    string   `json:"sw_version,omitempty"`
}

type haSensorConfig struct {
	Name                string   `json:"name"`
	StateTopic          string   `json:"state_topic"`
	UniqueID            string   `json:"unique_id"`
	Device              haDevice `json:"device"`
	AvailabilityTopic   string   `json:"availability_topic,omitempty"`
	PayloadAvailable    string   `json:"payload_available,omitempty"`
	PayloadNotAvailable string   `json:"payload_not_available,omitempty"`
	DeviceClass         string   `json:"device_class,omitempty"`
	StateClass          string   `json:"state_class,omitempty"`
	UnitOfMeasurement   string   `json:"unit_of_measurement,omitempty"`
	Icon                string   `json:"icon,omitempty"`
}

type haNumberConfig struct {
	Name                string   `json:"name"`
	StateTopic          string   `json:"state_topic"`
	CommandTopic        string   `json:"command_topic"`
	UniqueID            string   `json:"unique_id"`
	Device              haDevice `json:"device"`
	AvailabilityTopic   string   `json:"availability_topic,omitempty"`
	PayloadAvailable    string   `json:"payload_available,omitempty"`
	PayloadNotAvailable string   `json:"payload_not_available,omitempty"`
	Min                 float64  `json:"min"`
	Max                 float64  `json:"max"`
	Step                float64  `json:"step"`
	Mode                string   `json:"mode"`
	UnitOfMeasurement   string   `json:"unit_of_measurement,omitempty"`
	DeviceClass         string   `json:"device_class,omitempty"`
	EntityCategory      string   `json:"entity_category,omitempty"`
}

type haSwitchConfig struct {
	Name                string   `json:"name"`
	StateTopic          string   `json:"state_topic"`
	CommandTopic        string   `json:"command_topic"`
	UniqueID            string   `json:"unique_id"`
	Device              haDevice `json:"device"`
	AvailabilityTopic   string   `json:"availability_topic,omitempty"`
	PayloadAvailable    string   `json:"payload_available,omitempty"`
	PayloadNotAvailable string   `json:"payload_not_available,omitempty"`
	PayloadOn           string   `json:"payload_on"`
	PayloadOff          string   `json:"payload_off"`
	StateOn             string   `json:"state_on"`
	StateOff            string   `json:"state_off"`
	Icon                string   `json:"icon,omitempty"`
	EntityCategory      string   `json:"entity_category,omitempty"`
}

type haBinarySensorConfig struct {
	Name                string   `json:"name"`
	StateTopic          string   `json:"state_topic"`
	UniqueID            string   `json:"unique_id"`
	Device              haDevice `json:"device"`
	AvailabilityTopic   string   `json:"availability_topic,omitempty"`
	PayloadAvailable    string   `json:"payload_available,omitempty"`
	PayloadNotAvailable string   `json:"payload_not_available,omitempty"`
	PayloadOn           string   `json:"payload_on"`
	PayloadOff          string   `json:"payload_off"`
	DeviceClass         string   `json:"device_class"`
}

type discoveryPayload struct {
	topic   string
	payload any
}

func (dp discoveryPayload) encode() []byte {
	data, _ := json.Marshal(dp.payload)
	return data
}

func buildDiscoveryPayloads(c charger.Charger, baseTopic, gridTopic string, minAmps, maxAmps int, proxyEnabled bool) []discoveryPayload {
	nodeID := discoveryNodeID(c.ID)
	device := haDevice{
		Identifiers:  []string{nodeID},
		Manufacturer: strOrDefault(c.Vendor, "Panya"),
		Model:        strOrDefault(c.Model, "EV Charger"),
		Name:         "Panya Charge " + c.ID,
		SWVersion:    c.FirmwareVersion,
	}

	topic := func(suffix string) string {
		return fmt.Sprintf("%s/charge/%s/%s", baseTopic, c.ID, suffix)
	}
	discoveryTopic := func(component, objectID string) string {
		return fmt.Sprintf("homeassistant/%s/%s/%s/config", component, nodeID, objectID)
	}

	availTopic := topic("online")
	avail := "online"
	notAvail := "offline"

	payloads := []discoveryPayload{
		{
			discoveryTopic("sensor", "status"),
			haSensorConfig{
				Name: "Status", StateTopic: topic("status"), UniqueID: nodeID + "-status",
				Device: device, AvailabilityTopic: availTopic, PayloadAvailable: avail, PayloadNotAvailable: notAvail,
				Icon: "mdi:ev-station",
			},
		},
		{
			discoveryTopic("sensor", "power"),
			haSensorConfig{
				Name: "Charging Power", StateTopic: topic("power"), UniqueID: nodeID + "-power",
				Device: device, AvailabilityTopic: availTopic, PayloadAvailable: avail, PayloadNotAvailable: notAvail,
				DeviceClass: "power", StateClass: "measurement", UnitOfMeasurement: "kW", Icon: "mdi:flash",
			},
		},
		{
			discoveryTopic("sensor", "energy"),
			haSensorConfig{
				Name: "Session Energy", StateTopic: topic("energy"), UniqueID: nodeID + "-energy",
				Device: device, AvailabilityTopic: availTopic, PayloadAvailable: avail, PayloadNotAvailable: notAvail,
				DeviceClass: "energy", StateClass: "total_increasing", UnitOfMeasurement: "kWh", Icon: "mdi:counter",
			},
		},
		{
			discoveryTopic("sensor", "grid_power"),
			haSensorConfig{
				Name: "Grid Power", StateTopic: gridTopic, UniqueID: nodeID + "-grid-power",
				Device: device, AvailabilityTopic: availTopic, PayloadAvailable: avail, PayloadNotAvailable: notAvail,
				DeviceClass: "power", StateClass: "measurement", UnitOfMeasurement: "W", Icon: "mdi:transmission-tower",
			},
		},
		{
			discoveryTopic("number", "current"),
			haNumberConfig{
				Name: "Charging Current", StateTopic: topic("current"), CommandTopic: topic("command/set_amps"),
				UniqueID: nodeID + "-current",
				Device:   device, AvailabilityTopic: availTopic, PayloadAvailable: avail, PayloadNotAvailable: notAvail,
			Min: float64(minAmps), Max: float64(maxAmps), Step: 1, Mode: "slider",
			UnitOfMeasurement: "A", DeviceClass: "current", EntityCategory: "config",
			},
		},
		{
			discoveryTopic("switch", "charging"),
			haSwitchConfig{
				Name: "Charging", StateTopic: topic("charging_state"), CommandTopic: topic("command/state"),
				UniqueID: nodeID + "-charging",
				Device:   device, AvailabilityTopic: availTopic, PayloadAvailable: avail, PayloadNotAvailable: notAvail,
				PayloadOn: "start", PayloadOff: "stop", StateOn: "1", StateOff: "0", Icon: "mdi:power-plug", EntityCategory: "config",
			},
		},
	}

	if proxyEnabled {
		payloads = append(payloads, discoveryPayload{
			discoveryTopic("binary_sensor", "proxy_connected"),
			haBinarySensorConfig{
				Name:                "Proxy Connected",
				StateTopic:          topic("proxy_connected"),
				UniqueID:            nodeID + "-proxy-connected",
				Device:              device,
				AvailabilityTopic:   availTopic,
				PayloadAvailable:    avail,
				PayloadNotAvailable: notAvail,
				PayloadOn:           "ON",
				PayloadOff:          "OFF",
				DeviceClass:         "connectivity",
			},
		})
	}

	return payloads
}

func buildEnergySensorPayloads(device haDevice, nodeID, availTopic, solarTopic, consumptionTopic string) []discoveryPayload {
	avail := "online"
	notAvail := "offline"

	base := func(name, topic, objectID, icon string) discoveryPayload {
		return discoveryPayload{
			topic: fmt.Sprintf("homeassistant/sensor/%s/%s/config", nodeID, objectID),
			payload: haSensorConfig{
				Name:                name,
				StateTopic:          topic,
				UniqueID:            nodeID + "-" + objectID,
				Device:              device,
				AvailabilityTopic:   availTopic,
				PayloadAvailable:    avail,
				PayloadNotAvailable: notAvail,
				DeviceClass:         "power",
				StateClass:          "measurement",
				UnitOfMeasurement:   "W",
				Icon:                icon,
			},
		}
	}

	var payloads []discoveryPayload
	if solarTopic != "" {
		payloads = append(payloads, base("Solar Power", solarTopic, "solar_power", "mdi:solar-power"))
	}
	if consumptionTopic != "" {
		payloads = append(payloads, base("Home Consumption", consumptionTopic, "consumption_power", "mdi:home-lightning-bolt"))
	}
	return payloads
}

// PublishDiscovery publishes HA MQTT discovery payloads for all entities of a
// charger. Each entity is published to a retained config topic so HA picks it up
// on HA restart without needing the charger to reconnect.
func (p *Publisher) PublishDiscovery(c charger.Charger, minAmps, maxAmps int, proxyEnabled bool) {
	gridTopic := p.baseTopic + "/" + p.topics["grid_power"]
	payloads := buildDiscoveryPayloads(c, p.baseTopic, gridTopic, minAmps, maxAmps, proxyEnabled)

	nodeID := discoveryNodeID(c.ID)
	availTopic := fmt.Sprintf("%s/charge/%s/online", p.baseTopic, c.ID)
	device := haDevice{
		Identifiers:  []string{nodeID},
		Manufacturer: strOrDefault(c.Vendor, "Panya"),
		Model:        strOrDefault(c.Model, "EV Charger"),
		Name:         "Panya Charge " + c.ID,
		SWVersion:    c.FirmwareVersion,
	}

	solarTopic, consumptionTopic := "", ""
	if t, ok := p.topics["solar_power"]; ok && t != "" {
		solarTopic = p.baseTopic + "/" + t
	}
	if t, ok := p.topics["consumption_power"]; ok && t != "" {
		consumptionTopic = p.baseTopic + "/" + t
	}
	payloads = append(payloads, buildEnergySensorPayloads(device, nodeID, availTopic, solarTopic, consumptionTopic)...)

	for _, dp := range payloads {
		p.client.Publish(dp.topic, 1, true, dp.encode())
	}

	p.logger.Info("published HA discovery",
		"charger", c.ID,
		"node_id", nodeID,
		"entities", len(payloads),
	)
}

func discoveryNodeID(chargerID string) string {
	return "panya-charge-" + sanitizeID(chargerID)
}

// PublishGlobalDiscovery publishes HA MQTT discovery payloads for entities that
// are not tied to a specific charger (e.g. the global smart-charging switch).
// Must be called once on CSMS startup so HA picks up the global entity.
func (p *Publisher) PublishGlobalDiscovery() {
	const nodeID = "panya-charge"
	stateTopic := fmt.Sprintf("%s/smart_charging/state", p.baseTopic)
	commandTopic := fmt.Sprintf("%s/smart_charging/command", p.baseTopic)

	device := haDevice{
		Identifiers:  []string{nodeID},
		Manufacturer: "Panya",
		Model:        "CSMS",
		Name:         "Panya Charge",
	}

	payload := haSwitchConfig{
		Name:         "Smart Charging",
		StateTopic:   stateTopic,
		CommandTopic: commandTopic,
		UniqueID:     nodeID + "-smart-charging",
		Device:       device,
		PayloadOn:    "ON",
		PayloadOff:   "OFF",
		StateOn:      "ON",
		StateOff:     "OFF",
		Icon:         "mdi:solar-power",
	}

	topic := fmt.Sprintf("homeassistant/switch/%s/smart_charging/config", nodeID)
	p.client.Publish(topic, 1, true, discoveryPayload{payload: payload}.encode())
	p.logger.Info("published global HA discovery", "entity", "switch.smart_charging")
}

// sanitizeID replaces characters that are invalid in MQTT topics or HA entity IDs.
func sanitizeID(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, "_", "-")
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		}
	}
	result := b.String()
	if result == "" {
		return "unknown"
	}
	return result
}

func strOrDefault(val, def string) string {
	if val == "" {
		return def
	}
	return val
}
