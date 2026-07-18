package ocpp

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/xBlaz3kx/ocpp-go/ocpp1.6/core"
	"github.com/xBlaz3kx/ocpp-go/ocpp1.6/types"

	"github.com/chiabcc/panya-charge-oss/internal/domain/proxy"
)

func TestRelay_ThrottleDropsMeterValues(t *testing.T) {
	ctx := context.Background()
	repo := newFakeConfigRepo()

	repo.set("CP-1", &proxy.ProxyConfig{
		ChargerID:                "CP-1",
		ProxyEnabled:             true,
		UpstreamURL:              "ws://localhost:19999",
		UpstreamThrottleMVPerMin: 60,
	})

	relay := NewUpstreamRelay(repo, discardedLogger())
	cp := &stubCP{}
	injectStub(relay, "CP-1", cp)

	mvReq := &core.MeterValuesRequest{
		ConnectorId: 1,
		MeterValue: []types.MeterValue{
			{Timestamp: types.NewDateTime(time.Now()), SampledValue: []types.SampledValue{{Value: "5000", Measurand: types.MeasurandPowerActiveImport}}},
		},
	}

	if err := relay.Forward(ctx, "CP-1", proxy.ActionMeterValues, mvReq); err != nil {
		t.Fatalf("First Forward MeterValues: %v", err)
	}
	cp.mu.Lock()
	meterAfterFirst := cp.meterCalls
	cp.mu.Unlock()
	if meterAfterFirst != 1 {
		t.Errorf("meterCalls after first = %d, want 1", meterAfterFirst)
	}

	if err := relay.Forward(ctx, "CP-1", proxy.ActionMeterValues, mvReq); err != nil {
		t.Fatalf("Second Forward MeterValues (throttled): %v", err)
	}
	cp.mu.Lock()
	meterAfterSecond := cp.meterCalls
	cp.mu.Unlock()
	if meterAfterSecond != 1 {
		t.Errorf("meterCalls after throttled second = %d, want 1", meterAfterSecond)
	}

	time.Sleep(1100 * time.Millisecond)

	if err := relay.Forward(ctx, "CP-1", proxy.ActionMeterValues, mvReq); err != nil {
		t.Fatalf("Third Forward MeterValues after interval: %v", err)
	}
	cp.mu.Lock()
	meterAfterThird := cp.meterCalls
	cp.mu.Unlock()
	if meterAfterThird != 2 {
		t.Errorf("meterCalls after third = %d, want 2", meterAfterThird)
	}
}

func TestRelay_ThrottleDisabled(t *testing.T) {
	ctx := context.Background()
	repo := newFakeConfigRepo()

	repo.set("CP-1", &proxy.ProxyConfig{
		ChargerID:                "CP-1",
		ProxyEnabled:             true,
		UpstreamURL:              "ws://localhost:19999",
		UpstreamThrottleMVPerMin: 0,
	})

	relay := NewUpstreamRelay(repo, discardedLogger())
	cp := &stubCP{}
	injectStub(relay, "CP-1", cp)

	mvReq := &core.MeterValuesRequest{
		ConnectorId: 1,
		MeterValue: []types.MeterValue{
			{Timestamp: types.NewDateTime(time.Now()), SampledValue: []types.SampledValue{{Value: "5000", Measurand: types.MeasurandPowerActiveImport}}},
		},
	}

	for i := 0; i < 5; i++ {
		if err := relay.Forward(ctx, "CP-1", proxy.ActionMeterValues, mvReq); err != nil {
			t.Fatalf("Forward MeterValues %d: %v", i, err)
		}
	}

	cp.mu.Lock()
	calls := cp.meterCalls
	cp.mu.Unlock()
	if calls != 5 {
		t.Errorf("meterCalls = %d, want 5", calls)
	}
}

type testWSUpstream struct {
	listener net.Listener
	upgrader websocket.Upgrader
}

func newTestWSUpstream(t *testing.T) *testWSUpstream {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := &testWSUpstream{
		listener: l,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
	go srv.serve(t)
	return srv
}

func (s *testWSUpstream) URL() string {
	addr := s.listener.Addr().(*net.TCPAddr)
	return fmt.Sprintf("ws://%s/%s", addr.String(), "ws")
}

func (s *testWSUpstream) serve(t *testing.T) {
	t.Helper()
	srv := &http.Server{Handler: s}
	go func() { _ = srv.Serve(s.listener) }()
}

func (s *testWSUpstream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			_ = conn.Close()
			return
		}
	}
}

func (s *testWSUpstream) Close() {
	_ = s.listener.Close()
}

func TestRelay_OnStateChange_ConnectDisconnect(t *testing.T) {
	var (
		stateEventsMu sync.Mutex
		stateEvents   []struct {
			chargerID string
			connected bool
		}
	)
	recordState := func(chargerID string, connected bool) {
		stateEventsMu.Lock()
		defer stateEventsMu.Unlock()
		stateEvents = append(stateEvents, struct {
			chargerID string
			connected bool
		}{chargerID, connected})
	}

	srv := newTestWSUpstream(t)
	defer srv.Close()

	repo := newFakeConfigRepo()
	repo.set("CP-1", &proxy.ProxyConfig{
		ChargerID:    "CP-1",
		ProxyEnabled: true,
		UpstreamURL:  srv.URL(),
	})

	relay := NewUpstreamRelay(repo, discardedLogger())
	relay.OnStateChange = recordState

	if err := relay.Connect(context.Background(), "CP-1"); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	stateEventsMu.Lock()
	connectEvents := filterEvents(stateEvents, "CP-1", true)
	stateEventsMu.Unlock()
	if len(connectEvents) == 0 {
		t.Error("expected OnStateChange(chargerID, true) after Connect, got none")
	}

	relay.Disconnect("CP-1")

	stateEventsMu.Lock()
	disconnectEvents := filterEvents(stateEvents, "CP-1", false)
	stateEventsMu.Unlock()
	if len(disconnectEvents) == 0 {
		t.Error("expected OnStateChange(chargerID, false) after Disconnect, got none")
	}
}

func TestRelay_OnStateChange_NilSafe(t *testing.T) {
	repo := newFakeConfigRepo()
	relay := NewUpstreamRelay(repo, discardedLogger())

	cp := &stubCP{}
	injectStub(relay, "CP-1", cp)

	relay.Disconnect("CP-1")
	_ = relay.Connect(context.Background(), "CP-NONE")
}

func filterEvents(events []struct {
	chargerID string
	connected bool
}, id string, connected bool) []struct {
	chargerID string
	connected bool
} {
	var filtered []struct {
		chargerID string
		connected bool
	}
	for _, e := range events {
		if e.chargerID == id && e.connected == connected {
			filtered = append(filtered, e)
		}
	}
	return filtered
}

