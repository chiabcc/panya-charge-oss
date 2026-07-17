package proxy

import (
	"encoding/json"
	"testing"
)

func TestRouteDecision_String(t *testing.T) {
	cases := []struct {
		decision RouteDecision
		want     string
	}{
		{DecisionLocalOnly, "local_only"},
		{DecisionUpstreamOnly, "upstream_only"},
		{DecisionBoth, "both"},
		{DecisionDrop, "drop"},
		{RouteDecision(99), "unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			if got := tc.decision.String(); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRouteDecision_ShouldForward(t *testing.T) {
	if !DecisionUpstreamOnly.ShouldForward() {
		t.Fatal("UpstreamOnly should forward")
	}
	if !DecisionBoth.ShouldForward() {
		t.Fatal("Both should forward")
	}
	if DecisionLocalOnly.ShouldForward() {
		t.Fatal("LocalOnly should not forward")
	}
	if DecisionDrop.ShouldForward() {
		t.Fatal("Drop should not forward")
	}
}

func TestRouteDecision_ShouldHandleLocally(t *testing.T) {
	if !DecisionLocalOnly.ShouldHandleLocally() {
		t.Fatal("LocalOnly should handle locally")
	}
	if !DecisionBoth.ShouldHandleLocally() {
		t.Fatal("Both should handle locally")
	}
	if DecisionUpstreamOnly.ShouldHandleLocally() {
		t.Fatal("UpstreamOnly should not handle locally")
	}
	if DecisionDrop.ShouldHandleLocally() {
		t.Fatal("Drop should not handle locally")
	}
}

func TestDefaultPolicy_Inbound(t *testing.T) {
	p := DefaultPolicy()

	cases := []struct {
		action string
		want   RouteDecision
	}{
		{ActionBootNotification, DecisionBoth},
		{ActionHeartbeat, DecisionBoth},
		{ActionAuthorize, DecisionUpstreamOnly},
		{ActionStartTransaction, DecisionBoth},
		{ActionStopTransaction, DecisionBoth},
		{ActionStatusNotification, DecisionBoth},
		{ActionMeterValues, DecisionBoth},
		{ActionDataTransfer, DecisionUpstreamOnly},
		{ActionFirmwareStatusNotification, DecisionUpstreamOnly},
		{ActionDiagnosticsStatusNotification, DecisionUpstreamOnly},
	}

	for _, tc := range cases {
		t.Run(tc.action, func(t *testing.T) {
			got := p.Decide(DirectionInbound, tc.action)
			if got != tc.want {
				t.Fatalf("inbound %s: got %s, want %s", tc.action, got, tc.want)
			}
		})
	}
}

func TestDefaultPolicy_Outbound(t *testing.T) {
	p := DefaultPolicy()

	cases := []struct {
		action string
		want   RouteDecision
	}{
		{ActionSetChargingProfile, DecisionLocalOnly},
		{ActionClearChargingProfile, DecisionLocalOnly},
		{ActionRemoteStartTransaction, DecisionBoth},
		{ActionRemoteStopTransaction, DecisionBoth},
		{ActionTriggerMessage, DecisionBoth},
		{ActionGetConfiguration, DecisionBoth},
		{ActionChangeConfiguration, DecisionBoth},
		{ActionReset, DecisionBoth},
		{ActionUnlockConnector, DecisionBoth},
		{ActionGetLog, DecisionBoth},
		{ActionGetDiagnostics, DecisionBoth},
		{ActionReserveNow, DecisionUpstreamOnly},
		{ActionCancelReservation, DecisionUpstreamOnly},
		{ActionUpdateFirmware, DecisionUpstreamOnly},
		{ActionClearCache, DecisionUpstreamOnly},
	}

	for _, tc := range cases {
		t.Run(tc.action, func(t *testing.T) {
			got := p.Decide(DirectionOutbound, tc.action)
			if got != tc.want {
				t.Fatalf("outbound %s: got %s, want %s", tc.action, got, tc.want)
			}
		})
	}
}

// TestSetChargingProfileNeverForwards is a safety-critical rule: the solar
// surplus logic must never be visible to the upstream vendor cloud. If this
// test fails, smart charging and vendor cloud will conflict.
func TestSetChargingProfileNeverForwards(t *testing.T) {
	p := DefaultPolicy()

	if p.Decide(DirectionOutbound, ActionSetChargingProfile).ShouldForward() {
		t.Fatal("SetChargingProfile must NEVER forward upstream — it conflicts with vendor cloud")
	}
	if p.Decide(DirectionOutbound, ActionClearChargingProfile).ShouldForward() {
		t.Fatal("ClearChargingProfile must NEVER forward upstream")
	}
}

func TestDecide_UnknownAction_DefaultsToLocalOnly(t *testing.T) {
	p := DefaultPolicy()

	got := p.Decide(DirectionInbound, "NonexistentAction")
	if got != DecisionLocalOnly {
		t.Fatalf("unknown inbound action: got %s, want local_only (safe default)", got)
	}

	got = p.Decide(DirectionOutbound, "NonexistentAction")
	if got != DecisionLocalOnly {
		t.Fatalf("unknown outbound action: got %s, want local_only (safe default)", got)
	}
}

func TestMerge_OverridesInbound(t *testing.T) {
	base := DefaultPolicy()

	overrides := map[string]OverrideDecision{
		ActionMeterValues: OverrideDecision(DecisionLocalOnly),
	}

	merged := base.Merge(overrides)

	if got := merged.Decide(DirectionInbound, ActionMeterValues); got != DecisionLocalOnly {
		t.Fatalf("overridden MeterValues: got %s, want local_only", got)
	}

	if got := merged.Decide(DirectionInbound, ActionBootNotification); got != DecisionBoth {
		t.Fatalf("non-overridden BootNotification: got %s, want both", got)
	}
}

func TestMerge_OverridesOutbound(t *testing.T) {
	base := DefaultPolicy()

	overrides := map[string]OverrideDecision{
		ActionReset: OverrideDecision(DecisionUpstreamOnly),
	}

	merged := base.Merge(overrides)

	if got := merged.Decide(DirectionOutbound, ActionReset); got != DecisionUpstreamOnly {
		t.Fatalf("overridden Reset: got %s, want upstream_only", got)
	}
}

func TestMerge_DoesNotMutateBase(t *testing.T) {
	base := DefaultPolicy()

	overrides := map[string]OverrideDecision{
		ActionHeartbeat: OverrideDecision(DecisionLocalOnly),
	}

	_ = base.Merge(overrides)

	if got := base.Decide(DirectionInbound, ActionHeartbeat); got != DecisionBoth {
		t.Fatalf("base policy was mutated: got %s, want both", got)
	}
}

func TestMerge_EmptyOverrides_ReturnsEquivalent(t *testing.T) {
	base := DefaultPolicy()
	merged := base.Merge(nil)

	for action := range base.inbound {
		if merged.Decide(DirectionInbound, action) != base.Decide(DirectionInbound, action) {
			t.Fatalf("empty merge changed decision for %s", action)
		}
	}
}

func TestOverrideDecision_MarshalJSON(t *testing.T) {
	cases := []struct {
		decision OverrideDecision
		want     string
	}{
		{OverrideDecision(DecisionLocalOnly), `"local_only"`},
		{OverrideDecision(DecisionUpstreamOnly), `"upstream_only"`},
		{OverrideDecision(DecisionBoth), `"both"`},
		{OverrideDecision(DecisionDrop), `"drop"`},
	}

	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			data, err := json.Marshal(tc.decision)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if string(data) != tc.want {
				t.Fatalf("got %s, want %s", string(data), tc.want)
			}
		})
	}
}

