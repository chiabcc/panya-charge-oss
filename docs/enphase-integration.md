# Enphase Envoy Integration Guide

How to wire Enphase solar (via the Home Assistant Enphase Envoy integration)
into `panya-charge-oss` for solar surplus smart charging.

This is a companion to the [Home Assistant Integration Guide](home-assistant.md).
It only covers the Enphase-specific bridging — follow the HA guide first for
base setup (MQTT broker, charger wiring, discovery).

---

## Native HA Entity Reader (Add-on Only)

**If you are running the Panya Charge OSS add-on, you do not need any MQTT
bridge automations.** The add-on reads Enphase entity states directly from
Home Assistant's Services API and publishes them to the internal smart charging
controller without going through the MQTT broker.

To enable this mode in the add-on Configuration tab:

1. Set `solar_power_topic` to your Enphase production entity ID
   (e.g. `sensor.enphase_envoy_current_power_production`)
2. Set `consumption_power_topic` to your Enphase consumption entity ID
   (e.g. `sensor.enphase_envoy_home_power_consumption`)
3. (Optional) Set `grid_power_topic` to your grid power entity ID or leave
   empty to compute it from solar minus consumption

The add-on polls these entities automatically and feeds the values into the
smart charging calculator. No automations, no MQTT publishing, no extra
configuration needed.

> **Entity IDs vary** by Envoy model and firmware. Open
> **Settings → Devices & Services → Enphase Envoy** in HA to find your actual
> entity IDs. Common names include:
> - `sensor.envoy_<serial>_current_power_production`
> - `sensor.envoy_<serial>_home_power_consumption`
> - `sensor.enphase_envoy_current_power_production`

---

## Legacy: MQTT Automation Bridge (Deprecated)

> **This approach is deprecated.** If you use the HA add-on, use the native
> entity reader above instead. This section is preserved for users who run
> panya-charge-oss in standalone mode or need a custom setup.

### How the Data Flows (Legacy)

```
Enphase Envoy ──local API──→ HA Enphase integration ──→ HA sensor
                                                          │
                                                  (bridge via automation)
                                                          ▼
                                              MQTT topic panya subscribes to
                                                          │
                                                          ▼
                                                panya-charge-oss smart charging
```

`panya-charge-oss` never talks to the Envoy directly. The Envoy exposes a local
REST/WebSocket API; HA's [Enphase Envoy integration](https://www.home-assistant.io/integrations/enphase_envoy/)
polls it and exposes sensors. You bridge those sensors onto the MQTT topics
panya subscribes to.

---

## What panya Subscribes To (Standalone Mode)

> **Add-on users:** skip this table. Configure entity IDs directly in the
> add-on's Configuration tab using the native entity reader described above.

| Config key | Default topic | Required? | Payload |
|---|---|---|---|
| `mqtt.topics.grid_power` | `grid/power` | Yes | Watts, signed: **+ = importing, − = exporting** |
| `mqtt.topics.solar_power` | _(empty, disabled)_ | Optional | Watts, ≥ 0 |
| `mqtt.topics.consumption_power` | _(empty, disabled)_ | Optional | Watts, ≥ 0 (house load, excluding EV) |

**Payload format** — panya accepts either:

- Plain number: `1234.5`
- JSON object: `{"power": 1234.5}`

All topics are prefixed with `mqtt.base_topic` (default `panya`), so the full
path is e.g. `panya/grid/power`.

### Unit Handling (Watts vs Kilowatts)

panya expects **watts** in the MQTT payload. Most HA energy integrations report in watts
by default, but some use kilowatts. Check your entity's `unit_of_measurement` attribute in
**Developer Tools → States**:

- **`unit_of_measurement: W`** — pass through directly:
  ```yaml
  payload: "{{ states('sensor.your_entity') | float(0) | round(0) }}"
  ```
- **`unit_of_measurement: kW`** — multiply by 1000:
  ```yaml
  payload: "{{ (states('sensor.your_entity') | float(0) * 1000) | round(0) }}"
  ```

