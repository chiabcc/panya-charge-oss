# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.0.2](https://github.com/chiabcc/panya-charge-oss/compare/v0.0.1...v0.0.2) (2026-07-18)


### Bug Fixes

* correct ABB Terra AC display spec (LED indicators, no touchscreen) ([79abf95](https://github.com/chiabcc/panya-charge-oss/commit/79abf95b25647648ef00855228cc3d338cb8f925))

## [0.0.1] - 2026-07-17

Initial public release. Pure OCPP 1.6-J ↔ MQTT protocol bridge for Home Assistant.

### Added

- **OCPP 1.6-J CSMS** — WebSocket server for EV charger connectivity
  - BootNotification / StatusNotification handling
  - Transaction lifecycle (StartTransaction, MeterValues, StopTransaction)
  - Charging profile management (SetChargingProfile, ClearChargingProfile)
  - Remote start/stop transaction commands
  - Proxy/relay mode for upstream vendor cloud forwarding
- **MQTT Bridge** — Publishes charger status, power, energy, and online state
- **Home Assistant MQTT Discovery** — Auto-discovers charger entities via MQTT
  - 7 primary entities (sensors + binary sensors)
  - 2 configuration entities (current slider, charging switch)
  - Energy Dashboard compatible (Session Energy with `state_class: total_increasing`)
- **Smart Charging** — Solar surplus optimization controller
  - Grid-only mode and multi-source mode (grid + solar + consumption)
  - Pure-Go calculator (no I/O imports in domain layer)
  - 180s contactor cooldown, 6A fallback on stale data
- **CSMS Facade API** (`pkg/csms/`) — Public interface for embedding
  - Start/Stop lifecycle, Subscribe to 7 typed domain events, Chargers snapshot
  - `pkg/csmsfactory/` factory with config mapping and validation
- **CLI Entry Point** (`cmd/panya-charge-oss/main.go`)
  - YAML config with env var overrides (`PANYA_<SECTION>_<KEY>`)
  - slog structured logging (text or JSON)
  - Graceful shutdown via SIGINT/SIGTERM
- **In-memory state** — No database dependency; all state lives in process memory
- **Hexagonal architecture** — Domain/ports/adapters separation; domain layer is pure Go
- **Configuration** via YAML file with environment variable overrides
- **Documentation**:
  - Home Assistant integration guide (`docs/home-assistant.md`)
  - Development guide (`docs/development.md`)
  - OCPP compatibility notes (`docs/ocpp-compatibility.md`)
- **CI/CD** — GitHub Actions workflow (build, vet, test -race, golangci-lint)

### Target Hardware

- **ABB Terra AC W22-G5-R-0** (22kW, Type 2) — firmware ≥ 1.8.32 required for OCPP 1.6-J

> **Note:** This release was developed and validated using an OCPP 1.6-J
> simulator. Real hardware testing on the ABB Terra AC is pending. Charger
> compatibility notes in `docs/hardware/` reflect OCPP protocol research and
> vendor documentation, not empirical validation.

### Notes

- Pure Go implementation — no CGO, no c-ares, no SQLite
- Cross-compiles cleanly for linux/amd64, linux/arm64, darwin/arm64
- Apache 2.0 licensed
- No database, no web UI, no authentication — pure protocol bridge

[Unreleased]: https://github.com/chiabcc/panya-charge-oss/compare/v0.0.1...main
[0.0.1]: https://github.com/chiabcc/panya-charge-oss/releases/tag/v0.0.1
