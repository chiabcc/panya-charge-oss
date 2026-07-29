#!/bin/bash
set -euo pipefail

export PANYA_WEBUI_ENABLED=false
export PANYA_WEBUI_STATUS_ENABLED=true
export PANYA_WEBUI_LISTEN=0.0.0.0:8888

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

MQTT_BROKER_OPT="$(opt '.mqtt_broker')"

if [[ -n "${MQTT_BROKER_OPT}" ]]; then
    echo "[panya] using manual MQTT broker: ${MQTT_BROKER_OPT}"
    export PANYA_MQTT_BROKER="${MQTT_BROKER_OPT}"
    MQTT_USER_OPT="$(opt '.mqtt_username')"
    MQTT_PASS_OPT="$(opt '.mqtt_password')"
    if [[ -n "${MQTT_USER_OPT}" ]]; then
        export PANYA_MQTT_USERNAME="${MQTT_USER_OPT}"
        export PANYA_MQTT_PASSWORD="${MQTT_PASS_OPT}"
    fi
else
    echo "[panya] discovering MQTT via Supervisor Services API..."
    SVC_BODY="$(mktemp)"
    SVC_STATUS="$(curl -s -o "${SVC_BODY}" -w "%{http_code}" \
        -H "Authorization: Bearer ${SUPERVISOR_TOKEN}" \
        http://supervisor/services/mqtt)" || {
        echo "ERROR: cannot connect to Supervisor (curl failed)."
        echo "Is this running under HA Supervisor? If not, set mqtt_broker manually."
        rm -f "${SVC_BODY}"
        exit 1
    }

    if [[ "${SVC_STATUS}" != "200" ]]; then
        echo "ERROR: Supervisor /services/mqtt returned HTTP ${SVC_STATUS}"
        echo "Response body: $(cat "${SVC_BODY}")"
        case "${SVC_STATUS}" in
            404) echo "Hint: MQTT service not registered. Install and start the" \
                       "'Mosquitto broker' add-on, OR set mqtt_broker manually." ;;
            403) echo "Hint: access denied. Ensure Mosquitto broker is running." ;;
        esac
        rm -f "${SVC_BODY}"
        exit 1
    fi

    SERVICES_JSON="$(cat "${SVC_BODY}")"
    rm -f "${SVC_BODY}"

    RESULT="$(printf '%s' "${SERVICES_JSON}" | jq -r '.result // empty')"
    if [[ "${RESULT}" != "ok" ]]; then
        MESSAGE="$(printf '%s' "${SERVICES_JSON}" | jq -r '.message // "unknown error"')"
        echo "ERROR: MQTT service error: ${MESSAGE}"
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
    echo "[panya] discovered MQTT broker: ${PANYA_MQTT_BROKER}"

    if [[ -n "${MQTT_USER}" ]]; then
        export PANYA_MQTT_USERNAME="${MQTT_USER}"
        export PANYA_MQTT_PASSWORD="${MQTT_PASSWORD}"
    fi
fi

# Energy entity IDs (native HA entity reader)
export PANYA_ENERGY_HASS_GRID_ENTITY_ID="${grid_entity_id:-}"
export PANYA_ENERGY_HASS_SOLAR_ENTITY_ID="${solar_entity_id:-}"
export PANYA_ENERGY_HASS_CONSUMPTION_ENTITY_ID="${consumption_entity_id:-}"

# Supervisor token for HA API calls
export PANYA_HASS_TOKEN="${SUPERVISOR_TOKEN:-}"

exec /usr/local/bin/panya-charge-oss -config ""