When in doubt, publish the value and check panya logs: `DEBUG solar power updated watts=X`
— if `X` is in the hundreds/thousands range, it's correct. If it's in the single-digit
hundreds, you're likely dividing or multiplying by the wrong factor.

---

## Strategies at a Glance

The smart charging calculator picks a mode automatically based on which inputs
are available:

| Inputs available | Surplus formula | When to use |
|---|---|---|
| `solar_power` + `consumption_power` (both > 0) | `solar − consumption` | **Recommended** — most accurate, direct from Envoy |
| `grid_power` only | `−grid` | If you have a dedicated grid CT clamp |
| All three | `solar − consumption` primary, `grid` used for cross-validation drift check | Best — catches sensor drift |

The cross-validation check (`internal/domain/smartcharging/calculator.go`) warns
when `|solar − consumption + grid| > 500 W`, indicating sensor drift.

---

## Option A: Solar + Consumption (Standalone Mode)

> **Add-on users:** skip this section. Configure your entity IDs in the native
> entity reader instead.

Uses Envoy's two most reliable readings directly. Works for any Envoy model
that exposes production and consumption sensors (Envoy-S metered, IQ Gateway).

### Config

```yaml
mqtt:
  broker: "tcp://192.168.1.100:1883"
  base_topic: "panya"
  topics:
    grid_power: "grid/power"                  # required (cross-validation)
    solar_power: "solar/power"                # Envoy production
    consumption_power: "home/power"           # Envoy consumption
```

#### HA Add-on Configuration

If you're running panya as a Home Assistant add-on, configure these fields in the
add-on's Configuration tab instead of `config.yaml`:

| Add-on Field | Value |
|---|---|
| `grid_power_topic` | `grid/power` |
| `solar_power_topic` | `solar/power` |
| `consumption_power_topic` | `home/power` |

The add-on translates these to the same MQTT topics. The `base_topic` (default `panya`)
is still configured in the add-on settings.

### HA Automations

Create two automations to republish Envoy sensors onto panya's topics.

```yaml
# Republish Envoy solar production
- alias: "Bridge Envoy solar to panya"
  trigger:
    - platform: state
      entity_id: sensor.enphase_envoy_current_power_production
  action:
    - service: mqtt.publish
      data:
        topic: "panya/solar/power"
        payload: "{{ trigger.to_state.state | float(0) }}"

# Republish Envoy home consumption
- alias: "Bridge Envoy consumption to panya"
  trigger:
    - platform: state
      entity_id: sensor.enphase_envoy_home_power_consumption
  action:
    - service: mqtt.publish
      data:
        topic: "panya/home/power"
        payload: "{{ trigger.to_state.state | float(0) }}"
```

> **`float(0)` guard**: The `| float(0)` filter prevents silent failures when an
> entity goes "unavailable" or "unknown". Without it, panya's `parsePowerPayload`
> silently rejects the string "unavailable" — the app sees stale data and falls to
> safe state (6A). With `float(0)`, it publishes `0` instead, which is explicitly
> handled as "no surplus" — the charger falls to minimum but doesn't see stale data.

> **Entity IDs vary** by Envoy model and firmware. Open
> **Settings → Devices & Services → Enphase Envoy** in HA to find your actual
> entity IDs. Common names include:
> - `sensor.envoy_<serial>_current_power_production`
> - `sensor.envoy_<serial>_home_power_consumption`
> - `sensor.enphase_envoy_current_power_production`

#### Time-Based Trigger (Alternative)

The above automations use a `state` trigger — they fire whenever an entity value changes.
This works well for Enphase (entities update every 30-60s). If you prefer a predictable
cadence, or if your sensor updates at a different rate, use a `time_pattern` trigger:

```yaml
- alias: "Bridge Enphase energy to panya (time-based)"
  trigger:
    - platform: time_pattern
      seconds: "/15"
  action:
    - service: mqtt.publish
      data:
        topic: "panya/solar/power"
        payload: "{{ states('sensor.enphase_envoy_current_power_production') | float(0) | round(0) }}"
        retain: true
    - service: mqtt.publish
      data:
        topic: "panya/home/power"
        payload: "{{ states('sensor.enphase_envoy_home_power_consumption') | float(0) | round(0) }}"
        retain: true
```

