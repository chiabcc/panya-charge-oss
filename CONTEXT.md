# CONTEXT.md — Architecture Decisions

This document records the architectural decisions for `panya-charge-oss`, the invariants that must hold, and what is out of scope.

## Project Identity

**OCPP 1.6-J bridge, single static binary, no database, Home Assistant add-on deployment, optional embedded WebUI disabled by default, native HA entity reader for energy input.**

- **Core protocol bridge**: OCPP ↔ MQTT ↔ Home Assistant
- **No database**: state lives in memory and persists via OCPP
- **Optional embedded WebUI**: config editor (standalone dev only, off by default)
- **No authentication**: intended for LAN trust; `panya-charge` adds auth
- **Deployment**: Home Assistant add-on (sole user-facing deployment path)

## Decision Table

| # | Decision | Choice | Rationale |
|---|-----------|--------|------------|
| 1 | Apply semantics | No fields hot-reload in add-on mode; Save = restart | classifier applies only to standalone WebUI (dev) |
| 2 | Exposure | Opt-in; LAN bind requires auth token | webui.enabled default false; non-loopback without token refused |
| 3 | Env overrides | Effective values; env-set fields read-only | PANYA\_\* wins over YAML; UI shows source badge |
| 4 | Secrets | Write-only | mqtt.password never serialized; empty = keep existing |
| 5 | Rebuild during active session | Warn + explicit confirm; never block | OCPP offline keeps car charging; no deferred queue |
| 6 | UI stack | Go html/template + vendored htmx, go:embed | go build only toolchain; no Node |
| 7 | Docs | This file + one-line patches | AGENTS.md/README identity updated |
| 8 | Tests | TDD for logic, QA scenarios for all | Reload classifier, validation, secret masking, env flags, token gate test-driven |
| 9 | HA add-on mode | bash launcher → PANYA_* env vars; schema-driven; WebUI off | matches HA conventions; zero business-logic changes; SUPERVISOR_TOKEN available for MQTT broker discovery and HA entity-state reads |
| 10 | Energy input | Native HA entity reader via Supervisor proxy API | Replaces deprecated MQTT energy path; polls every 10s, keep-last-value on error |

## Invariants

1. **Domain purity preserved** — `internal/domain/**` never imports I/O packages.
2. **Validate-before-write** — every config save runs the schema validator.
3. **Reload classifier is pure** — no side effects, safe to repeat.
4. **Audit log on saves** — all config changes recorded.
5. **SetChargingProfile local-only unchanged** — never forwarded upstream.
6. **HA add-on mode forces webui.enabled=false** — HA schema form is the sole config surface. MQTT broker auto-injected via Services API.

## Out of Scope

- TLS termination (handled by reverse proxy or charger)
- RBAC (commercial `panya-charge` layer)
- Dashboard / visualization (commercial layer)
- Node toolchain (we ship Go binaries only)
- Config history (we provide effective values + source badges)
- Exposing new server-side API endpoints (inbound) beyond OCPP/MQTT/WebUI
- Consuming external APIs (outbound adapters) via domain ports — see EnergySource port (ports.go:87)
- YAML comment preservation (we strip comments)
- Schema migration (in-memory model only)
- Standalone deployment as a user-facing path (library embedding via `pkg/csms` remains supported for developers)
