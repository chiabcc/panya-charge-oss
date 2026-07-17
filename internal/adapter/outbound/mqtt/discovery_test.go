package mqtt

import (
	"encoding/json"
	"testing"

	"github.com/chiabcc/panya-charge-oss/internal/domain/charger"
)

func buildTestPayloads(t *testing.T, c charger.Charger) []discoveryPayload {
	t.Helper()
	return buildDiscoveryPayloads(c, "panya", "panya/grid/power", 6, 32, false)
}

func TestDiscoveryPayloadCount(t *testing.T) {
	c := charger.Charger{ID: "ABB-001", Vendor: "ABB", Model: "Terra AC"}
	payloads := buildTestPayloads(t, c)

	components := map[string]bool{}
	for _, dp := range payloads {
		components[dp.topic] = true
	}
	if len(components) != 6 {
		t.Errorf("expected 6 unique topics, got %d", len(components))
	}
}

func TestDiscoveryTopics(t *testing.T) {
	c := charger.Charger{ID: "EVSE-01", Vendor: "ABB", Model: "Terra AC"}
	payloads := buildDiscoveryPayloads(c, "panya", "panya/grid/power", 6, 32, false)

	expected := []string{
		"homeassistant/sensor/panya-charge-evse-01/status/config",
		"homeassistant/sensor/panya-charge-evse-01/power/config",
		"homeassistant/sensor/panya-charge-evse-01/energy/config",
		"homeassistant/sensor/panya-charge-evse-01/grid_power/config",
		"homeassistant/number/panya-charge-evse-01/current/config",
		"homeassistant/switch/panya-charge-evse-01/charging/config",
	}

	topics := make(map[string]bool)
	for _, dp := range payloads {
		topics[dp.topic] = true
	}
	for _, et := range expected {
		if !topics[et] {
			t.Errorf("missing discovery topic: %s", et)
		}
	}
}

func TestDiscoveryDeviceBlock(t *testing.T) {
	c := charger.Charger{
		ID:              "ABB-001",
		Vendor:          "ABB",
		Model:           "Terra AC W22-G5-R",
		FirmwareVersion: "1.8.32",
	}
	payloads := buildTestPayloads(t, c)

	for _, dp := range payloads {
		var cfg map[string]any
		if err := json.Unmarshal(dp.encode(), &cfg); err != nil {
			t.Fatalf("invalid JSON on %s: %v", dp.topic, err)
		}

		device, ok := cfg["device"].(map[string]any)
		if !ok {
			t.Errorf("missing device block on %s", dp.topic)
			continue
		}
		ids, ok := device["identifiers"].([]any)
		if !ok || len(ids) != 1 {
			t.Errorf("device.identifiers must have exactly 1 entry on %s", dp.topic)
			continue
		}
		if ids[0] != "panya-charge-abb-001" {
			t.Errorf("expected identifier 'panya-charge-abb-001', got '%v' on %s", ids[0], dp.topic)
		}
		if m, _ := device["manufacturer"].(string); m != "ABB" {
			t.Errorf("expected manufacturer 'ABB', got '%s' on %s", m, dp.topic)
		}
		if m, _ := device["model"].(string); m != "Terra AC W22-G5-R" {
			t.Errorf("expected model 'Terra AC W22-G5-R', got '%s' on %s", m, dp.topic)
		}
	}
}

func TestDiscoveryDeviceBlockDefaultVendor(t *testing.T) {
	c := charger.Charger{ID: "X-01", Vendor: "", Model: ""}
	payloads := buildTestPayloads(t, c)

	for _, dp := range payloads {
		var cfg map[string]any
		_ = json.Unmarshal(dp.encode(), &cfg)
		device := cfg["device"].(map[string]any)
		if m, _ := device["manufacturer"].(string); m != "Panya" {
			t.Errorf("expected default manufacturer 'Panya', got '%s' on %s", m, dp.topic)
		}
		if m, _ := device["model"].(string); m != "EV Charger" {
			t.Errorf("expected default model 'EV Charger', got '%s' on %s", m, dp.topic)
		}
	}
}

