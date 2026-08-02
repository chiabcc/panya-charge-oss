# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.2.1](https://github.com/chiabcc/panya-charge-oss/compare/v0.2.0...v0.2.1) (2026-08-02)


### Bug Fixes

* **smart-charging:** start energy poll loop, normalize kW sensors ([d9d44e3](https://github.com/chiabcc/panya-charge-oss/commit/d9d44e32538d695ed38272f293d8ac947761966d))

## [0.2.0](https://github.com/chiabcc/panya-charge-oss/compare/v0.1.0...v0.2.0) (2026-07-30)


### Features

* **energy:** native HA entity reader adapter ([#12](https://github.com/chiabcc/panya-charge-oss/issues/12)) ([114901f](https://github.com/chiabcc/panya-charge-oss/commit/114901fabf7e34542d55f9e988983352ebdbaa2c))
* **ha-addon:** add brand icon, maintainer, changelog, favicon ([427cb39](https://github.com/chiabcc/panya-charge-oss/commit/427cb39bb7c82bf6d37bcb7eaa9315ae784bb562))
* **ha-addon:** entity selector dropdowns, remove deprecated MQTT topic fields ([72f09d6](https://github.com/chiabcc/panya-charge-oss/commit/72f09d6b2afea229ca86f4a24f6777905dd23045))
* **webui:** show MQTT input topics on status page ([fbc2f51](https://github.com/chiabcc/panya-charge-oss/commit/fbc2f51805baa970a939b040a849545e0c32305f))


### Bug Fixes

* **ci:** build per-arch images matching HA {arch} convention ([3fde7f6](https://github.com/chiabcc/panya-charge-oss/commit/3fde7f64f5ee2745c116a457a462b6d5525e4c0c))
* **energy:** disable controller when no energy entities configured ([5e404bd](https://github.com/chiabcc/panya-charge-oss/commit/5e404bd30f753d16e722e1f90f60da990008a4cd))
* **ha-addon:** add icon and logo flags so logo shows on info page ([93063c7](https://github.com/chiabcc/panya-charge-oss/commit/93063c70f53b8db91011df7209e185d17ba1da05))
* **ha-addon:** add valid AppArmor profile + bump to 0.1.2 ([da77d91](https://github.com/chiabcc/panya-charge-oss/commit/da77d913797d02a99234062c2faad599500bb6e2))
* **ha-addon:** bind status page to 0.0.0.0 for ingress ([81ac0ae](https://github.com/chiabcc/panya-charge-oss/commit/81ac0ae288ff7af962aaedc78a3f926a14c42b32))
* **ha-addon:** bump version to 0.1.1 for apparmor fix ([910fe73](https://github.com/chiabcc/panya-charge-oss/commit/910fe7315c2db2ff3e00ba8fe4b522ef8ed95f64))
* **ha-addon:** copy icon.png and logo.png into Docker image ([a11859b](https://github.com/chiabcc/panya-charge-oss/commit/a11859bcd8a7f8c8377c5f58f9879dca7eca36d0))
* **ha-addon:** correct schema list syntax and services format ([11b1bec](https://github.com/chiabcc/panya-charge-oss/commit/11b1bec9c367d37017d2cda3810c161d4d654c7b))
* **ha-addon:** diagnostic MQTT errors + manual broker override ([bdcd011](https://github.com/chiabcc/panya-charge-oss/commit/bdcd011c26a2f3f82d8bc4535f89c7df3c064598))
* **ha-addon:** drop bashio, use plain bash + Supervisor REST API ([3a5b165](https://github.com/chiabcc/panya-charge-oss/commit/3a5b1659bf9bd0f48e63073fd10f4cb75a5d5a8e))
* **ha-addon:** entity? is not a valid schema type, use str? ([ae5a560](https://github.com/chiabcc/panya-charge-oss/commit/ae5a5600e393d28d0cfcd47dabee569717afab14))
* **ha-addon:** remove broken AppArmor profile ([b2d3d9d](https://github.com/chiabcc/panya-charge-oss/commit/b2d3d9dc8c7127716a37b10780f66c9e1c2b61d8))
* **ha-addon:** remove ingress_entry to avoid double-slash 404 ([2e5b2f9](https://github.com/chiabcc/panya-charge-oss/commit/2e5b2f91122d4bb2510902465c8880a9eb8d9dd4))
* **ha-addon:** remove invalid icon/logo keys, bump version to 0.1.20 ([97bce64](https://github.com/chiabcc/panya-charge-oss/commit/97bce64518564bd0247d6f16483162b8b5aadca5))
* **ha-addon:** replace blank logo.png with brand icon ([0a522ec](https://github.com/chiabcc/panya-charge-oss/commit/0a522ec22633301808259db8448c9278b14f62e5))
* **ha-addon:** restore external MQTT broker override fields ([7617f5e](https://github.com/chiabcc/panya-charge-oss/commit/7617f5e38401bc976eb6b5598908570033e1bc32))
* **ha-addon:** switch from HA base to Alpine to avoid s6-overlay ([f7a280f](https://github.com/chiabcc/panya-charge-oss/commit/f7a280f099e030aeb6ba146885cc10d471f6fce7))
* **ha-addon:** use {arch} placeholder in image field per HA convention ([1c3711b](https://github.com/chiabcc/panya-charge-oss/commit/1c3711befb2c0e5291f99621b5744d77da3c83a2))
* **ha-addon:** use #!/usr/bin/env bashio for s6 v3 compatibility ([79fd706](https://github.com/chiabcc/panya-charge-oss/commit/79fd706acbb4414e54a316984c9924ec20b33cd0))
* **ha-addon:** use correct Supervisor services API path ([ff640bb](https://github.com/chiabcc/panya-charge-oss/commit/ff640bb1caec482ef3a2c7d0ab1c5806f215f5d4))
* **ha-addon:** use valid entity? schema instead of entity(sensor)? ([c76c553](https://github.com/chiabcc/panya-charge-oss/commit/c76c553317167d7d28c3b4ddafa5f4fce1424e2e))
* keep process alive when MQTT unreachable ([2801c82](https://github.com/chiabcc/panya-charge-oss/commit/2801c82f0ffaf45d641ae20312f3a556053fbcb4))
* **webui:** entity ID code blocks unreadable on dark status page ([faa65f1](https://github.com/chiabcc/panya-charge-oss/commit/faa65f18cccd3df1bb75c243ff32cd512cd648bc))
* **webui:** prefix ingress path on / → /status redirect ([69d45aa](https://github.com/chiabcc/panya-charge-oss/commit/69d45aa3a4bdfbbd49050c3bf9e6499922e434b0))
* **webui:** redirect GET / to /status in status-only mode ([34770c8](https://github.com/chiabcc/panya-charge-oss/commit/34770c86da1f03e74e19aca65073e090ed5739cd))
* **webui:** serve status page at GET / directly (no redirect) ([58524cb](https://github.com/chiabcc/panya-charge-oss/commit/58524cb0550eafd8bde85f20bc6cd4efc3fb790c))
* **webui:** show real HA IP in OCPP URL, drop {ws} from display ([200a545](https://github.com/chiabcc/panya-charge-oss/commit/200a545adfc4cb5390c1f46c722d1931f09dc775))

## [0.1.0](https://github.com/chiabcc/panya-charge-oss/compare/v0.0.2...v0.1.0) (2026-07-28)


### Features

* embedded config WebUI with selective hot reload ([#8](https://github.com/chiabcc/panya-charge-oss/issues/8)) ([554d09a](https://github.com/chiabcc/panya-charge-oss/commit/554d09a18d51340cc172f4277325874dd1b25bfd))
* Home Assistant add-on packaging with read-only status page ([#9](https://github.com/chiabcc/panya-charge-oss/issues/9)) ([1df923e](https://github.com/chiabcc/panya-charge-oss/commit/1df923eeae1cde341fbbbf03e4c4c343980e6033))
* **smart-charging:** add global ON/OFF toggle switch for HA ([#6](https://github.com/chiabcc/panya-charge-oss/issues/6)) ([b1315b1](https://github.com/chiabcc/panya-charge-oss/commit/b1315b18629b44f6ee61580ba445acdfa93a0808))


### Bug Fixes

* **ci:** correct HA builder version tag format ([74e4569](https://github.com/chiabcc/panya-charge-oss/commit/74e45697a0c0f3151a864ec681c47fe691e45e4e))
* **ci:** replace deprecated HA builder with standard Docker buildx ([f1e42bb](https://github.com/chiabcc/panya-charge-oss/commit/f1e42bb1520a4a76d88e10a56d98a32ee52a608d))
* **ci:** use last legacy HA builder version (2026.02.1) ([fe7b4e4](https://github.com/chiabcc/panya-charge-oss/commit/fe7b4e44342bc691a292f9c43164533d4089f13e))
* **ci:** zero-pad HA builder version (2026.06.0) ([263d805](https://github.com/chiabcc/panya-charge-oss/commit/263d805b8cafd0c1f4f91731148873057644d22a))
* **dev-stack:** correct MQTT broker hostname and HA config for compose ([#5](https://github.com/chiabcc/panya-charge-oss/issues/5)) ([4ddb518](https://github.com/chiabcc/panya-charge-oss/commit/4ddb518ac2371ff7f20782b5151e285dca17223f))
* **ha-addon:** add image field for GHCR + set version 0.1.0 ([4f44530](https://github.com/chiabcc/panya-charge-oss/commit/4f445308a328fbaec05c61f8fc4f164dfbe2874e))

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
