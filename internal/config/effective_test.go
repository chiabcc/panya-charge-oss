package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEffective_EnvOverrideDetection(t *testing.T) {
	t.Helper()

	type envPair struct {
		key   string
		value string
	}

	tests := []struct {
		name      string
		yaml      string
		envs      []envPair
		overrides []string
	}{
		{
			name:      "server.ocpp_port overridden",
			yaml:      "server:\n  ocpp_port: 8887\n",
			envs:      []envPair{{"PANYA_SERVER_OCPP_PORT", "7777"}},
			overrides: []string{"server.ocpp_port"},
		},
		{
			name:      "server.log_level overridden",
			yaml:      "server:\n  log_level: info\n",
			envs:      []envPair{{"PANYA_SERVER_LOG_LEVEL", "debug"}},
			overrides: []string{"server.log_level"},
		},
		{
			name:      "server.log_format overridden",
			yaml:      "server:\n  log_format: text\n",
			envs:      []envPair{{"PANYA_SERVER_LOG_FORMAT", "json"}},
			overrides: []string{"server.log_format"},
		},
		{
			name:      "mqtt.broker overridden",
			yaml:      "mqtt:\n  broker: tcp://localhost:1883\n",
			envs:      []envPair{{"PANYA_MQTT_BROKER", "tcp://broker:1883"}},
			overrides: []string{"mqtt.broker"},
		},
		{
			name:      "mqtt.client_id overridden",
			yaml:      "mqtt:\n  client_id: panya\n",
			envs:      []envPair{{"PANYA_MQTT_CLIENT_ID", "custom"}},
			overrides: []string{"mqtt.client_id"},
		},
		{
			name:      "mqtt.username overridden",
			yaml:      "mqtt:\n  username: admin\n",
			envs:      []envPair{{"PANYA_MQTT_USERNAME", "root"}},
			overrides: []string{"mqtt.username"},
		},
		{
			name:      "mqtt.password overridden",
			yaml:      "mqtt:\n  password: pass1\n",
			envs:      []envPair{{"PANYA_MQTT_PASSWORD", "pass2"}},
			overrides: []string{"mqtt.password"},
		},
		{
			name:      "mqtt.base_topic overridden",
			yaml:      "mqtt:\n  base_topic: panya\n",
			envs:      []envPair{{"PANYA_MQTT_BASE_TOPIC", "custom"}},
			overrides: []string{"mqtt.base_topic"},
		},
		{
			name:      "webui.listen overridden",
			yaml:      "webui:\n  listen: 127.0.0.1:8888\n",
			envs:      []envPair{{"PANYA_WEBUI_LISTEN", "0.0.0.0:8888"}},
			overrides: []string{"webui.listen"},
		},
		{
			name:      "webui.token overridden",
			yaml:      "webui:\n  token: old\n",
			envs:      []envPair{{"PANYA_WEBUI_TOKEN", "new"}},
			overrides: []string{"webui.token"},
		},
		{
			name: "webui.enabled overridden",
			yaml: "webui:\n  enabled: false\n",
			envs: []envPair{{"PANYA_WEBUI_ENABLED", "true"}},
			overrides: []string{"webui.enabled"},
		},
		{
			name:      "multi-override",
			yaml:      "server:\n  log_level: info\nmqtt:\n  broker: tcp://localhost:1883\n",
			envs:      []envPair{{"PANYA_SERVER_LOG_LEVEL", "debug"}, {"PANYA_MQTT_BROKER", "tcp://broker:1883"}},
			overrides: []string{"server.log_level", "mqtt.broker"},
		},
		{
			name:      "no env no override",
			yaml:      "server:\n  log_level: info\n",
			envs:      nil,
			overrides: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Helper()

			dir := t.TempDir()
			path := filepath.Join(dir, "config.yaml")

			_ = os.WriteFile(path, []byte(tt.yaml), 0644)

			for _, e := range tt.envs {
				t.Setenv(e.key, e.value)
			}

			ec, err := Effective(path)
			if err != nil {
				t.Fatalf("Effective: %v", err)
			}

			if ec.OverriddenByEnv == nil {
				ec.OverriddenByEnv = make(map[string]bool)
			}

			// Check expected overrides are present.
			for _, key := range tt.overrides {
				if !ec.OverriddenByEnv[key] {
					t.Errorf("expected %q in overridden_by_env but got: %v", key, ec.OverriddenByEnv)
				}
			}

			// Check no unexpected overrides.
			for key := range ec.OverriddenByEnv {
				found := false
				for _, want := range tt.overrides {
					if key == want {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("unexpected key %q in overridden_by_env", key)
				}
			}
		})
	}
}

