# Home Assistant Integration Guide

This guide walks through connecting an OCPP 1.6-J compatible EV charger to
Home Assistant via the `panya-charge-oss` add-on.

## Architecture

```
Charger ──OCPP 1.6-J WebSocket──→ Panya Charge OSS add-on ──MQTT──→ Home Assistant
                                          ↑                            
                               Supervisor API (entity polling)
                                          ↓
                              Grid/Solar/Consumption sensors
```

The add-on acts as a CSMS (Central System Management System). It speaks OCPP
to the charger and MQTT to Home Assistant. Charger entities are discovered
automatically via MQTT Discovery. Energy sensors are read directly via the
Supervisor API — no bridge automations needed.

---

## Prerequisites

| Requirement | Details |
|-------------|---------|
| Home Assistant | 2024.x or newer (MQTT Discovery support) |
| MQTT Broker | Mosquitto broker add-on (recommended) or external broker |
| OCPP 1.6-J Charger | Target: ABB Terra AC; any OCPP 1.6-J charger should work (simulator-validated, hardware testing pending) |
| Network | Charger must reach HA on port 8887 (TCP) |

### 1. Install the Add-on

See the **[Install Guide](add-on-install.md)** for step-by-step installation.

Quick version:
1. **Settings → Add-ons → Add-on Store → ⋮ → Repositories**
2. Add: `https://github.com/chiabcc/panya-charge-oss`
3. Install **Panya Charge OSS**
4. Set charging parameters and energy entity IDs in the Configuration tab
5. Start the add-on

The add-on auto-discovers the MQTT broker via the Supervisor API — no manual broker config needed.

### 2. Point Your Charger at the CSMS

In your charger's web interface or configuration menu, set the OCPP backend URL:

```
ws://<ha-ip>:8887/{ws}
```

- Use your Home Assistant host's LAN IP address
- The `/{ws}` path suffix is **required**
- Set a charger identity (usually the serial number) — this becomes `<id>` in all MQTT topics

For the **ABB Terra AC** specifically:
- Firmware ≥ 1.8.32 required for OCPP 1.6-J
- Menu: **Settings → Backend → OCPP URL**
- Set the charge point ID in the same menu

### 3. Verify the Charger Connected

Check the add-on logs (Configuration tab → Log). You should see:

```
INFO boot notification charger_id=ABB-001243 model=Terra-AC firmware=1.8.32
INFO mqtt discovery published charger_id=ABB-001243
```

### 4. Verify Entities in Home Assistant

Once `BootNotification` fires, the add-on publishes MQTT Discovery payloads.
Home Assistant picks these up within a few seconds.

1. Go to **Settings → Devices & Services → MQTT**
2. You should see a device matching your charger (named by manufacturer + model)
3. Click into it — you should see the entities listed below

If entities don't appear:
- Confirm MQTT is integrated in HA (**Settings → Devices & Services** — look for "MQTT")
- Check the discovery topic in the MQTT integration's "Listening to a topic" debugger:
  ```
  panya/homeassistant/#  (if base_topic is "panya")
  ```
- The CSMS publishes discovery payloads as **retained** messages, so restarting HA will re-trigger discovery

---

## Entities

The charger appears as a single **device** in Home Assistant, with the
following entities grouped under it.

### Primary Entities (device dashboard)

| Entity | Type | Unit | Description |
|--------|------|------|-------------|
| Status | sensor | — | Connector status: `Available`, `Preparing`, `Charging`, `SuspendedEV`, `SuspendedEVSE`, `Finishing`, `Faulted` |
| Charging Power | sensor | kW | Instantaneous charging power (3 decimals) |
| Session Energy | sensor | kWh | Cumulative charging energy — Energy Dashboard compatible |
| Grid Power | sensor | W | Grid net power (negative = exporting to grid) |
| Solar Power | sensor | W | Solar production *(only if `solar_entity_id` configured)* |
| Home Consumption | sensor | W | Whole-home consumption *(only if `consumption_entity_id` configured)* |
| Proxy Connected | binary_sensor | — | Upstream relay active *(only when proxy relay enabled)* |

### Configuration Entities (under "Configuration" section)

| Entity | Type | Range | Description |
|--------|------|-------|-------------|
| Charging Current | number | `min_amps` – `max_amps` | Slider to set the active current limit |
| Charging | switch | — | Start / stop the charging session |

All entities use the `charge/<id>/online` topic as availability — HA marks
the device as unavailable when the charger disconnects from the CSMS.

---

## Energy Dashboard

The **Session Energy** sensor has `state_class: total_increasing`, making it
compatible with HA's Energy Dashboard.

To add your charger's energy consumption to the dashboard:

1. Go to **Settings → Energy**
2. Under **Electricity grid**, click **Add consumption**
3. Select the charger's **Session Energy** sensor
4. (Optional) Under **Solar production**, add your solar production sensor for net calculation

This gives you per-charger energy usage alongside your home's overall consumption.

---

## Smart Charging Setup

`panya-charge-oss` adjusts charging current based on power surplus. The add-on
reads energy entities directly via the Supervisor API every 10 seconds — no
MQTT bridge automations needed.

### Configuration

In the add-on Configuration tab, set these fields:

| Field | What to enter |
|-------|---------------|
| `solar_entity_id` | Your solar production sensor (e.g. `sensor.enphase_envoy_current_power_production`) |
| `consumption_entity_id` | Your whole-home consumption sensor (e.g. `sensor.enphase_envoy_home_power_consumption`) |
| `grid_entity_id` | Your grid power sensor (e.g. `sensor.grid_power`). Positive = importing, negative = exporting |

