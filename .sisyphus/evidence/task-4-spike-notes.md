# Task 4 Spike Notes — Hot-update Plumbing

## SPIKE Findings (file:line references)

### Controller.safeAmps
- **Field**: `internal/adapter/outbound/ocpp/controller.go:24` — `safeAmps int`
- **Constructor**: `internal/adapter/outbound/ocpp/controller.go:39` — `safeAmps` param, line 51 — assigned in NewController
- **Usage (safe fallback)**: `internal/adapter/outbound/ocpp/controller.go:190` (shouldStop path), `internal/adapter/outbound/ocpp/controller.go:207` (SetChargingProfile call), `internal/adapter/outbound/ocpp/controller.go:277` (revertAllToSafe)
- **Concurrency**: No existing mutex on `safeAmps`. Accessed by controller goroutine tick loop. Needs mutex for hot-update.
- **Handler min/max**: `internal/adapter/outbound/ocpp/handler.go:74-75` — `minAmps int`, `maxAmps int` — also no mutex. Used in discovery publishing at `handler.go:185` and `handler.go:517`.

### Calculator min/max
- **Fields**: `internal/domain/smartcharging/calculator.go:15-16` — `minAmps int`, `maxAmps int`
- **Constructor**: `internal/domain/smartcharging/calculator.go:22-29` — `minAmps`, `maxAmps` set in NewCalculator
- **Usage**: `internal/domain/smartcharging/calculator.go:52-62` — clamp idealAmps in Compute()
- **Concurrency**: Calculator is NOT thread-safe for hot-update; `lastLimit` map is mutated without a mutex. Needs sync.Mutex for setter.
- **Note**: Calculator is pure domain (`internal/domain/`) but adding a mutex for hot-update is acceptable — it's a stateful calculator, not pure.

### Commander contactor-cooldown
- **Const**: `internal/adapter/outbound/ocpp/commander.go:18` — `contactorCooldown = 180 * time.Second`
- **Usage**: `internal/adapter/outbound/ocpp/commander.go:40` — `enforceCooldown()` compares `time.Since(last) < contactorCooldown`
- **Concurrency**: `lastStartStop` map already protected by `mu sync.Mutex`. Replacing const with field + same mutex is safe.

### buildLogger
- **Location**: `internal/csms/csms.go:383-404`
- **Pattern**: Parses level string → `slog.Level` constant, creates handler with fixed `*slog.HandlerOptions{Level: sl}`, creates `slog.Logger`. No `*slog.LevelVar` used currently.
- **Handler options**: `internal/adapter/outbound/ocpp/handler.go:76` — `logger *slog.Logger`
- **Controller**: `internal/adapter/outbound/ocpp/controller.go:26` — `logger *slog.Logger`
- **Factory**: `pkg/csmsfactory/factory.go:41-106` — calls `internalcsms.New(internalCfg)` which calls `buildLogger`

### CSMS struct
- **Location**: `internal/csms/csms.go:28-47`
- **Relevant fields**: `handler *ocpp.Handler`, `controller *ocpp.Controller`, `commander *ocpp.Commander`
- **Note**: CSMS struct already holds references to all three — perfect for UpdateCharging method.

### Facade Interface
- **Location**: `pkg/csms/csms.go:11-29`
- **Current methods**: Start, Stop, Subscribe, Chargers
- **Additions needed**: `UpdateCharging(c ChargingParams) error`, `SetLogLevel(level string) error`

### Config Validation Rules
- **Location**: `internal/config/config.go:166-178`
- **Rules**: min >= 6, max <= 32, min <= max
- **Location**: `pkg/csmsfactory/factory.go:42-47` — partial validation (min<6 when non-zero, max>32)

### Handler min/max
- **Fields**: `internal/adapter/outbound/ocpp/handler.go:74-75` — `minAmps int`, `maxAmps int`
- **Usage**: `handler.go:185` — `h.discovery.PublishDiscovery(c, h.minAmps, h.maxAmps, proxyEnabled)`
- **Usage**: `handler.go:517` — same in OnConnect
- **Note**: Handler also needs mutex for min/max for hot-update safety.

## Design Decisions

1. **Controller.SetSafeAmps(int)** — `sync.Mutex` guards the `safeAmps` field. Same pattern for min/max in Calculator.
2. **Calculator.SetLimits(min, max int)** — `sync.Mutex` on Calculator. Validates min >= 6 before storing (the caller validates the bounds, we defensively clamp to 6).
3. **Commander.SetCooldown(time.Duration)** — Replace const with field, default 180s. Same `mu` mutex the Commander already uses.
4. **Handler.SetMinMax** — Mutex-guarded setter for handler min/max (used in discovery publishing).
5. **LevelVar** — Wire `*slog.LevelVar` through buildLogger. CSMS stores `*slog.LevelVar`. Facade.SetLogLevel parses string, adjusts LevelVar.
6. **CSMS.UpdateCharging** — Calls Calculator.SetLimits, Controller.SetSafeAmps, Commander.SetCooldown, Handler.SetMinMax. Validates min/max/coolown before dispatching.

## Thread Safety
- Controller tick goroutine reads safeAmps concurrently with SetSafeAmps writer → needs mutex
- Calculator.Compute() reads min/max concurrently with SetLimits writer → needs mutex
- Commander.enforceCooldown() reads cooldown concurrently with SetCooldown writer → needs atomic or mutex
- Handler reads min/max in OnBootNotification/OnConnect → needs mutex