func TestOverrideDecision_UnmarshalJSON(t *testing.T) {
	cases := []struct {
		input string
		want  OverrideDecision
	}{
		{`"local_only"`, OverrideDecision(DecisionLocalOnly)},
		{`"upstream_only"`, OverrideDecision(DecisionUpstreamOnly)},
		{`"both"`, OverrideDecision(DecisionBoth)},
		{`"drop"`, OverrideDecision(DecisionDrop)},
		{`"unknown_garbage"`, OverrideDecision(DecisionLocalOnly)},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			var got OverrideDecision
			if err := json.Unmarshal([]byte(tc.input), &got); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %d, want %d", got, tc.want)
			}
		})
	}
}

func TestOverrideDecision_RoundTrip(t *testing.T) {
	original := OverrideDecision(DecisionBoth)

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded OverrideDecision
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded != original {
		t.Fatalf("round-trip mismatch: got %d, want %d", decoded, original)
	}
}

func TestPolicyJSON_RoundTrip(t *testing.T) {
	overrides := map[string]OverrideDecision{
		ActionMeterValues: OverrideDecision(DecisionLocalOnly),
		ActionReset:       OverrideDecision(DecisionUpstreamOnly),
	}

	data, err := json.Marshal(overrides)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded map[string]OverrideDecision
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(decoded) != 2 {
		t.Fatalf("expected 2 overrides, got %d", len(decoded))
	}

	if decoded[ActionMeterValues] != OverrideDecision(DecisionLocalOnly) {
		t.Fatal("MeterValues override mismatch after round-trip")
	}
	if decoded[ActionReset] != OverrideDecision(DecisionUpstreamOnly) {
		t.Fatal("Reset override mismatch after round-trip")
	}
}

func TestDefaultPolicy_AllActionsCovered(t *testing.T) {
	p := DefaultPolicy()

	expectedInbound := []string{
		ActionBootNotification, ActionHeartbeat, ActionAuthorize,
		ActionStartTransaction, ActionStopTransaction, ActionStatusNotification,
		ActionMeterValues, ActionDataTransfer,
		ActionFirmwareStatusNotification, ActionDiagnosticsStatusNotification,
	}

	for _, action := range expectedInbound {
		if _, ok := p.inbound[action]; !ok {
			t.Fatalf("inbound action %s has no decision in default policy", action)
		}
	}

	expectedOutbound := []string{
		ActionSetChargingProfile, ActionClearChargingProfile,
		ActionRemoteStartTransaction, ActionRemoteStopTransaction,
		ActionTriggerMessage, ActionGetConfiguration, ActionChangeConfiguration,
		ActionReset, ActionUnlockConnector, ActionGetLog, ActionGetDiagnostics,
		ActionReserveNow, ActionCancelReservation, ActionUpdateFirmware,
		ActionClearCache,
	}

	for _, action := range expectedOutbound {
		if _, ok := p.outbound[action]; !ok {
			t.Fatalf("outbound action %s has no decision in default policy", action)
		}
	}
}