### Strategy Selection

| What you have | What to set |
|---|---|
| Solar + consumption sensors | `solar_entity_id` + `consumption_entity_id` (recommended — most accurate) |
| Grid power sensor only | `grid_entity_id` (simplest) |
| All three | Set all three — controller cross-validates |

When solar + consumption are both set, the controller calculates surplus as
`solar − consumption`. If only grid is set, it uses the grid sign directly.

### Common Entity IDs

Verify your entity IDs in **Developer Tools → States**. Common patterns:

| Integration | Solar Sensor | Consumption Sensor | Grid Sensor |
|---|---|---|---|
| Enphase Envoy | `sensor.enphase_envoy_current_power_production` | `sensor.enphase_envoy_home_power_consumption` | `sensor.enphase_envoy_grid_power` |
| SolarEdge | `sensor.solaredge_production_pwr` | *(none)* | `sensor.solaredge_net_energy_meter_watts` |
| P1 Meter (DSMR) | *(none)* | *(none)* | `sensor.utility_meter_grid_power` |
| Shelly 3EM | `sensor.shelly_em_xxx_channel_0_power` | *(varies)* | *(varies)* |
| Fronius | `sensor.fronius_smart_meter_power_active_phase_1` | *(none)* | `sensor.fronius_smart_meter_power` |

> **Note**: Entity names vary by configuration and HA version. Check your actual
> entities in HA Developer Tools.

See the [Enphase Envoy Integration Guide](enphase-integration.md) for a detailed walkthrough.

### How the Controller Responds

- **Surplus available** → increases charging current (up to `max_amps`)
- **Drawing from grid** → decreases current (down to `min_amps`)
- **No fresh data for 60s** → falls back to `min_amps` (6A) as a safe state
- **Start/stop commands** → 180-second contactor cooldown enforced to prevent hardware damage

---

## Vendor-Specific Guides

- [Enphase Envoy Integration](enphase-integration.md) — Enphase solar setup for smart charging

---

## Manual Control

You can control the charger manually via the HA UI entities, or directly
via MQTT topics.

### From the HA UI

- Toggle **Charging** switch to start/stop
- Move the **Charging Current** slider to change the active current limit

### From MQTT

```bash
# Set current limit to 16A for all chargers
mosquitto_pub -h <broker> -t "panya/charge/command/set_amps" -m "16"

# Start charging
mosquitto_pub -h <broker> -t "panya/charge/command/state" -m "start"

# Stop charging
mosquitto_pub -h <broker> -t "panya/charge/command/state" -m "stop"

# Control a specific charger (by ID)
mosquitto_pub -h <broker> -t "panya/charge/ABB-001243/command/set_amps" -m "10"
mosquitto_pub -h <broker> -t "panya/charge/ABB-001243/command/state" -m "start"
```

---

## Example Dashboard Card

Here's a YAML dashboard card for your charger. Replace `ABB-001243` with your
charger ID and entity IDs as shown in HA.

```yaml
type: vertical-stack
title: EV Charger
cards:
  - type: glance
    entities:
      - entity: sensor.abb_001243_status
        name: Status
      - entity: binary_sensor.abb_001243_online
        name: Online
  - type: entities
    entities:
      - entity: sensor.abb_001243_charging_power
        name: Power
      - entity: sensor.abb_001243_session_energy
        name: Session Energy
      - type: divider
      - entity: number.abb_001243_charging_current
        name: Current Limit
      - entity: switch.abb_001243_charging
        name: Charging
```

---

## Troubleshooting

### Charger doesn't connect to CSMS

| Check | How |
|-------|-----|
| OCPP URL format | Must be `ws://<ip>:8887/{ws}` — the `/{ws}` suffix is required |
| Port reachable | `telnet <csms-ip> 8887` from the charger's network |
| CSMS logs | Set `log_level: debug` and look for WebSocket handshake errors |
| Charger firmware | ABB Terra AC needs ≥ 1.8.32 for OCPP 1.6-J |

### Entities don't appear in HA

| Check | How |
|-------|-----|
| MQTT integration active | **Settings → Devices & Services** — look for "MQTT" with no errors |
| Broker connectivity | CSMS logs should show `mqtt subscriber connected` |
| Discovery topic | Use the MQTT integration's "Listen to topic" tool on `panya/homeassistant/#` |
| Retained messages | Discovery payloads are retained — restart HA to re-trigger discovery |

### Smart charging isn't adjusting

| Check | How |
|-------|-----|
| Grid data flowing | Monitor `panya/grid/power` — should update every few seconds |
| Data staleness | If no grid data for 60s, controller reverts to 6A (safe state) |
| Contactor cooldown | After start/stop, 180-second lockout prevents further commands |
| Log level | Set to `debug` to see controller decisions in logs |

### `SetChargingProfile` rejected by charger

- Use `TxDefaultProfile` (not `ChargePointMaxProfile`) — the CSMS handles this automatically
- For ABB Terra AC: use stack level 1 (max-1, working around a known firmware bug)
- Relative 0-100 kind, startPeriod = 0, limit > 0 — all handled by the CSMS

---

## Further Reading

- [MQTT Discovery specification](https://www.home-assistant.io/integrations/mqtt/#mqtt-discovery)
- [HA Energy Dashboard](https://www.home-assistant.io/docs/energy/)
- [OCPP 1.6-J specification](https://openchargealliance.org/downloads/)
- [Development guide](development.md) — architecture, testing, debugging
- [OCPP compatibility notes](ocpp-compatibility.md) — charger-specific quirks
