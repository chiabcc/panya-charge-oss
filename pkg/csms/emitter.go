package csms

import (
	"log/slog"
	"sync"
)

// DefaultEventBufferSize is the per-subscriber channel capacity used when no
// explicit buffer size is provided. 512 covers bursty MeterValues traffic
// (a charger reporting every few seconds) without blocking OCPP handlers.
const DefaultEventBufferSize = 512

// Emitter is a thread-safe multicast event channel manager. Subscribers each
// receive events on their own buffered channel. Emission is always
// non-blocking: when a subscriber's buffer is full the event is dropped and a
// warning logged, so a slow consumer never stalls the OCPP hot path.
type Emitter struct {
	subscribers []chan Event
	mu          sync.RWMutex
	bufSize     int
	logger      *slog.Logger
}

// NewEmitter returns an Emitter whose subscriber channels are buffered to
// bufSize. A non-positive bufSize falls back to DefaultEventBufferSize.
// A nil logger falls back to slog.Default().
func NewEmitter(bufSize int, logger *slog.Logger) *Emitter {
	if bufSize <= 0 {
		bufSize = DefaultEventBufferSize
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Emitter{
		bufSize: bufSize,
		logger:  logger,
	}
}

// BufferSize returns the per-subscriber channel capacity.
func (e *Emitter) BufferSize() int { return e.bufSize }

// Subscribe registers a new subscriber and returns a receive-only channel
// buffered to the emitter's capacity (default 512). The caller drains the
// channel; overflow is logged, never blocking the producer.
func (e *Emitter) Subscribe() <-chan Event {
	e.mu.Lock()
	defer e.mu.Unlock()
	ch := make(chan Event, e.bufSize)
	e.subscribers = append(e.subscribers, ch)
	return ch
}

// Emit broadcasts ev to every subscriber. The send is non-blocking: if a
// subscriber's buffer is full the event for that subscriber is dropped and a
// warning logged. Safe for concurrent use.
func (e *Emitter) Emit(ev Event) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	for _, ch := range e.subscribers {
		select {
		case ch <- ev:
		default:
			e.logger.Warn("event channel full — dropping event",
				"buffer_size", e.bufSize,
				"subscribers", len(e.subscribers),
			)
		}
	}
}
