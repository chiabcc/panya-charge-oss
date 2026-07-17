package ports

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chiabcc/panya-charge-oss/internal/domain/charger"
	"github.com/chiabcc/panya-charge-oss/internal/domain/proxy"
	"github.com/chiabcc/panya-charge-oss/internal/domain/session"
)

var ctx = context.Background()

func TestInMemoryChargerRepository_UpsertAndUpsertCharger(t *testing.T) {
	repo := NewInMemoryChargerRepository()
	c := charger.Charger{ID: "ABB-001", Vendor: "ABB", Model: "Terra AC"}

	err := repo.UpsertCharger(ctx, c)
	require.NoError(t, err)

	got, err := repo.GetCharger(ctx, "ABB-001")
	require.NoError(t, err)
	assert.Equal(t, "ABB", got.Vendor)

	c2 := charger.Charger{ID: "ABB-001", Vendor: "ABB", Model: "Terra AC v2"}
	require.NoError(t, repo.UpsertCharger(ctx, c2))
	got2, err := repo.GetCharger(ctx, "ABB-001")
	require.NoError(t, err)
	assert.Equal(t, "Terra AC v2", got2.Model)
}

func TestInMemoryChargerRepository_GetCharger_NotFound(t *testing.T) {
	repo := NewInMemoryChargerRepository()
	_, err := repo.GetCharger(ctx, "nope")
	assert.Error(t, err)
}

func TestInMemoryChargerRepository_ListChargers(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		repo := NewInMemoryChargerRepository()
		list, err := repo.ListChargers(ctx)
		require.NoError(t, err)
		assert.Empty(t, list)
	})

	t.Run("multiple", func(t *testing.T) {
		repo := NewInMemoryChargerRepository()
		for i := 0; i < 3; i++ {
			require.NoError(t, repo.UpsertCharger(ctx, charger.Charger{ID: fmt.Sprintf("C-%d", i)}))
		}
		list, err := repo.ListChargers(ctx)
		require.NoError(t, err)
		assert.Len(t, list, 3)
	})
}

func TestInMemoryChargerRepository_MarkOnline(t *testing.T) {
	repo := NewInMemoryChargerRepository()
	require.NoError(t, repo.UpsertCharger(ctx, charger.Charger{ID: "C1"}))

	require.NoError(t, repo.MarkOnline(ctx, "C1", true))
	got, _ := repo.GetCharger(ctx, "C1")
	assert.True(t, got.Online)

	require.NoError(t, repo.MarkOnline(ctx, "C1", false))
	got, _ = repo.GetCharger(ctx, "C1")
	assert.False(t, got.Online)

	err := repo.MarkOnline(ctx, "missing", true)
	assert.Error(t, err)
}

func TestInMemoryChargerRepository_Connectors(t *testing.T) {
	repo := NewInMemoryChargerRepository()

	conn := charger.Connector{ChargerID: "C1", ConnectorID: 1, Status: charger.StatusAvailable}
	require.NoError(t, repo.UpsertConnector(ctx, conn))

	got, err := repo.GetConnector(ctx, "C1", 1)
	require.NoError(t, err)
	assert.Equal(t, charger.StatusAvailable, got.Status)

	conn2 := charger.Connector{ChargerID: "C1", ConnectorID: 1, Status: charger.StatusCharging}
	require.NoError(t, repo.UpsertConnector(ctx, conn2))
	got, _ = repo.GetConnector(ctx, "C1", 1)
	assert.Equal(t, charger.StatusCharging, got.Status)

	conn3 := charger.Connector{ChargerID: "C1", ConnectorID: 2, Status: charger.StatusFaulted}
	require.NoError(t, repo.UpsertConnector(ctx, conn3))
	list, err := repo.ListConnectors(ctx, "C1")
	require.NoError(t, err)
	assert.Len(t, list, 2)
}

func TestInMemoryChargerRepository_GetConnector_NotFound(t *testing.T) {
	repo := NewInMemoryChargerRepository()
	_, err := repo.GetConnector(ctx, "C1", 1)
	assert.Error(t, err)
}

func TestInMemoryChargerRepository_ListConnectors_Empty(t *testing.T) {
	repo := NewInMemoryChargerRepository()
	list, err := repo.ListConnectors(ctx, "C1")
	require.NoError(t, err)
	assert.Empty(t, list)
}

func TestInMemorySessionRepository_CreateAndUpdate(t *testing.T) {
	repo := NewInMemorySessionRepository()

	s := session.Session{ID: "sess-1", TransactionID: 1, ChargerID: "C1"}
	require.NoError(t, repo.CreateSession(ctx, s))

	got, err := repo.GetSession(ctx, "sess-1")
	require.NoError(t, err)
	assert.Equal(t, 1, got.TransactionID)

	now := time.Now()
	got.StoppedAt = &now
	require.NoError(t, repo.UpdateSession(ctx, *got))

	updated, _ := repo.GetSession(ctx, "sess-1")
	assert.NotNil(t, updated.StoppedAt)
}

func TestInMemorySessionRepository_UpdateSession_NotFound(t *testing.T) {
	repo := NewInMemorySessionRepository()
	err := repo.UpdateSession(ctx, session.Session{ID: "ghost"})
	assert.Error(t, err)
}

func TestInMemorySessionRepository_GetActiveSession(t *testing.T) {
	repo := NewInMemorySessionRepository()

	now := time.Now()
	repo.CreateSession(ctx, session.Session{
		ID: "completed", ChargerID: "C1", ConnectorID: 1, StoppedAt: &now,
	})
	repo.CreateSession(ctx, session.Session{
		ID: "active", TransactionID: 5, ChargerID: "C1", ConnectorID: 1,
	})

	t.Run("active found", func(t *testing.T) {
		got, err := repo.GetActiveSession(ctx, "C1", 1)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, 5, got.TransactionID)
	})

	t.Run("none active", func(t *testing.T) {
		got, err := repo.GetActiveSession(ctx, "C2", 1)
		require.NoError(t, err)
		assert.Nil(t, got)
	})
}