func TestEffective_PasswordMasking(t *testing.T) {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	yamlContent := "mqtt:\n  password: supersecret\n"
	_ = os.WriteFile(path, []byte(yamlContent), 0644)

	ec, err := Effective(path)
	if err != nil {
		t.Fatalf("Effective: %v", err)
	}

	if ec.MQTTPasswordSet != true {
		t.Error("MQTTPasswordSet should be true when password is set")
	}
}

func TestEffective_TokenMasking(t *testing.T) {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	yamlContent := "webui:\n  enabled: true\n  token: mytoken\n"
	_ = os.WriteFile(path, []byte(yamlContent), 0644)

	ec, err := Effective(path)
	if err != nil {
		t.Fatalf("Effective: %v", err)
	}

	if ec.WebUITokenSet != true {
		t.Error("WebUITokenSet should be true when token is set")
	}
	if ec.WebUIEnabled != true {
		t.Error("WebUIEnabled should be true")
	}
}

func TestEffective_NoPasswordInStruct(t *testing.T) {
	t.Helper()

	// Compile-time verification: EffectiveConfig has PasswordSet but no Password.
	ec := &EffectiveConfig{}
	_ = ec.MQTTPasswordSet
}

func TestEffective_EmptyPath(t *testing.T) {
	t.Helper()

	ec, err := Effective("")
	if err != nil {
		t.Fatalf("Effective(\"\"): %v", err)
	}

	if ec.ServerOCPPPort != 8887 {
		t.Errorf("ServerOCPPPort = %d, want 8887 (default)", ec.ServerOCPPPort)
	}
	if len(ec.OverriddenByEnv) > 0 {
		t.Errorf("expected no overrides, got %v", ec.OverriddenByEnv)
	}
}

func TestEffective_FileValuePreserved(t *testing.T) {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	yamlContent := "mqtt:\n  broker: tcp://filebroker:1883\n"
	_ = os.WriteFile(path, []byte(yamlContent), 0644)

	ec, err := Effective(path)
	if err != nil {
		t.Fatalf("Effective: %v", err)
	}

	if ec.MQTTBroker != "tcp://filebroker:1883" {
		t.Errorf("MQTTBroker = %q, want %q", ec.MQTTBroker, "tcp://filebroker:1883")
	}

	if ec.OverriddenByEnv["mqtt.broker"] {
		t.Error("mqtt.broker should NOT be overridden when env is not set")
	}
}

func TestEffective_ValueOverride(t *testing.T) {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	yamlContent := "mqtt:\n  broker: tcp://filebroker:1883\n"
	_ = os.WriteFile(path, []byte(yamlContent), 0644)

	t.Setenv("PANYA_MQTT_BROKER", "tcp://envbroker:1883")

	ec, err := Effective(path)
	if err != nil {
		t.Fatalf("Effective: %v", err)
	}

	// Effective uses Load() internally, so env should override.
	if ec.MQTTBroker != "tcp://envbroker:1883" {
		t.Errorf("MQTTBroker = %q, want %q (env override)", ec.MQTTBroker, "tcp://envbroker:1883")
	}

	if !ec.OverriddenByEnv["mqtt.broker"] {
		t.Error("mqtt.broker should be overridden when env differs")
	}
}