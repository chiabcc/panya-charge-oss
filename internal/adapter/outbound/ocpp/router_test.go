package ocpp

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"

	"github.com/xBlaz3kx/ocpp-go/ocpp1.6/core"
	"github.com/xBlaz3kx/ocpp-go/ocpp1.6/firmware"

	"github.com/chiabcc/panya-charge-oss/internal/domain/proxy"
)

type fakeRelay struct {
	mu       sync.Mutex
	forwards []forwardCall
	err      error
}

type forwardCall struct {
	chargerID string
	action    string
}

func (f *fakeRelay) Forward(_ context.Context, chargerID string, action string, _ any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.forwards = append(f.forwards, forwardCall{chargerID: chargerID, action: action})
	return f.err
}

func (f *fakeRelay) IsConnected(_ string) bool { return true }

func (f *fakeRelay) calls() []forwardCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]forwardCall, len(f.forwards))
	copy(out, f.forwards)
	return out
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestRouter_BootNotification_ForwardsUpstream(t *testing.T) {
	relay := &fakeRelay{}
	r := NewRouter(proxy.DefaultPolicy(), relay, testLogger())

	d := r.RouteBootNotification("CP-1", &core.BootNotificationRequest{
		ChargePointVendor: "ABB",
		ChargePointModel:  "Terra AC",
	})

	if d != proxy.DecisionBoth {
		t.Errorf("BootNotification decision = %s, want both", d)
	}
	calls := relay.calls()
	if len(calls) != 1 {
		t.Fatalf("relay forwards = %d, want 1", len(calls))
	}
	if calls[0].action != proxy.ActionBootNotification {
		t.Errorf("forward action = %q, want %q", calls[0].action, proxy.ActionBootNotification)
	}
}

func TestRouter_Authorize_ForwardsOnly(t *testing.T) {
	relay := &fakeRelay{}
	r := NewRouter(proxy.DefaultPolicy(), relay, testLogger())

	d := r.RouteAuthorize("CP-1", &core.AuthorizeRequest{IdTag: "TAG1"})

	if d != proxy.DecisionUpstreamOnly {
		t.Errorf("Authorize decision = %s, want upstream_only", d)
	}
	calls := relay.calls()
	if len(calls) != 1 {
		t.Fatalf("relay forwards = %d, want 1", len(calls))
	}
}

func TestRouter_Heartbeat_ForwardsUpstream(t *testing.T) {
	relay := &fakeRelay{}
	r := NewRouter(proxy.DefaultPolicy(), relay, testLogger())

	d := r.RouteHeartbeat("CP-1", &core.HeartbeatRequest{})

	if d != proxy.DecisionBoth {
		t.Errorf("Heartbeat decision = %s, want both", d)
	}
	if len(relay.calls()) != 1 {
		t.Fatalf("relay forwards = %d, want 1", len(relay.calls()))
	}
}

func TestRouter_SetChargingProfile_NeverForwards(t *testing.T) {
	relay := &fakeRelay{}
	r := NewRouter(proxy.DefaultPolicy(), relay, testLogger())

	d := r.RouteSetChargingProfile("CP-1", nil)

	if d != proxy.DecisionLocalOnly {
		t.Errorf("SetChargingProfile decision = %s, want local_only", d)
	}
	if len(relay.calls()) != 0 {
		t.Fatalf("SetChargingProfile forwarded %d times, want 0 (NEVER upstream)", len(relay.calls()))
	}
}

func TestRouter_ClearChargingProfile_NeverForwards(t *testing.T) {
	relay := &fakeRelay{}
	r := NewRouter(proxy.DefaultPolicy(), relay, testLogger())

	d := r.RouteClearChargingProfile("CP-1", nil)

	if d != proxy.DecisionLocalOnly {
		t.Errorf("ClearChargingProfile decision = %s, want local_only", d)
	}
	if len(relay.calls()) != 0 {
		t.Fatalf("ClearChargingProfile forwarded %d times, want 0", len(relay.calls()))
	}
}

func TestRouter_FirmwareStatus_ForwardsUpstream(t *testing.T) {
	relay := &fakeRelay{}
	r := NewRouter(proxy.DefaultPolicy(), relay, testLogger())

	d := r.RouteFirmwareStatusNotification("CP-1", &firmware.FirmwareStatusNotificationRequest{
		Status: firmware.FirmwareStatusDownloaded,
	})

	if d != proxy.DecisionUpstreamOnly {
		t.Errorf("FirmwareStatusNotification decision = %s, want upstream_only", d)
	}
	if len(relay.calls()) != 1 {
		t.Fatalf("relay forwards = %d, want 1", len(relay.calls()))
	}
}

func TestRouter_NoopRelay_NeverConnects(t *testing.T) {
	n := NewNoopRelay(testLogger())
	if n.IsConnected("any") {
		t.Error("NoopRelay.IsConnected = true, want false")
	}
	if err := n.Forward(context.Background(), "CP-1", "BootNotification", nil); err != nil {
		t.Errorf("NoopRelay.Forward err = %v, want nil", err)
	}
}

func TestRouter_StartTransaction_Forwards(t *testing.T) {
	relay := &fakeRelay{}
	r := NewRouter(proxy.DefaultPolicy(), relay, testLogger())

	d := r.RouteStartTransaction("CP-1", &core.StartTransactionRequest{
		ConnectorId: 1,
		IdTag:       "TAG1",
		MeterStart:  0,
	})

	if d != proxy.DecisionBoth {
		t.Errorf("StartTransaction decision = %s, want both", d)
	}
	calls := relay.calls()
	if len(calls) != 1 {
		t.Fatalf("relay forwards = %d, want 1", len(calls))
	}
	if calls[0].chargerID != "CP-1" {
		t.Errorf("forward chargerID = %q, want CP-1", calls[0].chargerID)
	}
}

func TestRouter_StopTransaction_Forwards(t *testing.T) {
	relay := &fakeRelay{}
	r := NewRouter(proxy.DefaultPolicy(), relay, testLogger())

	d := r.RouteStopTransaction("CP-1", &core.StopTransactionRequest{
		TransactionId: 42,
		MeterStop:     1000,
	})

	if d != proxy.DecisionBoth {
		t.Errorf("StopTransaction decision = %s, want both", d)
	}
	if len(relay.calls()) != 1 {
		t.Fatalf("relay forwards = %d, want 1", len(relay.calls()))
	}
}

func TestRouter_RemoteStartTransaction_DecisionBoth(t *testing.T) {
	relay := &fakeRelay{}
	r := NewRouter(proxy.DefaultPolicy(), relay, testLogger())

	d := r.RouteRemoteStartTransaction("CP-1", &core.RemoteStartTransactionRequest{
		IdTag: "TAG1",
	})

	if d != proxy.DecisionBoth {
		t.Errorf("RemoteStartTransaction decision = %s, want both", d)
	}
}
