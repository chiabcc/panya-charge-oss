# Installing as a Home Assistant Add-on

Connect any OCPP 1.6-J compatible EV charger to Home Assistant using the
Panya Charge OSS add-on. It bridges the charger's OCPP WebSocket into MQTT
topics that Home Assistant discovers automatically, no custom components,
no HACS repositories.

---

## Prerequisites

| Requirement | Details |
|-------------|---------|
| **Home Assistant** | Supervised install (HAOS, HA Container on Docker, or OS on VM/container) with the Supervisor panel |
| **MQTT** | Home Assistant's built-in MQTT integration, configured and showing no errors |
| **Charger** | OCPP 1.6-J compatible. Target hardware: ABB Terra AC W22-G5-R-0 with firmware ≥ 1.8.32 |
| **Network** | Charger must reach the Home Assistant host on TCP port 8887 |

### Setting up MQTT in Home Assistant

If you don't already have MQTT set up:

1. Go to **Settings → Add-ons → Add-on Store**
2. Install **Mosquitto broker**, start it, and enable **Start on boot**
3. Go to **Settings → Devices & Services → Add Integration → MQTT**
4. Enter your HA login credentials (the Mosquitto add-on uses HA authentication)
5. Verify the connection shows no errors

---

## Adding the Repository

1. In Home Assistant, go to **Settings → Add-ons → Add-on Store**
2. Click the **⋮** (three dots) menu in the top-right corner
3. Select **Repositories**
4. Paste the repository URL and click **Add**:

   ```
   https://github.com/chiabcc/panya-charge-oss
   ```

5. The **Panya Charge OSS** add-on will now appear in your store

---

## Installing the Add-on

1. Click **Panya Charge OSS** from the add-on store
2. Click **Install**
3. Wait for installation to complete

> **Note:** You'll see `services: mqtt: need` in the status area. This is normal. The add-on won't start until MQTT is configured. MQTT must be set up *before* starting the add-on.

---

## Configuring the Add-on

The configuration is split across four groups in the add-on's **Configuration** tab.

### MQTT Settings

| Field | Default | Description |
|-------|---------|-------------|
| `base_topic` | `panya` | Top-level MQTT topic prefix. Must be unique if running multiple CSMS instances on the same broker |
| `client_id` | `panya-charge` | MQTT client identifier |

> **Tip:** If you run multiple CSMS instances against the same broker, use different base topics (e.g. `panya-garage`, `panya-driveway`).

### Logging

| Field | Default | Description |
|-------|---------|-------------|
| `log_level` | `info` | Log verbosity: `debug`, `info`, `warn`, or `error` |
| `log_format` | `text` | Output format: `text` or `json` |

Set to `debug` when troubleshooting connection issues.

### Charging Parameters

| Field | Default | Description |
|-------|---------|-------------|
| `min_amps` | `6` | Minimum charging current. Per IEC 61851, must be ≥ 6A |
| `max_amps` | `32` | Maximum charging current. Per Type 2 / 22 kW single-phase limit |
| `default_amps` | `6` | Current applied when the charger first connects or when smart charging falls back |
| `contactor_cooldown_sec` | `180` | Minimum seconds between start/stop commands to protect the charger contactor |

### MQTT Topics (Smart Charging)

These topics feed power data into the smart charging controller:

| Field | Default | Description |
|-------|---------|-------------|
| `grid_power_topic` | `grid/power` | Topic the CSMS subscribes to for grid power data. Positive = importing from grid, negative = exporting (surplus) |
| `solar_power_topic` | _(empty)_ | Optional: solar production topic. When set with `consumption_power_topic`, the CSMS calculates surplus as solar minus consumption (more accurate than grid alone) |
| `consumption_power_topic` | _(empty)_ | Optional: whole-home consumption topic |

Set a topic to empty to disable it. At minimum, set `grid_power_topic` to
point at your meter or Home Assistant energy sensor's MQTT topic.

### Advanced

| Field | Default | Description |
|-------|---------|-------------|
| `disconnect_threshold_sec` | `60` | Seconds without MQTT data before falling back to safe state (`min_amps`) |

### Applying Configuration

