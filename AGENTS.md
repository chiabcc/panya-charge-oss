# AGENTS.md — Context for AI agents working on panya-charge-oss

## Project Overview

`panya-charge-oss` is an open-source OCPP 1.6-J protocol bridge that connects EV chargers to Home Assistant via MQTT. It implements a CSMS (Central System Management System) that speaks OCPP 1.6-J over WebSocket to chargers, and publishes charger telemetry to MQTT topics that Home Assistant discovers automatically.

This is the **OSS core** — a pure protocol bridge with no database and no authentication; ships an optional embedded config WebUI (disabled by default — see CONTEXT.md). The commercial `panya-charge` project builds on this core with multi-tenant PostgreSQL, a web dashboard, auth, and AI optimization.

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
      webui/          # config WebUI server
    outbound/
      ocpp/           # OCPP WebSocket server + handlers
      mqtt/           # MQTT publisher (telemetry + HA discovery)
pkg/
  csms/               # public facade: Facade interface + Events
  csmsfactory/        # factory: New(cfg) → Facade
ha-addon/             # HA Add-on Store packaging
    config.yaml       # HA add-on manifest (schema, NOT panya app config)
    run.sh            # Bash launcher (translates options.json → PANYA_* env vars + MQTT discovery)
    Dockerfile        # Multi-stage build (golang builder → Alpine runtime)
    icon.png          # Add-on icon (shown in HA sidebar and add-on store)
    CHANGELOG.md      # User-facing changelog (shown in HA update dialog + Documentation tab)
    README.md         # Add-on store landing page
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
6. **WebUI binds loopback by default; non-loopback bind REQUIRES webui.token; mqtt.password must never appear in any HTTP response.**
7. **`config.yaml` filename is RESERVED by HA Supervisor's recursive search.** Do not move or rename `ha-addon/config.yaml`. Repo root's `config.yaml.example` is safe (`.example` suffix prevents discovery).
8. **HA add-on mode forces `webui.enabled=false`**; the bash launcher (`ha-addon/run.sh`) is the sole bridge between Supervisor's `options.json` and panya's env vars. It uses plain bash + jq + curl to the Supervisor REST API (no bashio).
9. **`repository.yaml` at repo root is the HA add-on repo registration file.**
10. **Every add-on release MUST update `ha-addon/CHANGELOG.md`** before tagging. HA Supervisor shows this file to users in the add-on update dialog and Documentation tab. See [Release Process](#ha-add-on-release-process) below.

## Hardware Target

- **ABB Terra AC W22-G5-R-0** (22kW, Type 2)
- Requires firmware ≥ 1.8.32 for OCPP 1.6-J support
- Use `TxDefaultProfile`, NOT `ChargePointMaxProfile`
- `stackLevel` bug: use max-1
- Relative kind only, limit > 0, startPeriod = 0

## Issue Tracker

See: https://github.com/chiabcc/panya-charge-oss/issues

## HA Add-on Release Process

Every release MUST follow these steps in order:

1. **Bump version** in `ha-addon/config.yaml` (`version: "X.Y.Z"`).
2. **Update `ha-addon/CHANGELOG.md`** — add a new `## X.Y.Z` section at the top with user-facing bullet points. Write for end-users (not developers). Describe what changed, what was fixed, what they'll notice. Keep it concise — one line per bullet.
3. **Commit** both files together: `git commit -m "release: vX.Y.Z"`.
4. **Tag**: `git tag -a vX.Y.Z -m "vX.Y.Z"` and push both.
5. CI auto-builds per-arch images and pushes to GHCR.
6. **Verify the GHCR image locally** before telling users to update (see below).

### CHANGELOG format

```markdown
## X.Y.Z

- One-line user-facing change (imperative mood, no jargon)
- Another change
```

Rules:
- Newest version at top.
- No "internal" or "refactor" entries — users don't care.
- If a fix resolves a user-reported error, name the symptom (e.g., "Fix ingress 404 after update").
- Every tagged release gets a section, even if it's a single fix.

### Pre-release verification

Before telling users to update, pull and test the exact GHCR image:

```bash
docker pull ghcr.io/chiabcc/amd64-panya-charge-oss:X.Y.Z
docker run --rm --entrypoint sh ghcr.io/chiabcc/amd64-panya-charge-oss:X.Y.Z -c 'cat /etc/alpine-release'
# Run with mock options.json to verify start.sh works
```