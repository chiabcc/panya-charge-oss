package ocpp

import (
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"
)

func TestCommander_SetCooldown_RaceFree(t *testing.T) {
	cmd := NewCommander(nil, slog.New(slog.NewTextHandler(io.Discard, nil)))

	var wg sync.WaitGroup
	cmd.mu.Lock()
	cmd.lastStartStop["CHG-001"] = time.Now().Add(-100 * time.Second)
	cmd.mu.Unlock()

	const iters = 200

	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < iters; i++ {
			_ = cmd.enforceCooldown("CHG-001")
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < iters; i++ {
			cmd.SetCooldown(time.Duration(180+i%60) * time.Second)
		}
	}()

	wg.Wait()
}

func TestCommander_SetCooldown_Applied(t *testing.T) {
	cmd := NewCommander(nil, slog.New(slog.NewTextHandler(io.Discard, nil)))

	cmd.mu.Lock()
	cmd.lastStartStop["CHG-001"] = time.Now().Add(-200 * time.Second)
	cmd.mu.Unlock()

	cmd.SetCooldown(300 * time.Second)

	err := cmd.enforceCooldown("CHG-001")
	if err == nil {
		t.Error("enforceCooldown() = nil, want error (200s < 300s cooldown)")
	}

	cmd.SetCooldown(100 * time.Second)

	err = cmd.enforceCooldown("CHG-001")
	if err != nil {
		t.Errorf("enforceCooldown() = %v, want nil (200s > 100s cooldown)", err)
	}
}