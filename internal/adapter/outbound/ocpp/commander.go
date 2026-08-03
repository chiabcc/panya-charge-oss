package ocpp

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/xBlaz3kx/ocpp-go/ocpp1.6/core"
	"github.com/xBlaz3kx/ocpp-go/ocpp1.6/smartcharging"
	"github.com/xBlaz3kx/ocpp-go/ocpp1.6/types"

	"github.com/chiabcc/panya-charge-oss/internal/domain/ports"
)

// chargingProfileSender is a narrow interface covering the OCPP commands
// the Commander actually invokes, avoiding the need to stub ~40 methods
// from the full ocpp16.CentralSystem in tests.
type chargingProfileSender interface {
	SetChargingProfile(clientID string, callback func(*smartcharging.SetChargingProfileConfirmation, error), connectorID int, chargingProfile *types.ChargingProfile, props ...func(request *smartcharging.SetChargingProfileRequest)) error
	RemoteStartTransaction(clientID string, callback func(*core.RemoteStartTransactionConfirmation, error), idTag string, props ...func(request *core.RemoteStartTransactionRequest)) error
	RemoteStopTransaction(clientID string, callback func(*core.RemoteStopTransactionConfirmation, error), transactionID int, props ...func(request *core.RemoteStopTransactionRequest)) error
	ClearChargingProfile(clientID string, callback func(*smartcharging.ClearChargingProfileConfirmation, error), props ...func(request *smartcharging.ClearChargingProfileRequest)) error
}

const commandTimeout = 15 * time.Second

const defaultContactorCooldown = 180 * time.Second

type Commander struct {
	cs                chargingProfileSender
	logger            *slog.Logger
	sessionRepo       ports.SessionRepository
	mu                sync.Mutex
	contactorCooldown time.Duration
	lastStartStop     map[string]time.Time
}

func NewCommander(cs chargingProfileSender, sessionRepo ports.SessionRepository, logger *slog.Logger) *Commander {
	return &Commander{
		cs:                cs,
		logger:            logger,
		sessionRepo:       sessionRepo,
		contactorCooldown: defaultContactorCooldown,
		lastStartStop:     make(map[string]time.Time),
	}
}

// SetCooldown updates the contactor protection cooldown duration.
func (c *Commander) SetCooldown(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.contactorCooldown = d
}

func (c *Commander) enforceCooldown(chargerID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	last, ok := c.lastStartStop[chargerID]
	if ok && time.Since(last) < c.contactorCooldown {
		remaining := c.contactorCooldown - time.Since(last)
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
	if err := c.sendChargingProfile(chargerID, connectorID, buildTxDefaultProfile(connectorID, limitAmps)); err != nil {
		return err
	}

	if connectorID <= 0 || c.sessionRepo == nil {
		return nil
	}

	session, err := c.sessionRepo.GetActiveSession(context.Background(), chargerID, connectorID)
	if err != nil || session == nil {
		return nil
	}

	txProfile := buildTxProfile(limitAmps, session.TransactionID)
	if sendErr := c.sendChargingProfile(chargerID, connectorID, txProfile); sendErr != nil {
		c.logger.Warn("txProfile not applied — charger may not support modifying live transactions",
			"charger", chargerID, "connector", connectorID, "transactionId", session.TransactionID, "err", sendErr)
	}

	return nil
}

func (c *Commander) sendChargingProfile(chargerID string, connectorID int, profile *types.ChargingProfile) error {
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