After editing, click **Save** at the bottom of the Configuration tab. The
add-on restarts automatically, which takes approximately 10 seconds.

---

## Pointing Your Charger at the CSMS

In your charger's web interface or configuration menu, set the OCPP backend
URL:

```
ws://<HA-IP>:8887/{ws}
```

**Use the Home Assistant host's LAN IP address, not the add-on DNS name
(like `core-panya_charge_oss`). Chargers cannot resolve internal DNS names.**
If you're unsure of the IP, check **Settings → System → Network** in Home
Assistant.

For **ABB Terra AC** chargers:

1. Navigate to **Settings → Backend → OCPP**
2. Set **OCPP URL** to the address above
3. Set the **Charge Point ID** to a unique identifier (usually the serial
   number). This ID appears in all MQTT topics under `charge/<id>/...`

---

## Verifying Everything Works

### 1. Start the Add-on

Click **Start** on the add-on page. You should see `Started` in the status
area within 10 seconds.

### 2. Watch the Logs

Go to the **Logs** tab in the add-on page. On startup, you should see:

```
INFO mqtt subscriber connected broker=tcp://<host>:1883
INFO ocpp server listening port=8887
INFO csms started
```

If you don't see `mqtt subscriber connected`, the add-on can't reach the MQTT
broker. Double-check your MQTT integration in Home Assistant.

### 3. Confirm the Charger Connected

When the charger boots (or after you save a new OCPP URL), it sends a
`BootNotification`. Look for:

```
INFO boot notification charger_id=<id> model=<model> firmware=<version>
INFO mqtt discovery published charger_id=<id>
```

If nothing appears after 30 seconds:

- Confirm the charger's OCPP URL exactly matches `ws://<HA-IP>:8887/{ws}`
- Check that port 8887 is reachable from the charger's network
- Set `log_level` to `debug` and restart the add-on

### 4. Check Home Assistant Entities

Once `BootNotification` fires, Home Assistant auto-discovers the charger's
entities within a few seconds:

1. Go to **Settings → Devices & Services → MQTT**
2. You should see a device matching your charger (named by manufacturer and
   model)
3. Click into it to see the entities listed below

See [docs/home-assistant.md](home-assistant.md) for the full entity reference.

### 5. Open the Status Page

Click **Open Web UI** on the add-on page. The status page loads via HA ingress,
showing:

- **OCPP URL** — the WebSocket address to enter in your charger's settings
- **MQTT Connection** — connected/disconnected badge + broker address
- **Chargers** — table with ID, model, status, connector, power, current limit
- **Smart Charging** — enabled/disabled, safe amps, grid/solar/consumption readings

The page auto-refreshes every 10 seconds.

### 6. Test Control

- Toggle the **Charging** switch in the HA UI to start/stop a session
- Adjust the **Charging Current** slider to change the current limit

---

## Behavior Notes

### Config Save = Restart

Saving configuration restarts the add-on. This creates approximately a 30-second
gap where the charger is disconnected. If you're in the middle of a charging
session, plan your config changes accordingly.

### MQTT Disconnect Safe Fallback

If the CSMS loses its MQTT connection for more than
`disconnect_threshold_sec` seconds (default 60), it falls back to
`min_amps` (6A) as a safe state. Charging resumes at the previous level once
MQTT data flow returns.

### Multiple Chargers

The same CSMS instance accepts connections from multiple chargers simultaneously.
Each charger's data flows under its own ID in the MQTT topics:

```
<base_topic>/charge/<charger-id>/status
<base_topic>/charge/<charger-id>/power
<base_topic>/charge/<charger-id>/energy
```

---

## Troubleshooting

### Add-on won't start

**Symptom:** Clicking Start does nothing, or logs show `services: mqtt: need`

**Cause:** The MQTT service isn't available.

**Fix:**

