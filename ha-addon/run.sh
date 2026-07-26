#!/usr/bin/with-contenv bashio
set -euo pipefail

# ---- Hard-disable WebUI in add-on context ----
export PANYA_WEBUI_ENABLED=false

# ---- Load user config from options.json (12 exports) ----
export PANYA_MQTT_BASE_TOPIC="$(bashio::config 'base_topic')"
export PANYA_MQTT_CLIENT_ID="$(bashio::config 'client_id')"
export PANYA_SERVER_LOG_LEVEL="$(bashio::config 'log_level')"
export PANYA_SERVER_LOG_FORMAT="$(bashio::config 'log_format')"
export PANYA_CHARGING_MIN_AMPS="$(bashio::config 'min_amps')"
export PANYA_CHARGING_MAX_AMPS="$(bashio::config 'max_amps')"
export PANYA_CHARGING_DEFAULT_AMPS="$(bashio::config 'default_amps')"
export PANYA_CHARGING_CONTACTOR_COOLDOWN_SEC="$(bashio::config 'contactor_cooldown_sec')"
export PANYA_MQTT_DISCONNECT_THRESHOLD_SEC="$(bashio::config 'disconnect_threshold_sec')"
export PANYA_MQTT_TOPIC_GRID_POWER="$(bashio::config 'grid_power_topic')"
export PANYA_MQTT_TOPIC_SOLAR_POWER="$(bashio::config 'solar_power_topic')"
export PANYA_MQTT_TOPIC_CONSUMPTION_POWER="$(bashio::config 'consumption_power_topic')"

# ---- Fetch MQTT credentials via dedicated Services API calls ----
MQTT_HOST="$(bashio::services mqtt 'host')"
MQTT_USER="$(bashio::services mqtt 'username')"
MQTT_PASSWORD="$(bashio::services mqtt 'password')"

# ---- Guard: MQTT host must be provided ----
if [[ -z "${MQTT_HOST}" ]]; then
    bashio::log.error "MQTT service not available via HA Services API"
    bashio::exit.nogood
fi

# ---- Broker URL (Mosquitto add-on internal port — Services API does not expose it) ----
export PANYA_MQTT_BROKER="tcp://${MQTT_HOST}:1883"

# ---- Export auth only when non-empty (empty = no auth in Go defaults) ----
if [[ -n "${MQTT_USER}" ]]; then
    export PANYA_MQTT_USERNAME="${MQTT_USER}"
    export PANYA_MQTT_PASSWORD="${MQTT_PASSWORD}"
fi

# ---- Exec binary with empty config path (env-var-only mode) ----
exec /usr/local/bin/panya-charge-oss -config ""