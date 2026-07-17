// Package csms defines the contract that a cloud-wrapping GoCSMS
// implementation must satisfy.  The Facade interface exposes charger
// management, event subscription, and lifecycle control without
// leaking internal details.
package csms

import "context"

// Facade defines the contract that a cloud-wrapping GoCSMS must implement.
// It exposes charger management, event subscription, and lifecycle control.
type Facade interface {
	// Start begins the OCPP server and MQTT client lifecycle.
	// Blocks until a fatal error or context cancellation.
	Start(ctx context.Context) error

	// Stop gracefully shuts down all OCPP sessions, the MQTT client,
	// and background goroutines.
	Stop()

	// Subscribe returns a receive-only channel of CSMS domain events.
	// The channel is buffered to the given capacity (default 512).
	// Overflow events are logged.  Returns nil if the Facade has not
	// been started yet.
	Subscribe(ctx context.Context, buffer int) <-chan Event

	// Chargers returns a point-in-time snapshot of all registered chargers.
	// The returned slice is safe for concurrent read (defensive copy).
	Chargers() []ChargerInfo
}