func TestDiscoveryUniqueIDs(t *testing.T) {
	c := charger.Charger{ID: "ABB-001"}
	payloads := buildTestPayloads(t, c)

	expected := map[string]string{
		"homeassistant/sensor/panya-charge-abb-001/status/config":     "panya-charge-abb-001-status",
		"homeassistant/sensor/panya-charge-abb-001/power/config":      "panya-charge-abb-001-power",
		"homeassistant/sensor/panya-charge-abb-001/energy/config":     "panya-charge-abb-001-energy",
		"homeassistant/sensor/panya-charge-abb-001/grid_power/config": "panya-charge-abb-001-grid-power",
		"homeassistant/number/panya-charge-abb-001/current/config":    "panya-charge-abb-001-current",
		"homeassistant/switch/panya-charge-abb-001/charging/config":   "panya-charge-abb-001-charging",
	}

	for _, dp := range payloads {
		var cfg map[string]any
		_ = json.Unmarshal(dp.encode(), &cfg)
		uid, _ := cfg["unique_id"].(string)
		if expected[dp.topic] == "" {
			t.Errorf("unexpected topic %s", dp.topic)
			continue
		}
		if uid != expected[dp.topic] {
			t.Errorf("on %s: expected unique_id '%s', got '%s'", dp.topic, expected[dp.topic], uid)
		}
	}
}

func TestDiscoverySensorFields(t *testing.T) {
	c := charger.Charger{ID: "ABB-001", Vendor: "ABB", Model: "Terra AC"}
	payloads := buildDiscoveryPayloads(c, "panya", "panya/grid/power", 6, 32, false)

	var powerCfg haSensorConfig
	found := false
	for _, dp := range payloads {
		if dp.topic == "homeassistant/sensor/panya-charge-abb-001/power/config" {
			_ = json.Unmarshal(dp.encode(), &powerCfg)
			found = true
			break
		}
	}
	if !found {
		t.Fatal("power sensor config not found")
	}

	if powerCfg.DeviceClass != "power" {
		t.Errorf("expected device_class 'power', got '%s'", powerCfg.DeviceClass)
	}
	if powerCfg.StateClass != "measurement" {
		t.Errorf("expected state_class 'measurement', got '%s'", powerCfg.StateClass)
	}
	if powerCfg.UnitOfMeasurement != "kW" {
		t.Errorf("expected unit 'kW', got '%s'", powerCfg.UnitOfMeasurement)
	}
	if powerCfg.StateTopic != "panya/charge/ABB-001/power" {
		t.Errorf("expected state_topic 'panya/charge/ABB-001/power', got '%s'", powerCfg.StateTopic)
	}
}

func TestDiscoveryEnergyFields(t *testing.T) {
	c := charger.Charger{ID: "ABB-001"}
	payloads := buildDiscoveryPayloads(c, "panya", "panya/grid/power", 6, 32, false)

	var energyCfg haSensorConfig
	for _, dp := range payloads {
		if dp.topic == "homeassistant/sensor/panya-charge-abb-001/energy/config" {
			_ = json.Unmarshal(dp.encode(), &energyCfg)
			break
		}
	}

	if energyCfg.DeviceClass != "energy" {
		t.Errorf("expected device_class 'energy', got '%s'", energyCfg.DeviceClass)
	}
	if energyCfg.StateClass != "total_increasing" {
		t.Errorf("expected state_class 'total_increasing', got '%s'", energyCfg.StateClass)
	}
}

func TestDiscoveryNumberFields(t *testing.T) {
	c := charger.Charger{ID: "ABB-001"}
	payloads := buildDiscoveryPayloads(c, "panya", "panya/grid/power", 6, 32, false)

	var numCfg haNumberConfig
	for _, dp := range payloads {
		if dp.topic == "homeassistant/number/panya-charge-abb-001/current/config" {
			_ = json.Unmarshal(dp.encode(), &numCfg)
			break
		}
	}

	if numCfg.Min != 6 {
		t.Errorf("expected min 6, got %v", numCfg.Min)
	}
	if numCfg.Max != 32 {
		t.Errorf("expected max 32, got %v", numCfg.Max)
	}
	if numCfg.Step != 1 {
		t.Errorf("expected step 1, got %v", numCfg.Step)
	}
	if numCfg.Mode != "slider" {
		t.Errorf("expected mode 'slider', got '%s'", numCfg.Mode)
	}
	if numCfg.UnitOfMeasurement != "A" {
		t.Errorf("expected unit 'A', got '%s'", numCfg.UnitOfMeasurement)
	}
	if numCfg.CommandTopic != "panya/charge/ABB-001/command/set_amps" {
		t.Errorf("expected command_topic 'panya/charge/ABB-001/command/set_amps', got '%s'", numCfg.CommandTopic)
	}
}

