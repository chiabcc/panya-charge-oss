# Enphase Envoy Integration Guide

How to wire Enphase solar (via the Home Assistant Enphase Envoy integration)
into `panya-charge-oss` for solar surplus smart charging.

This is a companion to the [Home Assistant Integration Guide](home-assistant.md).
It only covers the Enphase-specific bridging — follow the HA guide first for
base setup (MQTT broker, charger wiring, discovery).

---

## How the Data Flows

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

## What panya Subscribes To

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

## Option A: Solar + Consumption (Recommended)

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
        payload: "{{ trigger.to_state.state }}"

# Republish Envoy home consumption
- alias: "Bridge Envoy consumption to panya"
  trigger:
    - platform: state
      entity_id: sensor.enphase_envoy_home_power_consumption
  action:
    - service: mqtt.publish
      data:
        topic: "panya/home/power"
        payload: "{{ trigger.to_state.state }}"
```

> **Entity IDs vary** by Envoy model and firmware. Open
> **Settings → Devices & Services → Enphase Envoy** in HA to find your actual
> entity IDs. Common names include:
> - `sensor.envoy_<serial>_current_power_production`
> - `sensor.envoy_<serial>_home_power_consumption`
> - `sensor.enphase_envoy_current_power_production`

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
             {{ (states('sensor.enphase_envoy_home_power_consumption') | float
                 - states('sensor.enphase_envoy_current_power_production') | float)
                 | round(1) }}
   ```
   - Positive = importing, negative = exporting — matches panya's convention.

2. **Leave empty** — panya skips cross-validation and uses solar−consumption
   directly. Works fine, you just lose the drift check.

---

## Option B: Grid-Only (CT Clamps at Grid Point)

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
        payload: "{{ trigger.to_state.state }}"
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
payload: "{{ (trigger.to_state.state | float * -1) | round(1) }}"
```

---

## Option C: Point panya at Existing HA MQTT Topics

If you already publish Envoy data to MQTT (via Node-RED, AppDaemon, or another
integration), skip the bridge automation entirely and point panya at those
topics.

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
payload: "{{ (trigger.to_state.state | float * -1) | round(1) }}"
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