func TestInMemorySessionRepository_GetSessionByTransactionID(t *testing.T) {
	repo := NewInMemorySessionRepository()
	repo.CreateSession(ctx, session.Session{ID: "s1", TransactionID: 10, ChargerID: "C1"})

	got, err := repo.GetSessionByTransactionID(ctx, "C1", 10)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "s1", got.ID)

	got, err = repo.GetSessionByTransactionID(ctx, "C1", 99)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestInMemorySessionRepository_GetSession_NotFound(t *testing.T) {
	repo := NewInMemorySessionRepository()
	_, err := repo.GetSession(ctx, "ghost")
	assert.Error(t, err)
}

func TestInMemorySessionRepository_ListSessions(t *testing.T) {
	repo := NewInMemorySessionRepository()
	for i := 0; i < 5; i++ {
		require.NoError(t, repo.CreateSession(ctx, session.Session{
			ID: fmt.Sprintf("s%d", i), ChargerID: "C1",
		}))
	}

	t.Run("limit 2 offset 0", func(t *testing.T) {
		list, err := repo.ListSessions(ctx, 2, 0)
		require.NoError(t, err)
		assert.Len(t, list, 2)
	})

	t.Run("offset beyond range", func(t *testing.T) {
		list, err := repo.ListSessions(ctx, 10, 100)
		require.NoError(t, err)
		assert.Empty(t, list)
	})

	t.Run("limit exceeds total", func(t *testing.T) {
		list, err := repo.ListSessions(ctx, 100, 0)
		require.NoError(t, err)
		assert.Len(t, list, 5)
	})
}

func TestInMemoryMeterRepository_CRUD(t *testing.T) {
	repo := NewInMemoryMeterRepository()

	t0 := time.Now()
	t1 := t0.Add(time.Minute)
	t2 := t0.Add(2 * time.Minute)

	mv1 := MeterValue{ChargerID: "C1", Timestamp: t0, Value: 100}
	mv2 := MeterValue{ChargerID: "C1", Timestamp: t1, Value: 200}
	mv3 := MeterValue{ChargerID: "C2", Timestamp: t1, Value: 300}

	require.NoError(t, repo.StoreMeterValue(ctx, mv1))
	require.NoError(t, repo.StoreMeterValues(ctx, []MeterValue{mv2, mv3}))

	got, err := repo.GetMeterValues(ctx, "C1", t0, t1)
	require.NoError(t, err)
	assert.Len(t, got, 2)

	got, err = repo.GetMeterValues(ctx, "C2", t0, t2)
	require.NoError(t, err)
	assert.Len(t, got, 1)
}

func TestInMemoryMeterRepository_GetMeterValuesBySession(t *testing.T) {
	repo := NewInMemoryMeterRepository()
	require.NoError(t, repo.StoreMeterValue(ctx, MeterValue{SessionID: "s1", Value: 100}))
	require.NoError(t, repo.StoreMeterValue(ctx, MeterValue{SessionID: "s1", Value: 200}))
	require.NoError(t, repo.StoreMeterValue(ctx, MeterValue{SessionID: "s2", Value: 300}))

	got, err := repo.GetMeterValuesBySession(ctx, "s1")
	require.NoError(t, err)
	assert.Len(t, got, 2)
}

func TestInMemoryMeterRepository_PurgeOlderThan(t *testing.T) {
	repo := NewInMemoryMeterRepository()
	t0 := time.Now()
	old := t0.Add(-2 * time.Hour)
	recent := t0.Add(-5 * time.Minute)

	require.NoError(t, repo.StoreMeterValue(ctx, MeterValue{Timestamp: old, ChargerID: "C1"}))
	require.NoError(t, repo.StoreMeterValue(ctx, MeterValue{Timestamp: recent, ChargerID: "C1"}))
	require.NoError(t, repo.StoreMeterValue(ctx, MeterValue{Timestamp: t0, ChargerID: "C1"}))

	cutoff := t0.Add(-1 * time.Hour)
	purged, err := repo.PurgeOlderThan(ctx, cutoff)
	require.NoError(t, err)
	assert.Equal(t, int64(1), purged)

	all, _ := repo.GetMeterValues(ctx, "C1", time.Time{}, t0.Add(time.Minute))
	assert.Len(t, all, 2)
}

func TestInMemoryProxyConfigRepository(t *testing.T) {
	repo := NewInMemoryProxyConfigRepository()

	got, err := repo.GetProxyConfig(ctx, "C1")
	require.NoError(t, err)
	assert.Nil(t, got)

	cfg := proxy.ProxyConfig{ChargerID: "C1", ProxyEnabled: true}
	require.NoError(t, repo.UpsertProxyConfig(ctx, cfg))

	got, err = repo.GetProxyConfig(ctx, "C1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.True(t, got.ProxyEnabled)
}

func TestInMemoryChargerRepository_Concurrent(t *testing.T) {
	repo := NewInMemoryChargerRepository()
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(2)
		id := fmt.Sprintf("C-%d", i)
		go func() {
			defer wg.Done()
			repo.UpsertCharger(ctx, charger.Charger{ID: id})
		}()
		go func() {
			defer wg.Done()
			repo.ListChargers(ctx)
		}()
	}
	wg.Wait()

	list, _ := repo.ListChargers(ctx)
	assert.Len(t, list, 100)
}
