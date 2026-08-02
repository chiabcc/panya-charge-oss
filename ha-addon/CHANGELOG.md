# Changelog

## 0.2.5

### Bug Fixes

* **energy:** log when entity state is unavailable or unknown ([5a83970](https://github.com/chiabcc/panya-charge-oss/commit/5a839703281f1f884706d3d07fe8f6d72326a163))

## 0.2.4

- Fix smart charging still failing with 401 Unauthorized — earlier release used the wrong add-on manifest field (`homeassistant: true`, which is for pinning HA Core version, not for granting API access). Correct field is `homeassistant_api: true` which enables the HA REST API proxy at http://supervisor/core/api

## 0.2.3

### Bug Fixes

* **smart-charging:** correct Supervisor API base URL to avoid double /api/ ([2fc5d20](https://github.com/chiabcc/panya-charge-oss/commit/2fc5d2017ef9ec40568f26c1efdc13fed4d9ded8))

## 0.2.2

- Fix smart charging reading sensors failing with 401 Unauthorized — add-on now requests Home Assistant API access in its manifest so the Supervisor token can read entity states

## 0.2.1

- Fix smart charging never working — chargers stayed at minimum current (6A) with "energy data stale" warnings even when solar surplus was available
- Fix solar and consumption sensors reporting in kW (e.g. Enphase Envoy) being treated as watts — readings are now auto-converted so surplus calculations are correct

## 0.1.22

- Position as HA add-on only — remove standalone deployment from README and docs
- Fix icon and logo loading in Docker image
- Entity ID fields use plain text input in add-on config
- Restore external MQTT broker override fields
- Fix: disable smart charging controller when no energy entities configured (was forcing 6A every 10s)

## 0.1.19

- Entity selectors: pick grid/solar/consumption sensors from a dropdown instead of typing entity IDs
- Remove deprecated MQTT energy topic fields from add-on configuration
- Restore external MQTT broker override (mqtt_broker/username/password) for non-HA Mosquitto users
- Fix icon and logo not loading — copy image files into Docker image so sidebar shows the real icon

## 0.1.18

- Native HA entity reader: read solar/grid/consumption entities directly via Supervisor API
- MQTT energy input deprecated in favor of native HA entity reading

## 0.1.17

- Show MQTT input topics (grid/solar/consumption) on status page so users know where to publish sensor data

## 0.1.16

- Add brand icon (Panya Charge logo) for HA sidebar and add-on store
- Set maintainer: Chiab Code Code
- Add changelog visible in HA update dialog and Documentation tab
- Add green lightning favicon to status page

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
