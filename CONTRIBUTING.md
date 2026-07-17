# Contributing to panya-charge-oss

Thanks for your interest in contributing. This project is an open-source OCPP 1.6-J CSMS bridge that connects EV chargers to Home Assistant via MQTT. We welcome contributions that improve reliability, add charger compatibility, or enhance the smart charging logic.

## Prerequisites

- **Go 1.25+** — the project uses Go 1.25 as the minimum version
- **MQTT broker** — a local Mosquitto or EMQX instance running on `tcp://localhost:1883`
- **EV charger** — an OCPP 1.6-J compatible charger for end-to-end testing
- **Git**

## Development Workflow

### Run the server

```bash
go run ./cmd/panya-charge-oss
```

This starts the OCPP CSMS server and MQTT bridge with default configuration (OCPP on port 8887, MQTT on `tcp://localhost:1883`).

### Run tests

All tests should pass with race detection enabled:

```bash
go test -race ./...
```

### Run linters

```bash
golangci-lint run
```

Uses `golangci-lint` for code quality checks.

### Build

```bash
go build ./...
```

## Code Style

- **Formatting**: run `gofmt` and `goimports` on every file. Configure your editor to apply these on save.
- **Linting**: `golangci-lint` enforces style and quality rules. Run `golangci-lint run` before committing.
- **Naming**: follow standard Go conventions (MixedCaps for exported identifiers, lowercase for unexported). See the [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments).
- **Error handling**: wrap errors with `fmt.Errorf("context: %w", err)`. Do not swallow errors.
- **Logging**: use `log/slog` with structured fields. No unstructured `log.Print` calls.

## Architecture

The project follows a **hexagonal architecture** with three layers:

```
┌─────────────────────────────────────────────────────────┐
│                    Adapters                             │
│  inbound/  (MQTT subscriber → commands)                │
│  outbound/  (OCPP server, MQTT publisher)               │
├─────────────────────────────────────────────────────────┤
│                     Ports                               │
│  interfaces in internal/domain/ports/                   │
├─────────────────────────────────────────────────────────┤
│                     Domain                              │
│  charger/, session/, smartcharging/, proxy/             │
│  Pure Go — no I/O, no database, no network             │
└─────────────────────────────────────────────────────────┘
```

### Key boundaries

1. **`internal/domain/` must not import I/O packages** — no database, HTTP, or MQTT imports in domain code. Domain types and logic are pure Go.
2. **Ports live in `internal/domain/ports/`** — interfaces define what adapters must implement. The in-memory implementations in `inmemory.go` satisfy all repository interfaces.
3. **Adapters implement ports** — the OCPP adapter (`internal/adapter/outbound/ocpp/`) and MQTT adapters (`internal/adapter/inbound/mqtt/`, `internal/adapter/outbound/mqtt/`) are the only code that touches the network.

### Public facade

The `pkg/csms/` package is the only public API. It defines the `Facade` interface and `Event` types that downstream applications consume. See `pkg/csms/csms.go`, `pkg/csms/event.go`, and `pkg/csms/emitter.go`.

## Testing

Run the full suite with race detection:

```bash
go test -race ./...
```

The smart charging calculator (`internal/domain/smartcharging/`) and proxy policy (`internal/domain/proxy/`) have unit tests. MQTT discovery (`internal/adapter/outbound/mqtt/`) has snapshot tests for Home Assistant payloads.

> **Note:** The CSMS is currently validated against an OCPP 1.6-J simulator.
> Real hardware testing (ABB Terra AC) is pending. If you have access to OCPP
> 1.6-J hardware, compatibility testing and bug reports are especially welcome.

## Commit Conventions

This project uses [**Conventional Commits**](https://www.conventionalcommits.org/).
Releases are automated via [release-please](https://github.com/googleapis/release-please),
which generates changelogs and version bumps from commit messages.

### Format

```
<type>(<scope>): <description>

[optional body]

[optional footer(s)]
```

### Types

| Type | Section in changelog | Bump |
|------|---------------------|------|
| `feat` | Features | minor |
| `fix` | Bug Fixes | patch |
| `perf` | Performance | patch |
| `refactor` | Refactoring | — |
| `docs` | Documentation | — |
| `test` | Tests | — |
| `ci` | CI/CD | — |
| `build` | Build System | — |
| `chore` | (hidden) | — |

### Breaking changes

Use `!` after the type/scope, or a `BREAKING CHANGE:` footer. These trigger a
major version bump.

```
feat!(api): rename Facade.Start to Facade.Run

BREAKING CHANGE: Facade.Start is renamed to Facade.Run.
```

### Scopes (optional but recommended)

`config`, `csms`, `ocpp`, `mqtt`, `charging`, `discovery`, `proxy`, `docs`, `ci`

## Pull Request Process

1. Fork the repository
2. Create a feature branch from `main`
3. Write code and tests
4. Run `golangci-lint run` and `go test -race ./...`
5. Open a PR against `main`
6. CI runs tests and linting automatically
7. At least one review is required before merge

## What to contribute

Good first contributions:

- Charger-specific compatibility fixes (OCPP message handling)
- MQTT topic configuration improvements
- Smart charging calculator enhancements
- Documentation and examples
- Tests for untested paths

## License

By contributing, you agree that your contributions will be licensed under the [Apache 2.0](LICENSE) license.