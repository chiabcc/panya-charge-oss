# Changelog

## 0.1.15

- Show real HA IP in OCPP URL on status page (auto-detected via browser)
- Remove `{ws}` wildcard from OCPP URL display

## 0.1.14

- Fix ingress 404 caused by `ingress_entry: /status` creating a double-slash URL

## 0.1.13

- Keep process alive when MQTT broker is unreachable
- Status page now shows live MQTT connection state (Connected/Disconnected)
- HA discovery messages auto-republish when MQTT reconnects
- Safety systems unchanged (stale timeout, 6A fallback, contactor cooldown)

## 0.1.12

- Serve status page directly at `GET /` (no redirect) to fix ingress 404
- Hide Config nav link in status-only mode

## 0.1.11

- Prefix ingress path on `/` to `/status` redirect

## 0.1.10

- Redirect `GET /` to `/status` in status-only mode

## 0.1.9

- Bind status page to `0.0.0.0:8888` for HA ingress proxy access (fixes 502)

## 0.1.8

- Diagnostic MQTT errors: print HTTP status code and response body on failure
- Added optional `mqtt_broker` / `mqtt_username` / `mqtt_password` fields for manual broker config
- Added clear error messages when MQTT service is unavailable

## 0.1.7

- Fix wrong Supervisor API endpoint (`/core/api/services/mqtt` to `/services/mqtt`)
- Correctly parse Supervisor envelope-wrapped response `{result, data}`

## 0.1.6

- Switch from HA base image to Alpine to eliminate `s6-overlay-suexec` error

## 0.1.5

- Drop bashio dependency, use plain bash + jq + Supervisor REST API

## 0.1.4

- Change run.sh shebang to `#!/usr/bin/env bashio`

## 0.1.3

- Per-arch GHCR images (`{arch}-panya-charge-oss`)
- Add `{arch}` placeholder to image field

## 0.1.2

- Add valid AppArmor profile

## 0.1.1

- Remove broken AppArmor profile

## 0.1.0

- Initial HA add-on release
- OCPP 1.6-J WebSocket server
- MQTT bridge with HA discovery
- Smart charging with solar surplus optimization
- Read-only status page via HA ingress
