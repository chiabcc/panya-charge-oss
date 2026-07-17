# OCPP Compatibility

## Supported Version

**OCPP 1.6-J** (JSON over WebSocket). This is the only version supported.

## Supported Inbound Messages (Charger → CSMS)

The CSMS processes the following inbound OCPP messages:

| Message | Handler | Description |
|---------|---------|-------------|
| `BootNotification` | `HandleBootNotification` | Charger registration; stores vendor, model, firmware |
| `StatusNotification` | `HandleStatusNotification` | Connector status updates (Available, Charging, Faulted, etc.) |
| `MeterValues` | `HandleMeterValues` | Power, energy, and current readings |
| `StartTransaction` | `HandleStartTransaction` | Charging session start |
| `StopTransaction` | `HandleStopTransaction` | Charging session end |
| `Heartbeat` | `HandleHeartbeat` | Charger liveness check |

## Supported Outbound Commands (CSMS → Charger)

| Command | Description |
|---------|-------------|
| `RemoteStartTransaction` | Start charging remotely |
| `RemoteStopTransaction` | Stop active charging remotely |
| `SetChargingProfile` | Apply current limit profile |
| `ClearChargingProfile` | Remove charging profile |

### SetChargingProfile Constraints

This CSMS uses `TxDefaultProfile` with `Relative` kind and a single charging schedule period. See [Hardware Reference](./hardware/abb-terra-ac.md) for charger-specific constraints.

**Important:** `SetChargingProfile` is **local-only** — it is never forwarded through the proxy relay, even if upstream relay is configured.

## Connector Status Values

The CSMS tracks the following connector statuses (matching OCPP 1.6 spec):

- `Available`
- `Preparing`
- `Charging`
- `SuspendedEV`
- `SuspendedEVSE`
- `Finishing`
- `Reserved`
- `Unavailable`
- `Faulted`

## Proxy Relay

The CSMS includes a proxy relay policy engine that can forward OCPP messages to an upstream CSMS (e.g., a vendor cloud). Messages are checked against the relay policy before local processing:

- `BootNotification`, `StatusNotification`, `MeterValues` — forwarded if relay is active
- `StartTransaction`, `StopTransaction` — always processed locally
- `SetChargingProfile` — **never** forwarded (local-only)
- `Heartbeat` — forwarded if relay is active

See [`internal/adapter/outbound/ocpp/router.go`](../../internal/adapter/outbound/ocpp/router.go) for implementation details.

## WebSocket Configuration

| Parameter | Default | Description |
|-----------|---------|-------------|
| Port | `8887` | OCPP WebSocket server port |
| Path | `/{ws}` | WebSocket endpoint path |

The path `/{ws}` is required by the `ocpp-go` library — it matches the standard OCPP URL pattern.

## Security

- No TLS support in the base CSMS (terminate with a reverse proxy in production)
- No authentication at the OCPP layer (charger identity is derived from `chargePointIdentity` in `BootNotification`)
- For production deployments, use a reverse proxy (nginx, Caddy) with TLS termination and IP allowlisting