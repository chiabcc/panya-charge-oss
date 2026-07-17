# AGENTS.md — Context for AI agents working on panya-charge-oss

## Project Overview

`panya-charge-oss` is an open-source OCPP 1.6-J protocol bridge that connects EV chargers to Home Assistant via MQTT. It implements a CSMS (Central System Management System) that speaks OCPP 1.6-J over WebSocket to chargers, and publishes charger telemetry to MQTT topics that Home Assistant discovers automatically.

This is the **OSS core** — a pure protocol bridge with no database, no web UI, and no authentication. The commercial `panya-charge` project builds on this core with multi-tenant PostgreSQL, a web dashboard, auth, and AI optimization.

## Tech Stack

- **Go 1.25+** — hexagonal architecture
- **OCPP 1.6-J** via `xBlaz3kx/ocpp-go` v0.24.0
- **MQTT** via `eclipse/paho.mqtt.golang`
- **WebSocket** via `gorilla/websocket`
- **Config** via `gopkg.in/yaml.v3` with env var overrides

## Project Layout

```
cmd/
  panya-charge-oss/   # main entry point
internal/
  config/             # config loader (YAML + env vars)
  csms/               # internal CSMS implementation
  domain/
    charger/          # domain types (Charger, Connector, ConnectorStatus)
    session/          # domain types (Session)
    smartcharging/    # pure-Go solar surplus calculator
    proxy/            # proxy relay policy
    ports/            # hexagonal interfaces + in-memory repos
  adapter/
    inbound/
      mqtt/           # MQTT subscriber (commands from HA)
    outbound/
      ocpp/           # OCPP WebSocket server + handlers
      mqtt/           # MQTT publisher (telemetry + HA discovery)
pkg/
  csms/               # public facade: Facade interface + Events
  csmsfactory/        # factory: New(cfg) → Facade
```

## Build Commands

```bash
go build ./...          # Build all packages
go test -race ./...     # Run tests with race detection
go vet ./...            # Static analysis
```

## Critical Rules

1. **`internal/domain/**` must NOT import any I/O packages** — no database, net/http, mqtt, or websocket imports in domain code. Domain is pure Go.
2. **All OCPP handlers must call the message router before local logic** — the router (`ocpp.Router`) checks proxy relay policy and may forward messages upstream before local processing.
3. **Contactor protection: minimum 180s between start/stop commands** — the smart charging controller enforces this cooldown to prevent physical contactor damage.
4. **MQTT disconnect > 60s → revert to safe state (6A minimum)** — when grid power data is stale, the controller falls back to the minimum current.
5. **`SetChargingProfile` is local-only — NEVER forwarded upstream** — charging profiles are managed by this CSMS only, never relayed to vendor clouds.

## Hardware Target

- **ABB Terra AC W22-G5-R-0** (22kW, Type 2)
- Requires firmware ≥ 1.8.32 for OCPP 1.6-J support
- Use `TxDefaultProfile`, NOT `ChargePointMaxProfile`
- `stackLevel` bug: use max-1
- Relative kind only, limit > 0, startPeriod = 0

## Issue Tracker

See: https://github.com/chiabcc/panya-charge-oss/issues