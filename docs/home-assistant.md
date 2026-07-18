# Home Assistant Integration Guide

This guide walks through connecting an OCPP 1.6-J compatible EV charger to
Home Assistant via `panya-charge-oss`.

## Architecture

```
Charger ──OCPP 1.6-J WebSocket──→ panya-charge-oss ──MQTT──→ Home Assistant
                                       ↑                          ↓
                                   MQTT broker ←── Grid/Solar sensors
```

`panya-charge-oss` acts as a CSMS (Central System Management System). It speaks
OCPP to the charger on one side and MQTT to Home Assistant on the other. No
add-on, HACS repository, or custom component is required — Home Assistant's
built-in [MQTT integration](https://www.home-assistant.io/integrations/mqtt/)
discovers the charger entities automatically.

---

## Prerequisites

| Requirement | Details |
|-------------|---------|
| Home Assistant | 2024.x or newer (MQTT Discovery support) |
| MQTT Broker | Mosquitto broker add-on (recommended) or external broker |
| OCPP 1.6-J Charger | Target: ABB Terra AC; any OCPP 1.6-J charger should work (simulator-validated, hardware testing pending) |
| Go 1.25+ | Only if building from source |
| Network | Charger must reach the CSMS host on port 8887 (TCP) |

### 1. Set Up the MQTT Broker

If you don't already have an MQTT broker in your Home Assistant setup:

1. In HA, go to **Settings → Add-ons → Add-on Store**
2. Install **Mosquitto broker**
3. Start the add-on and enable *Start on boot*
4. Go to **Settings → Devices & Services → Add Integration → MQTT**
5. Configure to connect to your local broker:
   - Broker: `core-mosquitto` (if using the add-on)
   - Port: `1883`
   - Username / Password: your HA account credentials (the Mosquitto add-on uses HA authentication)

Verify the connection works:

```bash
# From a machine with mosquitto_sub, or use the Mosquitto add-on's "Log Viewer"
mosquitto_sub -h <ha-ip> -p 1883 -u <user> -P <pass> -t "#" -C 1
```

If you see a message within a few seconds, MQTT is working.

### 2. Configure panya-charge-oss

Create `config.yaml` pointing at your HA MQTT broker:

```yaml
server:
  ocpp_port: 8887
  ocpp_path: "/{ws}"
  log_level: info

mqtt:
  broker: "tcp://192.168.1.100:1883"   # your HA / Mosquitto broker IP
  client_id: "panya-charge"
  username: "your-ha-user"              # omit if broker has no auth
  password: "your-ha-password"          # omit if broker has no auth
  base_topic: "panya"
  disconnect_threshold_sec: 60

charging:
  min_amps: 6              # IEC 61851 minimum
  max_amps: 32             # Type 2 / 22 kW single-phase max
  contactor_cooldown_sec: 180
  default_amps: 6
```

> **Important:** `base_topic` must be unique per CSMS instance. If you run
> multiple CSMS instances against the same broker, use different base topics
> (e.g. `panya-garage`, `panya-driveway`).

Start the CSMS:

```bash
go run ./cmd/panya-charge-oss -config config.yaml
```

You should see:

```
INFO mqtt subscriber connected broker=tcp://192.168.1.100:1883
INFO ocpp server listening port=8887
INFO csms started
```

### 3. Point Your Charger at the CSMS

In your charger's web interface or configuration menu, set the OCPP backend URL:

```
ws://<csms-host-ip>:8887/{ws}
```

- Use `ws://` for plaintext WebSocket on the local network
- Use `wss://` only if you've set up TLS termination (reverse proxy with certs)
- The `/{ws}` path suffix is **required** — the OCPP library uses it to identify the WebSocket upgrade endpoint
- Set a charger identity (usually the serial number or a custom ID) — this becomes `<id>` in all MQTT topics

For the **ABB Terra AC** specifically:
- Firmware ≥ 1.8.32 required for OCPP 1.6-J
- Menu: **Settings → Backend → OCPP URL**
- Set the charge point ID in the same menu

### 4. Verify the Charger Connected

The charger should send a `BootNotification` on startup. Watch the CSMS logs:

```
INFO boot notification charger_id=ABB-001243 model=Terra-AC firmware=1.8.32
INFO mqtt discovery published charger_id=ABB-001243
```

If you don't see anything within 30 seconds:
- Confirm the charger's OCPP URL exactly matches `ws://<ip>:8887/{ws}`
- Check that port 8887 is reachable from the charger (firewall, VLANs)
- Set `log_level: debug` in the config to see WebSocket connection attempts

### 5. Verify Entities in Home Assistant

Once `BootNotification` fires, the CSMS publishes MQTT Discovery payloads.
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
| Session Energy | sensor | kWh | Cumulative energy this session — Energy Dashboard compatible |
| Grid Power | sensor | W | Grid net power (negative = exporting to grid) |
| Solar Power | sensor | W | Solar production *(only if `solar_power` topic configured)* |
| Home Consumption | sensor | W | Whole-home consumption *(only if `consumption_power` topic configured)* |
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

## Smart Charging with HA Sensors

`panya-charge-oss` adjusts charging current based on power surplus. To enable
this, the CSMS needs to know your grid / solar / consumption power.

### Option A: Using HA's Grid Power (Simplest)

If you have a grid meter that publishes to MQTT (Shelly, EMP2410, etc.):

```yaml
mqtt:
  base_topic: "panya"
  topics:
    grid_power: "energy/grid/power"   # your grid meter's power topic
```

The CSMS subscribes to `panya/energy/grid/power` and treats positive values
as grid import (charging from grid), negative as export (surplus available).

### Option B: Using Solar + Consumption (More Accurate)

If you have separate solar and home consumption sensors:

```yaml
mqtt:
  base_topic: "panya"
  topics:
    grid_power: "energy/grid/power"
    solar_power: "energy/solar/power"         # optional
    consumption_power: "energy/home/power"    # optional
```

When both `solar_power` and `consumption_power` are available, the CSMS uses
`solar - consumption` for surplus calculation (more accurate than grid alone,
which has metering lag and rounding).

### Wiring HA Sensors to the CSMS Topics

Your grid/solar/consumption sensors likely publish to their own topics. You
have two options:

1. **Re-publish to the expected topic** — use an HA automation or Node-RED to forward:
   ```yaml
   # Example HA automation
   trigger:
     - platform: state
       entity_id: sensor.grid_power
   action:
     - service: mqtt.publish
       data:
         topic: "panya/energy/grid/power"
         payload: "{{ trigger.to_state.state }}"
   ```

2. **Point the CSMS at your existing topics** — change the `topics.grid_power` config to match your sensor's topic. The CSMS accepts both raw numeric payloads and JSON objects with a `power` field.

### How the Controller Responds

- **Surplus available** → increases charging current (up to `max_amps`)
- **Drawing from grid** → decreases current (down to `min_amps`)
- **No grid data for 60s** → falls back to `min_amps` (6A) as a safe state
- **Start/stop commands** → 180-second contactor cooldown enforced to prevent hardware damage

---

## Vendor-Specific Guides

- [Enphase Envoy Integration](enphase-integration.md) — bridging Enphase solar
  production and consumption sensors for solar surplus smart charging

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
