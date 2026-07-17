package csms

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewEmitter_DefaultBufferSize(t *testing.T) {
	e := NewEmitter(0, nil)
	assert.Equal(t, DefaultEventBufferSize, e.BufferSize())
}

func TestNewEmitter_CustomBufferSize(t *testing.T) {
	e := NewEmitter(42, nil)
	assert.Equal(t, 42, e.BufferSize())
}

func TestNewEmitter_NilLogger(t *testing.T) {
	e := NewEmitter(0, nil)
	assert.NotNil(t, e)
}

func TestEmitter_SubscribeAndEmit(t *testing.T) {
	e := NewEmitter(16, nil)
	ch := e.Subscribe()

	e.Emit(StatusChanged{ChargerID: "C1", Status: "Charging"})

	select {
	case ev := <-ch:
		sc, ok := ev.(StatusChanged)
		require.True(t, ok)
		assert.Equal(t, "C1", sc.ChargerID)
		assert.Equal(t, "Charging", sc.Status)
	default:
		t.Fatal("expected to receive event")
	}
}

func TestEmitter_MultipleSubscribers(t *testing.T) {
	e := NewEmitter(16, nil)
	ch1 := e.Subscribe()
	ch2 := e.Subscribe()

	e.Emit(ChargerConnected{ChargerID: "C1"})

	for i, ch := range []<-chan Event{ch1, ch2} {
		select {
		case ev := <-ch:
			cc, ok := ev.(ChargerConnected)
			require.True(t, ok, "subscriber %d", i)
			assert.Equal(t, "C1", cc.ChargerID)
		default:
			t.Fatalf("subscriber %d did not receive event", i)
		}
	}
}

func TestEmitter_OverflowNonBlocking(t *testing.T) {
	e := NewEmitter(2, nil)
	ch := e.Subscribe()

	e.Emit(StatusChanged{ChargerID: "C1"})
	e.Emit(StatusChanged{ChargerID: "C2"})
	e.Emit(StatusChanged{ChargerID: "C3"})

	received := 0
drain:
	for {
		select {
		case <-ch:
			received++
		default:
			break drain
		}
	}
	assert.LessOrEqual(t, received, 2, "buffer can hold at most 2")
}

func TestEmitter_AllEventTypes(t *testing.T) {
	events := []Event{
		ChargerConnected{ChargerID: "C1"},
		ChargerDisconnected{ChargerID: "C1"},
		TransactionStarted{TxID: 1, ChargerID: "C1"},
		TransactionStopped{TxID: 1, ChargerID: "C1"},
		MeterValue{TxID: 1, ChargerID: "C1"},
		StatusChanged{ChargerID: "C1"},
		ChargingProfileUpdated{ChargerID: "C1"},
	}

	e := NewEmitter(len(events), nil)
	ch := e.Subscribe()

	for _, ev := range events {
		e.Emit(ev)
	}

	for i := 0; i < len(events); i++ {
		select {
		case <-ch:
		default:
			t.Fatalf("event %d not received", i)
		}
	}
}

func TestEmitter_ConcurrentEmitAndSubscribe(t *testing.T) {
	e := NewEmitter(256, nil)
	var wg sync.WaitGroup

	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			e.Emit(StatusChanged{ChargerID: "C1"})
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			e.Subscribe()
		}
	}()
	wg.Wait()
}