func TestDiscoverySwitchFields(t *testing.T) {
	c := charger.Charger{ID: "ABB-001"}
	payloads := buildDiscoveryPayloads(c, "panya", "panya/grid/power", 6, 32, false)

	var swCfg haSwitchConfig
	for _, dp := range payloads {
		if dp.topic == "homeassistant/switch/panya-charge-abb-001/charging/config" {
			_ = json.Unmarshal(dp.encode(), &swCfg)
			break
		}
	}

	if swCfg.PayloadOn != "start" {
		t.Errorf("expected payload_on 'start', got '%s'", swCfg.PayloadOn)
	}
	if swCfg.PayloadOff != "stop" {
		t.Errorf("expected payload_off 'stop', got '%s'", swCfg.PayloadOff)
	}
	if swCfg.StateOn != "1" {
		t.Errorf("expected state_on '1', got '%s'", swCfg.StateOn)
	}
	if swCfg.StateOff != "0" {
		t.Errorf("expected state_off '0', got '%s'", swCfg.StateOff)
	}
	if swCfg.CommandTopic != "panya/charge/ABB-001/command/state" {
		t.Errorf("expected command_topic 'panya/charge/ABB-001/command/state', got '%s'", swCfg.CommandTopic)
	}
}

func TestDiscoveryAvailability(t *testing.T) {
	c := charger.Charger{ID: "ABB-001"}
	payloads := buildDiscoveryPayloads(c, "panya", "panya/grid/power", 6, 32, false)

	for _, dp := range payloads {
		var cfg map[string]any
		_ = json.Unmarshal(dp.encode(), &cfg)

		if at, ok := cfg["availability_topic"].(string); !ok || at == "" {
			t.Errorf("missing availability_topic on %s", dp.topic)
			continue
		}
		if at, _ := cfg["availability_topic"].(string); at != "panya/charge/ABB-001/online" {
			t.Errorf("expected availability_topic 'panya/charge/ABB-001/online', got '%s' on %s", at, dp.topic)
		}
		if pa, _ := cfg["payload_available"].(string); pa != "online" {
			t.Errorf("expected payload_available 'online' on %s, got '%s'", dp.topic, pa)
		}
		if pna, _ := cfg["payload_not_available"].(string); pna != "offline" {
			t.Errorf("expected payload_not_available 'offline' on %s, got '%s'", dp.topic, pna)
		}
	}
}

func TestDiscoveryGridPowerTopic(t *testing.T) {
	c := charger.Charger{ID: "ABB-001"}
	payloads := buildDiscoveryPayloads(c, "panya", "custom/grid/topic", 6, 32, false)

	var gridCfg haSensorConfig
	for _, dp := range payloads {
		if dp.topic == "homeassistant/sensor/panya-charge-abb-001/grid_power/config" {
			_ = json.Unmarshal(dp.encode(), &gridCfg)
			break
		}
	}
	if gridCfg.StateTopic != "custom/grid/topic" {
		t.Errorf("expected grid state_topic 'custom/grid/topic', got '%s'", gridCfg.StateTopic)
	}
}

func TestDiscoveryProxySensorEnabled(t *testing.T) {
	c := charger.Charger{ID: "ABB-001"}
	payloads := buildDiscoveryPayloads(c, "panya", "panya/grid/power", 6, 32, true)

	if len(payloads) != 7 {
		t.Fatalf("expected 7 discovery payloads with proxy enabled, got %d", len(payloads))
	}

	found := false
	for _, dp := range payloads {
		if dp.topic == "homeassistant/binary_sensor/panya-charge-abb-001/proxy_connected/config" {
			found = true
			break
		}
	}
	if !found {
		t.Error("proxy binary_sensor topic not found when proxy is enabled")
	}

	var proxyCfg haBinarySensorConfig
	for _, dp := range payloads {
		if dp.topic == "homeassistant/binary_sensor/panya-charge-abb-001/proxy_connected/config" {
			_ = json.Unmarshal(dp.encode(), &proxyCfg)
			break
		}
	}
	if proxyCfg.DeviceClass != "connectivity" {
		t.Errorf("expected device_class 'connectivity', got '%s'", proxyCfg.DeviceClass)
	}
	if proxyCfg.PayloadOn != "ON" {
		t.Errorf("expected payload_on 'ON', got '%s'", proxyCfg.PayloadOn)
	}
	if proxyCfg.PayloadOff != "OFF" {
		t.Errorf("expected payload_off 'OFF', got '%s'", proxyCfg.PayloadOff)
	}
	if proxyCfg.StateTopic != "panya/charge/ABB-001/proxy_connected" {
		t.Errorf("expected state_topic 'panya/charge/ABB-001/proxy_connected', got '%s'", proxyCfg.StateTopic)
	}
	if proxyCfg.UniqueID != "panya-charge-abb-001-proxy-connected" {
		t.Errorf("expected unique_id 'panya-charge-abb-001-proxy-connected', got '%s'", proxyCfg.UniqueID)
	}
}