1. Confirm the MQTT integration is configured in Home Assistant (**Settings →
   Devices & Services**. Look for "MQTT" with no red error banner.
2. Restart the Mosquitto broker add-on if it's not running
3. Try starting the Panya Charge add-on again

### Charger can't connect to CSMS

**Symptom:** No `boot notification` in the add-on logs after charging the
charger's OCPP URL.

**Checklist:**

- The URL must use the **HA host's LAN IP** (e.g. `192.168.1.100`), not
  `core-panya_charge_oss` or `homeassistant.local`. Chargers can't resolve
  internal DNS names.
- The URL must use `ws://` (not `http://` or `wss://` unless you've set up TLS)
- The path must be exactly `/{ws}` (case-sensitive)
- Port 8887 must be reachable from the charger's network (no firewall/VLAN
  blocking)

Test connectivity from a terminal on the same network as the charger:

```
telnet <HA-IP> 8887
```

### Configuration changes aren't applying

**Cause:** The add-on needs to restart after saving.

**Fix:**

1. Make sure you clicked **Save** (not just closing the tab)
2. Wait ~10 seconds for the restart to complete
3. Check the Logs tab. The add-on should show fresh startup messages.
4. Verify the effective config by checking the log output after restart

### Entities don't appear in Home Assistant

**Checklist:**

- MQTT integration is active with no errors
- Discovery topic is being listened to: in the MQTT integration settings,
  check that it's subscribing to `<base_topic>/homeassistant/#`
- Discovery payloads are retained. Restarting Home Assistant re-triggers discovery.
- Set `log_level` to `debug` and look for `mqtt discovery published`

### Smart charging isn't adjusting current

**Checklist:**

- `grid_power_topic` is set and your meter publishes to that topic
- Values are flowing (positive for import, negative for export)
- If no data arrives within `disconnect_threshold_sec` seconds, the controller
  falls back to `min_amps` (6A)
- After a start/stop command, the `contactor_cooldown_sec` lockout prevents
  further adjustments for 180 seconds

### `SetChargingProfile` rejected by charger

This is handled automatically by the CSMS. If you still see rejections:

- The CSMS uses `TxDefaultProfile` (not `ChargePointMaxProfile`)
- For ABB Terra AC: it uses stack level 1 to work around a known firmware bug
- Relative 0-100 kind, `startPeriod = 0`, and limit > 0 are all applied
  automatically

---

## Switching from Standalone

If you were running panya-charge-oss as a standalone binary, migrate your
`config.yaml` values to the add-on UI fields:

| config.yaml key | Add-on field |
|-----------------|-------------|
| `server.ocpp_port` | _Fixed at 8887 in add-on; no field_ |
| `server.log_level` | `log_level` |
| `server.log_format` | `log_format` |
| `mqtt.broker` | _Auto-populated from HA Services API; no field_ |
| `mqtt.client_id` | `client_id` |
| `mqtt.base_topic` | `base_topic` |
| `mqtt.username` | _Auto-populated from HA Services API; no field_ |
| `mqtt.password` | _Auto-populated from HA Services API; no field_ |
| `charging.min_amps` | `min_amps` |
| `charging.max_amps` | `max_amps` |
| `charging.default_amps` | `default_amps` |
| `charging.contactor_cooldown_sec` | `contactor_cooldown_sec` |
| `mqtt.topics.grid_power` | `grid_power_topic` |
| `mqtt.topics.solar_power` | `solar_power_topic` |
| `mqtt.topics.consumption_power` | `consumption_power_topic` |
| `mqtt.disconnect_threshold_sec` | `disconnect_threshold_sec` |

The `mqtt.broker`, `mqtt.username`, and `mqtt.password` are fetched
automatically from Home Assistant's Services API. You don't need to enter
them. The `ocpp_port` is fixed at 8887 and can't be changed.

The add-on's WebUI is disabled by default and not exposed in the add-on
environment. If you were relying on the WebUI, continue using the standalone
binary or access the charger directly.

---

## Further Reading

- [Home Assistant Integration Guide](home-assistant.md) - entity reference,
  energy dashboard wiring, manual MQTT control
- [Smart Charging with HA Sensors](home-assistant.md#smart-charging-with-ha-sensors)
  wiring grid, solar, and consumption sensors
- [Config WebUI](webui.md) - optional web interface (standalone mode only)
- [ABB Terra AC Hardware Notes](hardware/abb-terra-ac.md) - charger-specific
  quirks and firmware details
- [OCPP Compatibility Notes](ocpp-compatibility.md) - charger-specific
  behaviors and known issues
