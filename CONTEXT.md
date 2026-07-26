# CONTEXT.md — Architecture Decisions

This document records the eight architectural decisions for `panya-charge-oss`, the invariants that must hold, and what is out of scope.

## Project Identity

**OCPP 1.6-J bridge, single static binary, no database, optional embedded WebUI disabled by default.**

- **Core protocol bridge**: OCPP ↔ MQTT ↔ Home Assistant
- **No database**: state lives in memory and persists via OCPP
- **Optional embedded WebUI**: config editor, disabled by default
- **No authentication**: intended for LAN trust; `panya-charge` adds auth

## Decision Table

| # | Decision | Choice | Rationale |
|---|-----------|--------|------------|
| 1 | Apply semantics | Selective hot reload + rebuild fallback | charging params/log\_level live; broker/port require restart |
| 2 | Exposure | Opt-in; LAN bind requires auth token | webui.enabled default false; non-loopback without token refused |
| 3 | Env overrides | Effective values; env-set fields read-only | PANYA\_\* wins over YAML; UI shows source badge |
| 4 | Secrets | Write-only | mqtt.password never serialized; empty = keep existing |
| 5 | Rebuild during active session | Warn + explicit confirm; never block | OCPP offline keeps car charging; no deferred queue |
| 6 | UI stack | Go html/template + vendored htmx, go:embed | go build only toolchain; no Node |
| 7 | Docs | This file + one-line patches | AGENTS.md/README identity updated |
| 8 | Tests | TDD for logic, QA scenarios for all | Reload classifier, validation, secret masking, env flags, token gate test-driven |

## Invariants

1. **Domain purity preserved** — `internal/domain/**` never imports I/O packages.
2. **Validate-before-write** — every config save runs the schema validator.
3. **Reload classifier is pure** — no side effects, safe to repeat.
4. **Audit log on saves** — all config changes recorded.
5. **SetChargingProfile local-only unchanged** — never forwarded upstream.

## Out of Scope

- TLS termination (handled by reverse proxy or charger)
- RBAC (commercial `panya-charge` layer)
- Dashboard / visualization (commercial layer)
- Node toolchain (we ship Go binaries only)
- Config history (we provide effective values + source badges)
- Extra API endpoints beyond OCPP/MQTT/WebUI
- YAML comment preservation (we strip comments)
- Schema migration (in-memory model only)