func TestDiscoveryProxySensorDisabled(t *testing.T) {
	c := charger.Charger{ID: "ABB-001"}
	payloads := buildDiscoveryPayloads(c, "panya", "panya/grid/power", 6, 32, false)

	if len(payloads) != 6 {
		t.Fatalf("expected 6 discovery payloads with proxy disabled, got %d", len(payloads))
	}

	for _, dp := range payloads {
		if dp.topic == "homeassistant/binary_sensor/panya-charge-abb-001/proxy_connected/config" {
			t.Error("proxy binary_sensor topic should not be present when proxy is disabled")
		}
	}
}

func TestSanitizeID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"ABB-001", "abb-001"},
		{"EVSE_02", "evse-02"},
		{"Charger 3", "charger-3"},
		{"ABB/Terra", "abbterra"},
		{"UPPER", "upper"},
		{"", "unknown"},
		{"123", "123"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := sanitizeID(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeID(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestDiscoveryNodeID(t *testing.T) {
	if id := discoveryNodeID("ABB-001"); id != "panya-charge-abb-001" {
		t.Errorf("expected 'panya-charge-abb-001', got '%s'", id)
	}
}

func TestStrOrDefault(t *testing.T) {
	if v := strOrDefault("", "fallback"); v != "fallback" {
		t.Errorf("expected 'fallback', got '%s'", v)
	}
	if v := strOrDefault("value", "fallback"); v != "value" {
		t.Errorf("expected 'value', got '%s'", v)
	}
}

func TestBuildEnergySensorPayloads_None(t *testing.T) {
	device := haDevice{Identifiers: []string{"test"}}
	payloads := buildEnergySensorPayloads(device, "test-node", "avail", "", "")
	if len(payloads) != 0 {
		t.Errorf("expected 0 payloads with empty topics, got %d", len(payloads))
	}
}

func TestBuildEnergySensorPayloads_SolarOnly(t *testing.T) {
	device := haDevice{Identifiers: []string{"test"}}
	payloads := buildEnergySensorPayloads(device, "test-node", "avail", "solar/topic", "")
	if len(payloads) != 1 {
		t.Fatalf("expected 1 payload, got %d", len(payloads))
	}
	var cfg haSensorConfig
	json.Unmarshal(payloads[0].encode(), &cfg)
	if cfg.Name != "Solar Power" {
		t.Errorf("expected name 'Solar Power', got '%s'", cfg.Name)
	}
	if cfg.StateTopic != "solar/topic" {
		t.Errorf("expected state_topic 'solar/topic', got '%s'", cfg.StateTopic)
	}
	if cfg.UniqueID != "test-node-solar_power" {
		t.Errorf("expected unique_id 'test-node-solar_power', got '%s'", cfg.UniqueID)
	}
	if cfg.DeviceClass != "power" || cfg.StateClass != "measurement" || cfg.UnitOfMeasurement != "W" {
		t.Errorf("unexpected device/state/unit: %s/%s/%s", cfg.DeviceClass, cfg.StateClass, cfg.UnitOfMeasurement)
	}
}

func TestBuildEnergySensorPayloads_ConsumptionOnly(t *testing.T) {
	device := haDevice{Identifiers: []string{"test"}}
	payloads := buildEnergySensorPayloads(device, "test-node", "avail", "", "consumption/topic")
	if len(payloads) != 1 {
		t.Fatalf("expected 1 payload, got %d", len(payloads))
	}
	var cfg haSensorConfig
	json.Unmarshal(payloads[0].encode(), &cfg)
	if cfg.Name != "Home Consumption" {
		t.Errorf("expected name 'Home Consumption', got '%s'", cfg.Name)
	}
	if cfg.StateTopic != "consumption/topic" {
		t.Errorf("expected state_topic 'consumption/topic', got '%s'", cfg.StateTopic)
	}
}

func TestBuildEnergySensorPayloads_Both(t *testing.T) {
	device := haDevice{Identifiers: []string{"test"}}
	payloads := buildEnergySensorPayloads(device, "test-node", "avail", "solar/topic", "consumption/topic")
	if len(payloads) != 2 {
		t.Fatalf("expected 2 payloads, got %d", len(payloads))
	}
}
