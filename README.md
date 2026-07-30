# panya-charge-oss

[![License: Apache 2.0](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.25-00ADD8.svg)](https://go.dev)
[![OCPP](https://img.shields.io/badge/OCPP-1.6--J-green.svg)](https://openchargealliance.org/)

> ⚠️ **Alpha — simulator-validated only.** This CSMS has been tested against an
> OCPP 1.6-J simulator. Real hardware validation (ABB Terra AC) is in progress.
> Use at your own risk. See [Target Hardware](docs/hardware/abb-terra-ac.md)
> for details.

Open-source OCPP 1.6-J protocol bridge for EV chargers. Connects any OCPP 1.6-J compatible charger to Home Assistant, enabling smart charging with solar surplus optimization.

## What It Does

- **OCPP 1.6-J CSMS** — WebSocket server for EV charger connectivity
- **Home Assistant** — Auto-discovers charger entities via MQTT Discovery
- **Smart Charging** — Adjusts charging current based on solar surplus, reading grid/solar/consumption entities directly via the HA Supervisor API
- **Safety** — 180s contactor cooldown, 6A fallback on data staleness

## Install

Install from the Home Assistant Add-on Store:

1. Go to **Settings → Add-ons → Add-on Store → ⋮ → Repositories**
2. Add: `https://github.com/chiabcc/panya-charge-oss`
3. Click **Panya Charge OSS** → **Install**
4. Configure charging parameters and energy entity IDs in the add-on UI
5. Set your charger's OCPP URL to `ws://<HA-IP>:8887/{ws}`

See **[Install Guide](docs/add-on-install.md)** for full details and troubleshooting.

## Status Page

The add-on serves a read-only status page via HA ingress — click **Open Web UI** on the add-on page. Shows runtime information: OCPP URL, MQTT connection state, connected chargers, smart charging readings, and configured energy entity IDs.

## Smart Charging Setup

In the add-on Configuration tab, set the energy entity IDs for your solar/grid/consumption sensors:

- **grid_entity_id** — e.g. `sensor.grid_power` (negative = surplus/exporting)
- **solar_entity_id** — e.g. `sensor.enphase_envoy_current_power_production`
- **consumption_entity_id** — e.g. `sensor.home_power_consumption`

Smart charging activates when grid power goes negative (surplus). If no entity IDs are configured, smart charging is disabled gracefully. See **[Enphase Integration Guide](docs/enphase-integration.md)** for details.

## For Developers

To embed the CSMS in your own Go application, see
[docs/development.md](docs/development.md).

## License

Apache 2.0 — see [LICENSE](LICENSE)
