package audit

import (
	"context"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/jordigilh/kubernaut-apifrontend/internal/requestid"
	"github.com/jordigilh/kubernaut-apifrontend/internal/security"
)

// BufferConfig configures the BufferedEmitter.
type BufferConfig struct {
	Writer          Writer
	Logger          logr.Logger
	BufferSize      int
	FlushInterval   time.Duration
	BatchSize       int
	OverflowCounter prometheus.Counter
	EventsCounter   *prometheus.CounterVec
}

// BufferedEmitter buffers audit events and flushes them asynchronously
// to a durable backend (Writer). Fire-and-forget per ADR-038.
type BufferedEmitter struct {
	buffer          chan *Event
	writer          Writer
	logger          logr.Logger
	flushInterval   time.Duration
	batchSize       int
	overflowCounter prometheus.Counter
	eventsCounter   *prometheus.CounterVec
	done            chan struct{}
	wg              sync.WaitGroup
}

// NewBufferedEmitter creates a BufferedEmitter. Call Start() to launch the
// background flush loop. Separating construction from goroutine launch enables
// deterministic testing of buffer overflow behavior.
func NewBufferedEmitter(cfg BufferConfig) *BufferedEmitter { //nolint:gocritic // hugeParam: called once at startup
	bufSize := cfg.BufferSize
	if bufSize <= 0 {
		bufSize = 4096
	}
	flushInterval := cfg.FlushInterval
	if flushInterval <= 0 {
		flushInterval = 5 * time.Second
	}
	batchSize := cfg.BatchSize
	if batchSize <= 0 {
		batchSize = 100
	}

	return &BufferedEmitter{
		buffer:          make(chan *Event, bufSize),
		writer:          cfg.Writer,
		logger:          cfg.Logger.WithName("audit-buffer"),
		flushInterval:   flushInterval,
		batchSize:       batchSize,
		overflowCounter: cfg.OverflowCounter,
		eventsCounter:   cfg.EventsCounter,
		done:            make(chan struct{}),
	}
}

// Start launches the background flush loop. Must be called exactly once.
func (e *BufferedEmitter) Start() {
	e.wg.Add(1)
	go e.flushLoop()
}

// Emit sanitizes and buffers an audit event. Non-blocking; drops on overflow.
// The event is shallow-cloned internally; callers may safely reuse the pointer
// after Emit returns.
func (e *BufferedEmitter) Emit(ctx context.Context, event *Event) {
	ev := *event
	ev.Timestamp = time.Now()
	if ev.RequestID == "" {
		ev.RequestID = requestid.FromContext(ctx)
	}
	ev.Detail = security.RedactMap(ev.Detail)

	select {
	case e.buffer <- &ev:
		if e.eventsCounter != nil {
			e.eventsCounter.WithLabelValues(string(ev.Type)).Inc()
		}
	default:
		if e.overflowCounter != nil {
			e.overflowCounter.Inc()
		}
		e.logger.Error(nil, "audit buffer overflow, event dropped",
			"event_type", string(ev.Type),
			"request_id", ev.RequestID,
		)
	}
}

// EmitBlocking sanitizes and buffers a security-critical audit event with
// backpressure. Unlike Emit, it blocks (up to context deadline) rather than
// dropping the event if the buffer is full. Use for authentication failures,
// RBAC denials, and other FedRAMP-required security events.
func (e *BufferedEmitter) EmitBlocking(ctx context.Context, event *Event) {
	ev := *event
	ev.Timestamp = time.Now()
	if ev.RequestID == "" {
		ev.RequestID = requestid.FromContext(ctx)
	}
	ev.Detail = security.RedactMap(ev.Detail)

	select {
	case e.buffer <- &ev:
		if e.eventsCounter != nil {
			e.eventsCounter.WithLabelValues(string(ev.Type)).Inc()
		}
	case <-ctx.Done():
		if e.overflowCounter != nil {
			e.overflowCounter.Inc()
		}
		e.logger.Error(nil, "audit buffer full, critical event lost due to context deadline",
			"event_type", string(ev.Type),
			"request_id", ev.RequestID,
		)
	}
}

// Close stops accepting new events, drains the buffer, and flushes remaining
// events to the writer. If the writer fails, remaining events are logged.
// The context deadline bounds total close time.
//
// If Close returns ctx.Err() due to timeout, the flushLoop goroutine continues
// running until it finishes draining the bounded buffer (max 4096 events). This
// is a bounded goroutine leak — it completes once all buffered events are flushed
// or logged. The writer may be called after the caller's shutdown sequence completes.
func (e *BufferedEmitter) Close(ctx context.Context) error {
	close(e.done)
	done := make(chan struct{})
	go func() {
		e.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (e *BufferedEmitter) flushLoop() {
	defer e.wg.Done()
	ticker := time.NewTicker(e.flushInterval)
	defer ticker.Stop()

	batch := make([]*Event, 0, e.batchSize)

	for {
		select {
		case <-e.done:
			// Drain remaining buffered events and flush
			for {
				select {
				case evt := <-e.buffer:
					batch = append(batch, evt)
				default:
					if len(batch) > 0 {
						e.flushBatchBackground(batch)
					}
					return
				}
			}
		case evt := <-e.buffer:
			batch = append(batch, evt)
			if len(batch) >= e.batchSize {
				e.flushBatchBackground(batch)
				batch = make([]*Event, 0, e.batchSize)
			}
		case <-ticker.C:
			if len(batch) > 0 {
				e.flushBatchBackground(batch)
				batch = make([]*Event, 0, e.batchSize)
			}
		}
	}
}

func (e *BufferedEmitter) flushBatchBackground(batch []*Event) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := e.flushBatch(ctx, batch); err != nil {
		e.logger.Error(err, "audit batch flush failed, logging events",
			"count", len(batch),
		)
		for _, evt := range batch {
			e.logEvent(evt)
		}
	}
}

func (e *BufferedEmitter) flushBatch(ctx context.Context, batch []*Event) error {
	if e.writer == nil || len(batch) == 0 {
		return nil
	}
	return e.writer.WriteAuditEvents(ctx, batch)
}

func (e *BufferedEmitter) logEvent(evt *Event) {
	e.logger.Info("unflushed audit event",
		"event_type", string(evt.Type),
		"user_id", evt.UserID,
		"request_id", evt.RequestID,
		"timestamp", evt.Timestamp.Format(time.RFC3339Nano),
	)
}

// Compile-time interface check.
var _ ClosableEmitter = (*BufferedEmitter)(nil)
