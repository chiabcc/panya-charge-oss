package config

import (
	"slices"
	"testing"
)

func TestClassify(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		old, new   *Config
		wantClass  ApplyClass
		wantFields []string
		wantWarn    bool
	}{
		{
			name:       "zero diff",
			old:        newConfig(),
			new:        newConfig(),
			wantClass:  ApplyNone,
			wantFields: nil,
			wantWarn:   false,
		},
		{
			name: "hot: log_level",
			old: func() *Config {
				cfg := newConfig()
				cfg.Server.LogLevel = "info"
				return cfg
			}(),
			new: func() *Config {
				cfg := newConfig()
				cfg.Server.LogLevel = "debug"
				return cfg
			}(),
			wantClass: ApplyHot,
			wantFields: []string{"server.log_level"},
			wantWarn:  false,
		},
		{
			name: "hot: min_amps",
			old: func() *Config {
				cfg := newConfig()
				cfg.Charging.MinAmps = 6
				return cfg
			}(),
			new: func() *Config {
				cfg := newConfig()
				cfg.Charging.MinAmps = 8
				return cfg
			}(),
			wantClass: ApplyHot,
			wantFields: []string{"charging.min_amps"},
			wantWarn:  false,
		},
		{
			name: "hot: max_amps",
			old: func() *Config {
				cfg := newConfig()
				cfg.Charging.MaxAmps = 32
				return cfg
			}(),
			new: func() *Config {
				cfg := newConfig()
				cfg.Charging.MaxAmps = 20
				return cfg
			}(),
			wantClass: ApplyHot,
			wantFields: []string{"charging.max_amps"},
			wantWarn:  false,
		},
		{
			name: "hot: contactor_cooldown_sec",
			old: func() *Config {
				cfg := newConfig()
				cfg.Charging.ContactorCooldownSec = 180
				return cfg
			}(),
			new: func() *Config {
				cfg := newConfig()
				cfg.Charging.ContactorCooldownSec = 240
				return cfg
			}(),
			wantClass: ApplyHot,
			wantFields: []string{"charging.contactor_cooldown_sec"},
			wantWarn:  false,
		},
		{
			name: "hot: default_amps",
			old: func() *Config {
				cfg := newConfig()
				cfg.Charging.DefaultAmps = 6
				return cfg
			}(),
			new: func() *Config {
				cfg := newConfig()
				cfg.Charging.DefaultAmps = 8
				return cfg
			}(),
			wantClass: ApplyHot,
			wantFields: []string{"charging.default_amps"},
			wantWarn:  false,
		},
		{
			name: "hot: webui.token",
			old: func() *Config {
				cfg := newConfig()
				cfg.WebUI.Token = ""
				return cfg
			}(),
			new: func() *Config {
				cfg := newConfig()
				cfg.WebUI.Token = "secret-token"
				return cfg
			}(),
			wantClass: ApplyHot,
			wantFields: []string{"webui.token"},
			wantWarn:  false,
		},
		{
			name: "rebuild: ocpp_port",
			old: func() *Config {
				cfg := newConfig()
				cfg.Server.OCPPPort = 8887
				return cfg
			}(),
			new: func() *Config {
				cfg := newConfig()
				cfg.Server.OCPPPort = 9000
				return cfg
			}(),
			wantClass: ApplyRebuild,
			wantFields: []string{"server.ocpp_port"},
			wantWarn:  false,
		},
		{
			name: "rebuild: ocpp_path",
			old: func() *Config {
				cfg := newConfig()
				cfg.Server.OCPPPath = "/{ws}"
				return cfg
			}(),
			new: func() *Config {
				cfg := newConfig()
				cfg.Server.OCPPPath = "/ws"
				return cfg
			}(),
			wantClass: ApplyRebuild,
			wantFields: []string{"server.ocpp_path"},
			wantWarn:  true,
		},
		{
			name: "rebuild: broker",
			old: func() *Config {
				cfg := newConfig()
				cfg.MQTT.Broker = "tcp://localhost:1883"
				return cfg
			}(),
			new: func() *Config {
				cfg := newConfig()
				cfg.MQTT.Broker = "tcp://broker.local:1883"
				return cfg
			}(),
			wantClass: ApplyRebuild,
			wantFields: []string{"mqtt.broker"},
			wantWarn:  false,
		},
		{
			name: "rebuild: client_id",
			old: func() *Config {
				cfg := newConfig()
				cfg.MQTT.ClientID = "panya-charge"
				return cfg
			}(),
			new: func() *Config {
				cfg := newConfig()
				cfg.MQTT.ClientID = "custom-id"
				return cfg
			}(),
			wantClass: ApplyRebuild,
			wantFields: []string{"mqtt.client_id"},
			wantWarn:  false,
		},
		{
			name: "rebuild: username",
			old: func() *Config {
				cfg := newConfig()
				cfg.MQTT.Username = ""
				return cfg
			}(),
			new: func() *Config {
				cfg := newConfig()
				cfg.MQTT.Username = "myuser"
				return cfg
			}(),
			wantClass: ApplyRebuild,
			wantFields: []string{"mqtt.username"},
			wantWarn:  false,
		},
		{
			name: "rebuild: password",
			old: func() *Config {
				cfg := newConfig()
				cfg.MQTT.Password = ""
				return cfg
			}(),
			new: func() *Config {
				cfg := newConfig()
				cfg.MQTT.Password = "supersecret"
				return cfg
			}(),
			wantClass: ApplyRebuild,
			wantFields: []string{"mqtt.password"},
			wantWarn:  false,
		},
		{
			name: "rebuild: base_topic",
			old: func() *Config {
				cfg := newConfig()
				cfg.MQTT.BaseTopic = "panya"
				return cfg
			}(),
			new: func() *Config {
				cfg := newConfig()
				cfg.MQTT.BaseTopic = "custom"
				return cfg
			}(),
			wantClass: ApplyRebuild,
			wantFields: []string{"mqtt.base_topic"},
			wantWarn:  false,
		},
		{
			name: "rebuild: log_format",
			old: func() *Config {
				cfg := newConfig()
				cfg.Server.LogFormat = "text"
				return cfg
			}(),
			new: func() *Config {
				cfg := newConfig()
				cfg.Server.LogFormat = "json"
				return cfg
			}(),
			wantClass: ApplyRebuild,
			wantFields: []string{"server.log_format"},
			wantWarn:  false,
		},
		{
			name: "rebuild: topics.charge_status",
			old: func() *Config {
				cfg := newConfig()
				return cfg
			}(),
			new: func() *Config {
				cfg := newConfig()
				cfg.MQTT.Topics["charge_status"] = "custom/charge/status"
				return cfg
			}(),
			wantClass: ApplyRebuild,
			wantFields: []string{"mqtt.topics.*"},
			wantWarn:  false,
		},
		{
			name: "rebuild: disconnect_threshold_sec",
			old: func() *Config {
				cfg := newConfig()
				cfg.MQTT.DisconnectThresholdSec = 60
				return cfg
			}(),
			new: func() *Config {
				cfg := newConfig()
				cfg.MQTT.DisconnectThresholdSec = 120
				return cfg
			}(),
			wantClass: ApplyRebuild,
			wantFields: []string{"mqtt.disconnect_threshold_sec"},
			wantWarn:  false,
		},
		{
			name: "process_restart: webui.enabled",
			old: func() *Config {
				cfg := newConfig()
				cfg.WebUI.Enabled = false
				return cfg
			}(),
			new: func() *Config {
				cfg := newConfig()
				cfg.WebUI.Enabled = true
				return cfg
			}(),
			wantClass: ApplyProcessRestart,
			wantFields: []string{"webui.enabled"},
			wantWarn:  false,
		},
		{
			name: "process_restart: webui.listen",
			old: func() *Config {
				cfg := newConfig()
				cfg.WebUI.Listen = "127.0.0.1:8888"
				return cfg
			}(),
			new: func() *Config {
				cfg := newConfig()
				cfg.WebUI.Listen = "0.0.0.0:8888"
				return cfg
			}(),
			wantClass: ApplyProcessRestart,
			wantFields: []string{"webui.listen"},
			wantWarn:  false,
		},
		{
			name: "multi_field_highest_wins",
			old: func() *Config {
				cfg := newConfig()
				return cfg
			}(),
			new: func() *Config {
				cfg := newConfig()
				cfg.Server.LogLevel = "debug"
				cfg.WebUI.Enabled = true
				return cfg
			}(),
			wantClass: ApplyProcessRestart,
			wantFields: []string{"server.log_level", "webui.enabled"},
			wantWarn:  false,
		},
		{
			name: "multi_field_rebuild_wins_over_hot",
			old: func() *Config {
				cfg := newConfig()
				cfg.Charging.MinAmps = 6
				return cfg
			}(),
			new: func() *Config {
				cfg := newConfig()
				cfg.Charging.MinAmps = 8
				cfg.MQTT.Broker = "tcp://broker.local:1883"
				return cfg
			}(),
			wantClass: ApplyRebuild,
			wantFields: []string{"charging.min_amps", "mqtt.broker"},
			wantWarn:  false,
		},
		{
			name: "rebuild_with_hot_and_none",
			old: func() *Config {
				cfg := newConfig()
				return cfg
			}(),
			new: func() *Config {
				cfg := newConfig()
				cfg.Server.LogLevel = "debug"
				cfg.MQTT.Broker = "tcp://broker.local:1883"
				return cfg
			}(),
			wantClass: ApplyRebuild,
			wantFields: []string{"server.log_level", "mqtt.broker"},
			wantWarn:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := ClassifyChanges(tt.old, tt.new)
			if report.Class != tt.wantClass {
				t.Errorf("ClassifyChanges(): got class %v, want %v", report.Class, tt.wantClass)
			}
			if !equals(report.Fields, tt.wantFields) {
				t.Errorf("Fields mismatch:\ngot:  %v\nwant: %v", report.Fields, tt.wantFields)
			}
			if report.ChargerReconfigureRequired != tt.wantWarn {
				t.Errorf("ChargerReconfigureRequired: got %v, want %v", report.ChargerReconfigureRequired, tt.wantWarn)
			}
		})
	}

	// Verify all leaf fields are covered by tests
	wantFields := map[string]bool{
		"server.ocpp_port":      true,
		"server.ocpp_path":      true,
		"server.log_level":      true,
		"server.log_format":     true,
		"mqtt.broker":           true,
		"mqtt.client_id":        true,
		"mqtt.username":         true,
		"mqtt.password":         true,
		"mqtt.base_topic":       true,
		"mqtt.topics.*":         true,
		"mqtt.disconnect_threshold_sec": true,
		"charging.min_amps":     true,
		"charging.max_amps":     true,
		"charging.contactor_cooldown_sec": true,
		"charging.default_amps": true,
		"webui.enabled":         true,
		"webui.listen":          true,
		"webui.token":           true,
	}
	covered := make(map[string]bool)
	for _, tt := range tests {
		for _, f := range tt.wantFields {
			covered[f] = true
		}
	}
	for field := range wantFields {
		if !covered[field] {
			t.Errorf("All fields not covered by tests: %s", field)
		}
	}
}

func newConfig() *Config {
	return &Config{
		Server: ServerConfig{},
		MQTT: MQTTConfig{
			Topics: map[string]string{
				"charge_status":           "charge/status",
				"charge_power":            "charge/power",
				"charge_energy":           "charge/energy",
				"grid_power":              "grid/power",
				"solar_power":             "",
				"consumption_power":       "",
				"command_set_amps":        "charge/command/set_amps",
				"command_state":           "charge/command/state",
				"smart_charging_command":  "smart_charging/command",
			},
		},
		Charging: ChargingConfig{},
		WebUI:  WebUIConfig{},
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equals(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	aCopy := make([]string, len(a))
	bCopy := make([]string, len(b))
	for i := range a {
		aCopy[i] = a[i]
		bCopy[i] = b[i]
	}
	slices.Sort(aCopy)
	slices.Sort(bCopy)
	return equal(aCopy, bCopy)
}
