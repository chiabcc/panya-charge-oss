# Development Guide

This guide covers setting up a development environment for panya-charge-oss, the
open-source OCPP 1.6-J protocol bridge.

## Table of Contents

- [Prerequisites](#prerequisites)
- [Getting the Source](#getting-the-source)
- [Configuration](#configuration)
- [Building](#building)
- [Running](#running)
- [Testing](#testing)
- [MQTT Topics](#mqtt-topics)
- [Debugging](#debugging)
- [Common Issues](#common-issues)

## Prerequisites

| Tool | Version | Purpose |
|------|---------|---------|
| Go | 1.25+ | Language runtime |
| MQTT broker | Mosquitto, EMQX, or Home Assistant | Runtime dependency |
| Git | any | Source control |

**No CGO required.** The project is pure Go with no C dependencies, making
cross-compilation straightforward.

## Getting the Source

```bash
git clone https://github.com/chiabcc/panya-charge-oss.git
cd panya-charge-oss
```

Verify Go modules resolve:

```bash
go mod download
go mod verify
```

## Configuration

The application reads configuration from `config.yaml` in the current directory.
Create one from scratch or use the defaults:

```yaml
server:
  ocpp_port: 8887          # OCPP WebSocket server port
  ocpp_path: /{ws}         # WebSocket endpoint path
  log_level: debug          # debug | info | warn | error
  log_format: text          # text (dev) | json (prod)

mqtt:
  broker: tcp://localhost:1883   # MQTT broker address
  client_id: panya-charge
  base_topic: panya
  disconnect_threshold_sec: 60    # Grid data stale timeout

charging:
  min_amps: 6                   # IEC 61851 minimum
  max_amps: 32                  # Type 2 max
  contactor_cooldown_sec: 180   # Safety cooldown
  default_amps: 6               # Initial limit
```

All values can be overridden via environment variables using the `PANYA_<SECTION>_<KEY>` pattern:

```bash
PANYA_MQTT_BROKER=tcp://localhost:1883 PANYA_SERVER_LOG_LEVEL=debug go run ./cmd/panya-charge-oss
```

## Building

```bash
go build ./...
go build -o bin/panya-charge-oss ./cmd/panya-charge-oss/
```

## Running

```bash
go run ./cmd/panya-charge-oss
```

The CSMS listens on port 8887 for OCPP WebSocket connections.
Point your charger to `ws://localhost:8887/{ws}`.

### Library Usage

To embed the CSMS in your own application, use `pkg/csmsfactory`:

```go
package main

import (
    "context"
    "log"
    "github.com/chiabcc/panya-charge-oss/pkg/csmsfactory"
)

func main() {
    facade, err := csmsfactory.New(csmsfactory.Config{
        Server:   csmsfactory.ServerConfig{OCPPPort: 8887, OCPPPath: "/{ws}"},
        MQTT:     csmsfactory.MQTTConfig{Broker: "tcp://localhost:1883"},
        Charging: csmsfactory.ChargingConfig{MinAmps: 6, MaxAmps: 32},
    })
    if err != nil {
        log.Fatal(err)
    }
    if err := facade.Start(context.Background()); err != nil {
        log.Fatal(err)
    }
}
```

## Testing

### Unit Tests

```bash
go test ./...
```

### Race Detection

```bash
go test -race ./...
```

### Integration Tests

Integration tests start an in-process WebSocket server and simulate a charge
point:

```bash
go test -race -tags=integration ./...
```

### Test Coverage

```bash
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

## MQTT Topics

### Published Topics (base topic: `panya`)

| Topic | Payload | Retained | Description |
|-------|---------|----------|-------------|
| `charge/<id>/status` | `Available`, `Charging`, `Faulted`, etc. | No | Connector status |
| `charge/<id>/power` | `kW` (3 decimals) | No | Instantaneous charging power |
| `charge/<id>/energy` | `kWh` (3 decimals) | No | Session energy |
| `charge/<id>/online` | `online` / `offline` | Yes | Charger availability |
| `charge/<id>/current` | `amps` (int) | Yes | Active current limit |
| `charge/<id>/charging_state` | `0` / `1` | No | Charging active or idle |
| `charge/<id>/proxy_connected` | `ON` / `OFF` | Yes | Upstream relay status |
| `grid/power` | `watts` (int) | No | Grid power (negative = export) |

### Subscribed Topics (commands)

| Topic | Payload | Description |
|-------|---------|-------------|
| `charge/command/set_amps` | `amps` (int) | Set current limit for all chargers |
| `charge/command/state` | `start` / `stop` | Start/stop all chargers |
| `charge/<id>/command/set_amps` | `amps` (int) | Set current limit for specific charger |
| `charge/<id>/command/state` | `start` / `stop` | Start/stop specific charger |

### Energy Data Topics

The smart charging controller subscribes to grid power, solar power, and
consumption power topics. Configure these in `mqtt.topics`:

```yaml
mqtt:
  topics:
    grid_power: "grid/power"
    solar_power: "solar/power"        # optional — enables multi-source mode
    consumption_power: "home/power"   # optional — enables multi-source mode
```

When `solar_power` and `consumption_power` are available, the CSMS uses
`solar - consumption` for surplus calculation (more accurate than grid alone).

### Home Assistant MQTT Discovery

The CSMS auto-publishes retained discovery payloads on `homeassistant/<component>/<node_id>/<object_id>/config` when a charger registers via `BootNotification`. Entities are grouped under a single device (manufacturer, model, firmware).

For end-user setup instructions (broker config, charger wiring, Energy Dashboard, example dashboard cards), see the **[Home Assistant Integration Guide](home-assistant.md)**.

**Primary entities** (visible on the device dashboard):

| Entity | Type | Topic | Unit | Notes |
|--------|------|-------|------|-------|
| Status | sensor | `charge/<id>/status` | — | Connector status (Available, Charging, Faulted, etc.) |
| Charging Power | sensor | `charge/<id>/power` | kW | `state_class: measurement` |
| Session Energy | sensor | `charge/<id>/energy` | kWh | `state_class: total_increasing` (HA Energy Dashboard compatible) |
| Grid Power | sensor | `grid/power` | W | `state_class: measurement` |
| Solar Power | sensor | `solar/power` | W | Optional — only if `solar_power` topic configured |
| Home Consumption | sensor | `consumption/power` | W | Optional — only if `consumption_power` topic configured |
| Proxy Connected | binary_sensor | `charge/<id>/proxy_connected` | — | Only when proxy relay enabled |

**Configuration entities** (`entity_category: config`, under the Configuration section):

| Entity | Type | Topic | Notes |
|--------|------|-------|-------|
| Charging Current | number | `charge/<id>/current` | Slider, min/max from config, amps |
| Charging | switch | `charge/<id>/charging_state` | Start/stop toggle |

All entities use `charge/<id>/online` as availability topic — HA marks the device unavailable when the charger disconnects.

## Debugging

### OCPP WebSocket Debugging

Set `log_level: debug` to see all OCPP message exchanges:

```yaml
server:
  log_level: debug
```

### MQTT Debugging

Monitor published topics:

```bash
mosquitto_sub -h localhost -t "panya/#" -v
```

Publish test commands:

```bash
mosquitto_pub -h localhost -t "panya/charge/command/set_amps" -m "16"
mosquitto_pub -h localhost -t "panya/charge/command/state" -m "start"
```

### OCPP Message Logging

The CSMS logs inbound and outbound OCPP messages with charger ID, action,
and payload when running at debug level.

## Smart Charging Architecture

```
Grid Power MQTT  ──┐
Solar Power MQTT  ──┼──→ EnergyTracker → Calculator → Controller → SetChargingProfile
Consumption MQTT ───┘                              (OCPP command)
```

1. **EnergyTracker** collects grid, solar, and consumption power from MQTT
2. **Calculator** computes ideal current limit from surplus data
3. **Controller** applies hysteresis, cooldown, and safety limits, then sends
   `SetChargingProfile` via OCPP

## Architecture

```
cmd/
  panya-charge-oss/   # main entry point

internal/
  config/             # YAML + env config loader
  csms/               # CSMS implementation (Start, Stop, Subscribe, Chargers)
  domain/
    charger/          # Charger, Connector, ConnectorStatus types
    session/          # Session domain types
    smartcharging/    # Pure-Go solar surplus calculator
    proxy/            # Proxy relay policy
    ports/            # Hexagonal port interfaces + in-memory repos
  adapter/
    inbound/
      mqtt/           # MQTT command subscriber
    outbound/
      ocpp/           # OCPP WebSocket server + handlers
      mqtt/           # MQTT publisher + HA discovery

pkg/
  csms/               # Public Facade interface + events
  csmsfactory/        # Factory: New(cfg) → Facade
```

## Common Issues

| Symptom | Cause | Fix |
|---------|-------|-----|
| `mqtt connect: dial tcp` error | No MQTT broker running | Start Mosquitto: `docker run -d -p 1883:1883 eclipse-mosquitto` |
| Charger won't connect | OCPP URL mismatch | Use `ws://host:8887/{ws}` exactly |
| `charging.min_amps must be >= 6` | Config validation | Set `min_amps: 6` |
| `SetChargingProfile` rejected by charger | Wrong profile type | Use `TxDefaultProfile` with stack level 1 |
| Port already in use: 8887 | Another process on the port | Change `ocpp_port` or kill the process |