# Enphase Envoy Integration Guide

How to wire Enphase solar (via the Home Assistant Enphase Envoy integration)
into `panya-charge-oss` for solar surplus smart charging.

---

## Setup

The add-on reads Enphase entity states directly from Home Assistant's
Supervisor API. No MQTT bridge automations needed — the add-on polls
energy entities every 10s and feeds them into the smart charging controller.

In the add-on Configuration tab, set these fields:

| Field | Entity ID example | Required? |
|-------|-------------------|-----------|
| `solar_entity_id` | `sensor.enphase_envoy_current_power_production` | Yes (for surplus mode) |
| `consumption_entity_id` | `sensor.enphase_envoy_home_power_consumption` | Yes (for surplus mode) |
| `grid_entity_id` | `sensor.grid_power` | Optional (fallback if solar/consumption unavailable) |

> **Entity IDs vary** by Envoy model and firmware. Open
> **Settings → Devices & Services → Enphase Envoy** in HA to find your actual
> entity IDs. Common names:
> - `sensor.envoy_<serial>_current_power_production`
> - `sensor.envoy_<serial>_home_power_consumption`
> - `sensor.enphase_envoy_current_power_production`

If no entity IDs are configured, smart charging is disabled gracefully.

---

## Sign Conventions

From `internal/domain/smartcharging/types.go`:

| Field | Sign | Range |
|-------|------|-------|
| Grid power | + = importing, − = exporting | any |
| Solar power | Always positive | ≥ 0 |
| Consumption power | Always positive (house load, excludes EV) | ≥ 0 |

**Surplus** (what the controller optimizes for):

- `surplus > 0` → solar exceeds house load → ramp charging up
- `surplus < 0` → drawing from grid → ramp charging down
- `surplus < min_amps × 230V` → stop charging

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
- **Stale data > 60 s** (`disconnect_threshold_sec`) → falls back to `default_amps` (6 A).

---

## Verification

1. Set `log_level: debug` in the add-on Configuration tab.
2. Watch for these log lines:
   ```
   DEBUG ha energy: ... entity_id=sensor.enphase_envoy_current_power_production
   ```
3. Check the status page (Open Web UI) — entity IDs should appear under Energy Sources.
4. When solar exceeds house consumption, charging current ramps up. When importing from grid, it ramps down to minimum.

---

## Troubleshooting

### Charging always pinned at 6 A

- Verify entity IDs are correct in the add-on Configuration tab
- Check the entity has a valid numeric state in HA (Developer Tools → States)
- Entity state should be `"unavailable"` or `"unknown"` → the adapter keeps last value, staleness kicks in after 60s

### Charging current jumps around rapidly

Solar production is noisy (clouds, inverter quantization). The controller has
a 2 A hysteresis band built in. If that's not enough, smooth the source sensor
in HA:

```yaml
# template sensor with moving average
template:
  - sensor:
      - name: "Envoy Solar Smoothed"
        state: "{{ states('sensor.enphase_envoy_current_power_production') | float }}"
        unit_of_measurement: "W"
```

Or use the
[`filter` integration](https://www.home-assistant.io/integrations/filter/)
with `time_simple_moving_average` over a 60-second window.

---

## Further Reading

- [Home Assistant Integration Guide](home-assistant.md) — base setup
- [HA Enphase Envoy integration](https://www.home-assistant.io/integrations/enphase_envoy/)
- [OCPP compatibility notes](ocpp-compatibility.md) — charger-specific quirks
- [Development guide](development.md) — architecture, testing
