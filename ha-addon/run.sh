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

SERVICES_JSON="$(curl -sf -H "Authorization: Bearer ${SUPERVISOR_TOKEN}" http://supervisor/services/mqtt)" || {
    echo "ERROR: cannot reach Supervisor services API"
    exit 1
}

RESULT="$(printf '%s' "${SERVICES_JSON}" | jq -r '.result // empty')"
if [[ "${RESULT}" != "ok" ]]; then
    MESSAGE="$(printf '%s' "${SERVICES_JSON}" | jq -r '.message // "unknown error"')"
    echo "ERROR: MQTT service not available via Supervisor API: ${MESSAGE}"
    echo "Hint: install and start the Mosquitto broker add-on, then restart this add-on."
    exit 1
fi

MQTT_HOST="$(printf '%s' "${SERVICES_JSON}" | jq -r '.data.host // empty')"
MQTT_PORT="$(printf '%s' "${SERVICES_JSON}" | jq -r '.data.port // 1883')"
MQTT_SSL="$(printf '%s' "${SERVICES_JSON}" | jq -r '.data.ssl // false')"
MQTT_USER="$(printf '%s' "${SERVICES_JSON}" | jq -r '.data.username // empty')"
MQTT_PASSWORD="$(printf '%s' "${SERVICES_JSON}" | jq -r '.data.password // empty')"

if [[ -z "${MQTT_HOST}" ]]; then
    echo "ERROR: MQTT service response missing host"
    exit 1
fi

SCHEME="tcp"
if [[ "${MQTT_SSL}" == "true" ]]; then
    SCHEME="ssl"
fi
export PANYA_MQTT_BROKER="${SCHEME}://${MQTT_HOST}:${MQTT_PORT}"

if [[ -n "${MQTT_USER}" ]]; then
    export PANYA_MQTT_USERNAME="${MQTT_USER}"
    export PANYA_MQTT_PASSWORD="${MQTT_PASSWORD}"
fi

exec /usr/local/bin/panya-charge-oss -config ""
