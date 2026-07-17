package ocpp

import (
	"github.com/xBlaz3kx/ocpp-go/ocpp1.6/types"
)

const (
	// abbTxDefaultStackLevel: ABB Terra AC accepts TxDefaultProfile at stackLevel=1.
	// Do NOT use ChargePointMaxProfile or stackLevel > 1 — the charger rejects it.
	// Reference: issue #8, evcc-io/evcc#28868, lbbrhzn/ocpp#1737
	abbTxDefaultStackLevel = 1
	abbProfileID           = 1
	abbProfileKind         = types.ChargingProfileKindRelative
	abbProfilePurpose      = types.ChargingProfilePurposeTxDefaultProfile
)

func buildTxDefaultProfile(connectorID, limitAmps int) *types.ChargingProfile {
	stackLevel := abbTxDefaultStackLevel

	period := types.NewChargingSchedulePeriod(0, float64(limitAmps))
	schedule := types.NewChargingSchedule(types.ChargingRateUnitAmperes, period)

	return types.NewChargingProfile(
		abbProfileID,
		stackLevel,
		abbProfilePurpose,
		abbProfileKind,
		schedule,
	)
}
