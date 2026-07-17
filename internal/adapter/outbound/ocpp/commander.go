package ocpp

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/xBlaz3kx/ocpp-go/ocpp1.6"
	"github.com/xBlaz3kx/ocpp-go/ocpp1.6/core"
	"github.com/xBlaz3kx/ocpp-go/ocpp1.6/smartcharging"
	"github.com/xBlaz3kx/ocpp-go/ocpp1.6/types"
)

const (
	commandTimeout    = 15 * time.Second
	contactorCooldown = 180 * time.Second
)

type Commander struct {
	cs            ocpp16.CentralSystem
	logger        *slog.Logger
	mu            sync.Mutex
	lastStartStop map[string]time.Time
}

func NewCommander(cs ocpp16.CentralSystem, logger *slog.Logger) *Commander {
	return &Commander{
		cs:            cs,
		logger:        logger,
		lastStartStop: make(map[string]time.Time),
	}
}

func (c *Commander) enforceCooldown(chargerID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	last, ok := c.lastStartStop[chargerID]
	if ok && time.Since(last) < contactorCooldown {
		remaining := contactorCooldown - time.Since(last)
		return fmt.Errorf("contactor protection: %s on cooldown for %s", chargerID, remaining.Round(time.Second))
	}
	return nil
}

func (c *Commander) markStartStop(chargerID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastStartStop[chargerID] = time.Now()
}

func (c *Commander) SetChargingProfile(chargerID string, connectorID, limitAmps int) error {
	profile := buildTxDefaultProfile(connectorID, limitAmps)
	errCh := make(chan error, 1)

	err := c.cs.SetChargingProfile(chargerID, func(conf *smartcharging.SetChargingProfileConfirmation, err error) {
		if err != nil {
			errCh <- fmt.Errorf("set charging profile: %w", err)
			return
		}
		if conf.Status != smartcharging.ChargingProfileStatusAccepted {
			errCh <- fmt.Errorf("charger rejected profile: %s", conf.Status)
			return
		}
		errCh <- nil
	}, connectorID, profile)

	if err != nil {
		return fmt.Errorf("send set charging profile to %s: %w", chargerID, err)
	}

	select {
	case err := <-errCh:
		return err
	case <-time.After(commandTimeout):
		return fmt.Errorf("set charging profile timeout for %s", chargerID)
	}
}

func (c *Commander) RemoteStartTransaction(chargerID string, connectorID int, idTag string) error {
	if err := c.enforceCooldown(chargerID); err != nil {
		return err
	}

	errCh := make(chan error, 1)

	err := c.cs.RemoteStartTransaction(chargerID, func(conf *core.RemoteStartTransactionConfirmation, err error) {
		if err != nil {
			errCh <- fmt.Errorf("remote start: %w", err)
			return
		}
		if conf.Status != types.RemoteStartStopStatusAccepted {
			errCh <- fmt.Errorf("charger rejected remote start: %s", conf.Status)
			return
		}
		c.markStartStop(chargerID)
		errCh <- nil
	}, idTag, func(req *core.RemoteStartTransactionRequest) {
		req.ConnectorId = &connectorID
	})

	if err != nil {
		return fmt.Errorf("send remote start to %s: %w", chargerID, err)
	}

	select {
	case err := <-errCh:
		return err
	case <-time.After(commandTimeout):
		return fmt.Errorf("remote start timeout for %s", chargerID)
	}
}

func (c *Commander) RemoteStopTransaction(chargerID string, transactionID int) error {
	if err := c.enforceCooldown(chargerID); err != nil {
		return err
	}

	errCh := make(chan error, 1)

	err := c.cs.RemoteStopTransaction(chargerID, func(conf *core.RemoteStopTransactionConfirmation, err error) {
		if err != nil {
			errCh <- fmt.Errorf("remote stop: %w", err)
			return
		}
		if conf.Status != types.RemoteStartStopStatusAccepted {
			errCh <- fmt.Errorf("charger rejected remote stop: %s", conf.Status)
			return
		}
		c.markStartStop(chargerID)
		errCh <- nil
	}, transactionID)

	if err != nil {
		return fmt.Errorf("send remote stop to %s: %w", chargerID, err)
	}

	select {
	case err := <-errCh:
		return err
	case <-time.After(commandTimeout):
		return fmt.Errorf("remote stop timeout for %s", chargerID)
	}
}

func (c *Commander) ClearChargingProfile(chargerID string, connectorID int) error {
	errCh := make(chan error, 1)

	err := c.cs.ClearChargingProfile(chargerID, func(conf *smartcharging.ClearChargingProfileConfirmation, err error) {
		if err != nil {
			errCh <- fmt.Errorf("clear charging profile: %w", err)
			return
		}
		errCh <- nil
	}, func(req *smartcharging.ClearChargingProfileRequest) {
		req.ConnectorId = &connectorID
	})

	if err != nil {
		return fmt.Errorf("send clear charging profile to %s: %w", chargerID, err)
	}

	select {
	case err := <-errCh:
		return err
	case <-time.After(commandTimeout):
		return fmt.Errorf("clear charging profile timeout for %s", chargerID)
	}
}