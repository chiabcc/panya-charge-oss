#!/bin/bash
set -euo pipefail

export PANYA_WEBUI_ENABLED=false
export PANYA_WEBUI_STATUS_ENABLED=true

OPTIONS="/data/options.json"
SUPERVISOR_TOKEN="${SUPERVISOR_TOKEN:-}"

opt() {
    jq -r "$1 // empty" "$OPTIONS"
}

export PANYA_MQTT_BASE_TOPIC="$(opt '.base_topic')"
export PANYA_MQTT_CLIENT_ID="$(opt '.client_id')"
export PANYA_SERVER_LOG_LEVEL="$(opt '.log_level')"
export PANYA_SERVER_LOG_FORMAT="$(opt '.log_format')"
export PANYA_CHARGING_MIN_AMPS="$(opt '.min_amps')"
export PANYA_CHARGING_MAX_AMPS="$(opt '.max_amps')"
export PANYA_CHARGING_DEFAULT_AMPS="$(opt '.default_amps')"
export PANYA_CHARGING_CONTACTOR_COOLDOWN_SEC="$(opt '.contactor_cooldown_sec')"
export PANYA_MQTT_DISCONNECT_THRESHOLD_SEC="$(opt '.disconnect_threshold_sec')"
export PANYA_MQTT_TOPIC_GRID_POWER="$(opt '.grid_power_topic')"
export PANYA_MQTT_TOPIC_SOLAR_POWER="$(opt '.solar_power_topic')"
export PANYA_MQTT_TOPIC_CONSUMPTION_POWER="$(opt '.consumption_power_topic')"

MQTT_HOST="$(curl -sf -H "Authorization: Bearer ${SUPERVISOR_TOKEN}" http://supervisor/core/api/services/mqtt | jq -r '.host // empty')"
MQTT_USER="$(curl -sf -H "Authorization: Bearer ${SUPERVISOR_TOKEN}" http://supervisor/core/api/services/mqtt | jq -r '.username // empty')"
MQTT_PASSWORD="$(curl -sf -H "Authorization: Bearer ${SUPERVISOR_TOKEN}" http://supervisor/core/api/services/mqtt | jq -r '.password // empty')"

if [[ -z "${MQTT_HOST}" ]]; then
    echo "ERROR: MQTT service not available via Supervisor API"
    exit 1
fi

export PANYA_MQTT_BROKER="tcp://${MQTT_HOST}:1883"

if [[ -n "${MQTT_USER}" ]]; then
    export PANYA_MQTT_USERNAME="${MQTT_USER}"
    export PANYA_MQTT_PASSWORD="${MQTT_PASSWORD}"
fi

exec /usr/local/bin/panya-charge-oss -config ""
