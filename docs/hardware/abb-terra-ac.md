# Hardware Reference — ABB Terra AC W22-G5-R-0

> **Status:** Target hardware. This document reflects OCPP protocol research
> and vendor documentation. The CSMS has been validated against an OCPP
> simulator; real hardware testing on the ABB Terra AC is pending.

## Overview

The primary hardware target for panya-charge-oss is the **ABB Terra AC W22-G5-R-0**, a 22 kW Type 2 AC wallbox charger with OCPP 1.6-J support.

## Specifications

| Parameter | Value |
|-----------|-------|
| Max power | 22 kW (3-phase) |
| Connector | Type 2 |
| Protocol | OCPP 1.6-J over WebSocket |
| IP rating | IP54 (indoor/outdoor) |
| Display | LED status indicators (green / yellow / red) |

## Firmware Requirements

- **Minimum firmware**: 1.8.32 (OCPP 1.6-J support)
- Check firmware version via `BootNotification` payload (no display on this model)

## OCPP Configuration

Point the charger's OCPP backend URL to:

```
ws://<host>:8887/{ws}
```

Where `<host>` is the machine running panya-charge-oss. The `/{ws}` path component is required — it's the WebSocket endpoint path used by the OCPP server.

## Charging Profile Constraints

This charger has specific requirements for `SetChargingProfile` calls:

### Profile Type

Use **`TxDefaultProfile`** only. The charger does not accept `ChargePointMaxProfile`.

### Stack Level

The charger rejects `stackLevel > 1`. A single profile (stack level 1) is used.

### Profile Kind

Use **`Relative`** kind only. Absolute profiles are not supported.

### Charging Schedule

- `startPeriod` must be `0`
- `limit > 0` (the charger rejects a zero-amp limit)
- Rate unit: `A` (amperes)

### Implementation Reference

See [`internal/adapter/outbound/ocpp/profile.go`](../../internal/adapter/outbound/ocpp/profile.go) for the exact profile construction used by the CSMS:

```go
// Key constants:
stackLevel = 1
profileKind = Relative
profilePurpose = TxDefaultProfile
startPeriod = 0
limit = min(ideal_amps, max_amps)
```

### Mid-Session Profile Updates (TxProfile)

**Confirmed on real ABB Terra AC hardware:** `SetChargingProfile` with `purpose=TxDefaultProfile` is accepted by the charger mid-session but is **NOT applied** to the ongoing transaction. The charger only applies the TxDefaultProfile that was current when the transaction started.

**Workaround implemented in the CSMS:** When a connector has an active transaction, the CSMS sends **two** charging profiles:
1. `TxDefaultProfile` — persists the new limit for future transactions (unchanged behavior).
2. `TxProfile` with `transactionId` set to the active session's OCPP transaction ID — modifies the live transaction's current limit immediately.

If the charger rejects the TxProfile (e.g., firmware doesn't support it), the CSMS logs a warning but does NOT fail the call — the TxDefaultProfile was already accepted and will apply on the next transaction:

```
level=WARN msg="txProfile not applied — charger may not support modifying live transactions" charger=... connector=... transactionId=... err=...
```

## Safety Considerations

### Contactor Protection

The CSMS enforces a **minimum 180-second cooldown** between start/stop commands to prevent physical contactor damage. This is configurable via `charging.contactor_cooldown_sec` in `config.yaml`.

### Minimum Current

A **6A minimum** is enforced (IEC 61851). The charger will not accept charging profiles below this threshold.

### MQTT Disconnect Fallback

If no grid power data is received for more than 60 seconds (configurable via `mqtt.disconnect_threshold_sec`), the CSMS falls back to the minimum current (6A) as a safe state.

## Commissioning Checklist

1. Verify firmware version >= 1.8.32
2. Configure charger OCPP URL: `ws://<host>:8887/{ws}`
3. Configure MQTT broker connection in `config.yaml`
4. Verify charger boots and registers (check logs for `BootNotification` success)
5. Verify MQTT topics are published (`panya/charge/<id>/status`)
6. Test remote start/stop via MQTT command topics
7. Verify smart charging response to grid power readings

## Troubleshooting

| Symptom | Cause | Fix |
|---------|-------|-----|
| Charger rejects `SetChargingProfile` | Wrong profile type or stack level | Use `TxDefaultProfile`, stack level 1 |
| Charger connects then disconnects | OCPP URL path mismatch | Ensure URL ends with `/{ws}` |
| `stackLevel` rejected | Stack level > 1 | Use max stack level minus 1 |
| Charger ignores charging limit | Limit = 0 | Ensure limit > 0 (min 6A) |
| No charging profile applied | `absolute` kind used | Use `relative` kind only |