This publishes all energy values every 15 seconds — between panya's 10-second poll
interval and its 60-second staleness threshold. One automation handles everything.

### Grid Power Source

For `grid_power` (used for cross-validation), pick one:

1. **Compute in HA** (if no dedicated grid meter):
   ```yaml
   - alias: "Compute grid power for panya"
     trigger:
       - platform: state
         entity_id: sensor.enphase_envoy_home_power_consumption
       - platform: state
         entity_id: sensor.enphase_envoy_current_power_production
     action:
       - service: mqtt.publish
         data:
            topic: "panya/grid/power"
            payload: >
              {{ (states('sensor.enphase_envoy_home_power_consumption') | float(0)
                  - states('sensor.enphase_envoy_current_power_production') | float(0))
                  | round(1) }}
    ```
   - Positive = importing, negative = exporting — matches panya's convention.

2. **Leave empty** — panya skips cross-validation and uses solar−consumption
   directly. Works fine, you just lose the drift check.

### MQTT Retain

Set `retain: true` in your `mqtt.publish` actions. This ensures panya sees the
last-known energy value immediately on reconnect (e.g., after a broker restart or panya
restart), so the controller doesn't see stale data during the gap. The 60-second
staleness threshold still applies — if the automation stops publishing, panya falls to
safe state regardless.

Add to your automation actions:
```yaml
data:
  topic: "panya/solar/power"
  payload: "{{ ... }}"
  retain: true  # ← add this
```

---

## Option B: Grid-Only (CT Clamps at Grid Point) — Legacy

> **Add-on users:** skip this section. The native entity reader supports a
> single grid power entity; set `grid_power_topic` and leave the others empty.

If your Envoy has CT clamps installed at the main service panel (common on
Envoy-S Metered), the grid reading alone is sufficient. Simpler setup, one
sensor.

### Config

```yaml
mqtt:
  broker: "tcp://192.168.1.100:1883"
  base_topic: "panya"
  topics:
    grid_power: "grid/power"
    # Leave solar_power and consumption_power empty
```

#### HA Add-on Configuration

If you're running panya as a Home Assistant add-on, configure this field in the
add-on's Configuration tab instead of `config.yaml`:

| Add-on Field | Value |
|---|---|
| `grid_power_topic` | `grid/power` |

Leave `solar_power_topic` and `consumption_power_topic` empty to match Option B.
The `base_topic` (default `panya`) is still configured in the add-on settings.

### HA Automation

```yaml
- alias: "Bridge Envoy grid power to panya"
  trigger:
    - platform: state
      entity_id: sensor.enphase_envoy_grid_power
  action:
    - service: mqtt.publish
      data:
        topic: "panya/grid/power"
        payload: "{{ trigger.to_state.state | float(0) }}"
```

### Sign Convention Check (Critical)

Envoy's "Grid Power" entity sign varies by firmware:

