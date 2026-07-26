package config

type ApplyClass int

const (
	ApplyNone ApplyClass = iota
	ApplyHot
	ApplyRebuild
	ApplyProcessRestart
)

type ChangeReport struct {
	Class                      ApplyClass
	Fields                     []string
	ChargerReconfigureRequired bool
}

func ClassifyChanges(old, new *Config) ChangeReport {
	report := ChangeReport{
		Fields:                     []string{},
		ChargerReconfigureRequired: false,
		Class:                      ApplyNone,
	}

	if old.Server.OCPPPort != new.Server.OCPPPort {
		report.Fields = append(report.Fields, "server.ocpp_port")
		report.Class = max(report.Class, ApplyRebuild)
	}
	if old.Server.OCPPPath != new.Server.OCPPPath {
		report.Fields = append(report.Fields, "server.ocpp_path")
		report.ChargerReconfigureRequired = true
		report.Class = max(report.Class, ApplyRebuild)
	}
	if old.Server.LogLevel != new.Server.LogLevel {
		report.Fields = append(report.Fields, "server.log_level")
		report.Class = max(report.Class, ApplyHot)
	}
	if old.Server.LogFormat != new.Server.LogFormat {
		report.Fields = append(report.Fields, "server.log_format")
		report.Class = max(report.Class, ApplyRebuild)
	}
	if old.MQTT.Broker != new.MQTT.Broker {
		report.Fields = append(report.Fields, "mqtt.broker")
		report.Class = max(report.Class, ApplyRebuild)
	}
	if old.MQTT.ClientID != new.MQTT.ClientID {
		report.Fields = append(report.Fields, "mqtt.client_id")
		report.Class = max(report.Class, ApplyRebuild)
	}
	if old.MQTT.Username != new.MQTT.Username {
		report.Fields = append(report.Fields, "mqtt.username")
		report.Class = max(report.Class, ApplyRebuild)
	}
	if old.MQTT.Password != new.MQTT.Password {
		report.Fields = append(report.Fields, "mqtt.password")
		report.Class = max(report.Class, ApplyRebuild)
	}
	if old.MQTT.BaseTopic != new.MQTT.BaseTopic {
		report.Fields = append(report.Fields, "mqtt.base_topic")
		report.Class = max(report.Class, ApplyRebuild)
	}
	if !mapsEqual(old.MQTT.Topics, new.MQTT.Topics) {
		report.Fields = append(report.Fields, "mqtt.topics.*")
		report.Class = max(report.Class, ApplyRebuild)
	}
	if old.MQTT.DisconnectThresholdSec != new.MQTT.DisconnectThresholdSec {
		report.Fields = append(report.Fields, "mqtt.disconnect_threshold_sec")
		report.Class = max(report.Class, ApplyRebuild)
	}
	if old.Charging.MinAmps != new.Charging.MinAmps {
		report.Fields = append(report.Fields, "charging.min_amps")
		report.Class = max(report.Class, ApplyHot)
	}
	if old.Charging.MaxAmps != new.Charging.MaxAmps {
		report.Fields = append(report.Fields, "charging.max_amps")
		report.Class = max(report.Class, ApplyHot)
	}
	if old.Charging.ContactorCooldownSec != new.Charging.ContactorCooldownSec {
		report.Fields = append(report.Fields, "charging.contactor_cooldown_sec")
		report.Class = max(report.Class, ApplyHot)
	}
	if old.Charging.DefaultAmps != new.Charging.DefaultAmps {
		report.Fields = append(report.Fields, "charging.default_amps")
		report.Class = max(report.Class, ApplyHot)
	}
	if old.WebUI.Enabled != new.WebUI.Enabled {
		report.Fields = append(report.Fields, "webui.enabled")
		report.Class = max(report.Class, ApplyProcessRestart)
	}
	if old.WebUI.Listen != new.WebUI.Listen {
		report.Fields = append(report.Fields, "webui.listen")
		report.Class = max(report.Class, ApplyProcessRestart)
	}
	if old.WebUI.Token != new.WebUI.Token {
		report.Fields = append(report.Fields, "webui.token")
		report.Class = max(report.Class, ApplyHot)
	}

	return report
}

func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bb, ok := b[k]; !ok || bb != v {
			return false
		}
	}
	return true
}