- Some firmwares: **+ = importing** (matches panya's convention — no transform needed)
- Other firmwares: **+ = exporting** (inverted — must flip the sign)

**Verify with debug logs.** Start panya with `log_level: debug`. During solar
hours (midday, sunny), you should see:

```
DEBUG grid power updated watts=-2300    ← correct (exporting surplus)
```

If you see positive watts during surplus, flip the sign in the automation:

```yaml
payload: "{{ (trigger.to_state.state | float(0) * -1) | round(1) }}"
```

If your Enphase setup exposes a "Current net power consumption" entity (rather than a
dedicated "Grid Power" entity), note that some Enphase firmware versions expose this as
an always-positive value (import only) with a separate export entity. To verify:

**During midday surplus** (solar producing more than home is consuming):
- If "Current net power consumption" shows a **NEGATIVE** value (e.g., `-1.2`) → correct, proceed
- If it shows `0` or a **positive** value → your integration doesn't support signed net power
- If you see a separate "net energy production" entity → use Option A instead (solar + consumption)

---

## Option C: Point panya at Existing HA MQTT Topics (Standalone Only)

If you already publish Envoy data to MQTT (via Node-RED, AppDaemon, or another
integration), skip the bridge automation entirely and point panya at those
topics. **Add-on users should use the native entity reader instead.**

### Config

```yaml
mqtt:
  broker: "tcp://192.168.1.100:1883"
  base_topic: "panya"
  topics:
    grid_power: "homeassistant/sensor/envoy_123456/grid_power/state"
    solar_power: "homeassistant/sensor/envoy_123456/production/state"
    consumption_power: "homeassistant/sensor/envoy_123456/consumption/state"
```

### Notes

- These topics are relative to `base_topic`, so the full path panya subscribes
  to would be `panya/homeassistant/sensor/...` — likely not what you want.
- For cross-namespace topics, use a leading `/` workaround or — cleaner —
  restructure your config so the energy topics live under the same base topic
  as panya.
- **Payload must match** what your existing publisher sends. panya accepts
  plain numbers or `{"power": N}` JSON. Other JSON schemas won't parse.

---

## Option D: Direct Envoy Local API (Advanced)

Bypass HA entirely — poll the Envoy's local API and publish straight to MQTT
using a small script. Useful if:

- You don't run HA
- You want lower latency than HA's polling interval
- You want to avoid the HA middleware

### Minimal Go Example

```go
// Poll Envoy production every 5s, publish to MQTT.
// Requires Envoy local API token (from Enphase installer portal).
package main

import (
    "encoding/json"
    "fmt"
    "net/http"
    "time"

    mqtt "github.com/eclipse/paho.mqtt.golang"
)

type production struct {
    Watts int `json:"wNow"`
}

func main() {
    opts := mqtt.NewClientOptions().AddBroker("tcp://localhost:1883")
    client := mqtt.NewClient(opts)
    client.Connect()

    token := "<your-envoy-token>"
    url := "http://192.168.1.50/production.json"

    for range time.Tick(5 * time.Second) {
        req, _ := http.NewRequest("GET", url, nil)
        req.Header.Set("Authorization", "Bearer "+token)
        resp, err := http.DefaultClient.Do(req)
        if err != nil {
            continue
        }
        var prod struct{ Production []struct{ MeasurementType string; Watts int } }
        json.NewDecoder(resp.Body).Decode(&prod)
        resp.Body.Close()

        for _, p := range prod.Production {
            if p.MeasurementType == "production" {
                client.Publish("panya/solar/power", 1, false,
                    fmt.Sprintf("%d", p.Watts))
            }
        }
    }
}
```

**Caveats:**

- Envoy local API requires a token from the Enphase installer portal (7-day
  or 1-year tokens). Token refresh is your responsibility.
- API endpoint and response schema vary by Envoy firmware. Test with `curl`
  first.
- You still need a consumption reading for Option A. The Envoy consumption
  endpoint (`/consumption.json` or `/ivp/meters/readings`) is only available
  on metered Envoys.

---

## Sign Conventions Reference

From `internal/domain/smartcharging/types.go`:

| Field | Sign | Range |
|---|---|---|
| `GridPowerW` | + = importing, − = exporting | any |
| `SolarPowerW` | Always positive | ≥ 0 |
| `ConsumptionPowerW` | Always positive (house load, excludes EV) | ≥ 0 |

**Surplus** (what the controller optimizes for):

- `surplus > 0` → solar exceeds house load → ramp charging up
- `surplus < 0` → drawing from grid → ramp charging down
- `surplus < min_amps × 230V` → `ShouldStop = true`

---

## Algorithm at a Glance

From `internal/domain/smartcharging/calculator.go`:

```
surplus = solar − consumption          (or −grid if solar/consumption missing)
availableW = max(surplus, 0)
idealAmps = floor(availableW / 230)    # 230V single-phase EU/Thailand
clamp to [min_amps=6, max_amps=32]
if |idealAmps − prev| < 2A  → hold previous (hysteresis — prevents chatter)
if idealAmps < 6A           → ShouldStop=true
```

- **230 V assumed** (single-phase). 22 kW @ 32 A = 7.4 kW/phase — matches ABB Terra AC W22.
- **2 A hysteresis** prevents contactor wear from small solar fluctuations.
- **Stale data > 60 s** (`mqtt.disconnect_threshold_sec`) → falls back to `default_amps` (6 A).

---

## Configuration Reference

```yaml
mqtt:
  broker: "tcp://192.168.1.100:1883"
  base_topic: "panya"
  topics:
    grid_power: "grid/power"                  # always subscribed
    solar_power: "solar/power"                # optional (empty = disabled)
    consumption_power: "home/power"           # optional (empty = disabled)

charging:
  min_amps: 6                # IEC 61851 minimum — do not lower
  max_amps: 32               # Type 2 single-phase max
  contactor_cooldown_sec: 180   # DO NOT LOWER — physical contactor protection
  default_amps: 6            # fallback when data goes stale
```

---

## Verification

1. Start panya-charge-oss with `log_level: debug`.
2. Watch for these log lines:
   ```
   DEBUG grid power updated watts=-2300       # negative = exporting surplus ✓
   DEBUG solar power updated watts=4200       # matches Envoy dashboard ✓
   DEBUG consumption power updated watts=1800 # matches Envoy dashboard ✓
   ```
3. The smart-charging controller logs `reason="solar surplus adjusted (solar-consumption)"`
   — confirms Option A is active. If you see `(grid fallback)`, your
   solar/consumption topics aren't reaching panya.

---

## Troubleshooting

### Charging always pinned at 6 A

| Check | How |
|-------|-----|
| Topics flowing | Debug logs should show `grid/solar/consumption power updated` every few seconds |
| Payload format | If logs show "ignoring non-numeric payload", the automation is sending wrong format |
| Entity IDs | Verify automations reference real entities (HA → Settings → Devices & Services → Enphase) |
| Sign convention | Negative grid watts during sunny midday = correct |

### Charging ramps up when importing from grid

Grid sign is flipped. Multiply by `-1` in your automation:

```yaml
payload: "{{ (trigger.to_state.state | float(0) * -1) | round(1) }}"
```

### Charging current jumps around rapidly

Solar production is noisy (clouds, inverter quantization). panya has a 2 A
hysteresis band built in; if that's not enough, smooth the source sensor in HA:

```yaml
# Use a time-simple moving average
template:
  - sensor:
      - name: "Envoy Solar Smoothed"
        state: "{{ states('sensor.enphase_envoy_current_power_production') | float }}"
        unit_of_measurement: "W"
        # Apply exponential smoothing via a trigger-based template
```

Or use the
[`filter` integration](https://www.home-assistant.io/integrations/filter/)
with `time_simple_moving_average` over a 60-second window.

### `solar surplus adjusted (grid fallback)` in logs

Your `solar_power` or `consumption_power` topics are empty or zero. Either:

- The HA automation isn't firing (check HA automation traces)
- The config keys are misspelled
- The Envoy entity ID changed after a firmware update

### Cross-validation drift warnings

When all three inputs are configured and `|solar − consumption + grid| > 500 W`,
the controller logs a drift warning. Causes:

- One sensor is stale (Envoy updated production but not consumption yet)
- CT clamp orientation flipped (sign issue on grid or consumption)
- Time skew between independent polling sources

Not fatal — controller falls back to solar−consumption. But investigate if it
persists.

---

## Further Reading

- [Home Assistant Integration Guide](home-assistant.md) — base setup
- [HA Enphase Envoy integration](https://www.home-assistant.io/integrations/enphase_envoy/)
- [Enphase Envoy Local API](https://enphase.com/download/accessing-iq-gateway-local-data)
- [OCPP compatibility notes](ocpp-compatibility.md) — charger-specific quirks
- [Development guide](development.md) — architecture, testing
