package streaming

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// TrackedConnection represents a single SSE connection managed by the tracker.
type TrackedConnection struct {
	ID     string
	Writer http.ResponseWriter
	Cancel context.CancelFunc
}

// ConnectionTracker manages active SSE connections for graceful shutdown.
// Thread-safe for concurrent Add/Remove/DrainAll calls.
type ConnectionTracker struct {
	mu        sync.Mutex
	conns     map[string]*TrackedConnection
	gauge     prometheus.Gauge
	drainWait time.Duration
}

// NewConnectionTracker creates a new tracker with an optional Prometheus gauge.
// drainWait is the grace period between sending shutdown frames and force-closing;
// pass 0 for immediate cancellation (useful in tests).
func NewConnectionTracker(gauge prometheus.Gauge, drainWait time.Duration) *ConnectionTracker {
	return &ConnectionTracker{
		conns:     make(map[string]*TrackedConnection),
		gauge:     gauge,
		drainWait: drainWait,
	}
}

// Add registers a new SSE connection.
func (t *ConnectionTracker) Add(conn *TrackedConnection) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.conns[conn.ID] = conn
	if t.gauge != nil {
		t.gauge.Set(float64(len(t.conns)))
	}
}

// Remove deregisters an SSE connection (e.g., on client disconnect).
func (t *ConnectionTracker) Remove(id string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.conns, id)
	if t.gauge != nil {
		t.gauge.Set(float64(len(t.conns)))
	}
}

// Count returns the number of active connections.
func (t *ConnectionTracker) Count() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.conns)
}

// DrainAll sends an SSE shutdown event to all connections, waits for the
// configured drainWait period for clients to self-disconnect, then force-cancels
// remaining connections. Returns the number of connections drained (includes both
// those that self-disconnected and those force-cancelled — the count reflects
// all connections active at drain start, not exclusively forced closures).
//
// Concurrency note (ARCH-9): The shutdown frame write occurs after clearing the
// map but while handler goroutines may still be writing to the same ResponseWriter.
// The drainWait grace period is designed so handlers observe context cancellation
// and stop writing before the frame is sent. No mutex protects the ResponseWriter
// because http.ResponseWriter is not safe for concurrent use and adding one would
// risk deadlock with streaming flushers. If drainWait is too short, a data race on
// the ResponseWriter is theoretically possible but benign (connection is closing).
func (t *ConnectionTracker) DrainAll(ctx context.Context) int {
	t.mu.Lock()
	snapshot := make([]*TrackedConnection, 0, len(t.conns))
	for _, conn := range t.conns {
		snapshot = append(snapshot, conn)
	}
	t.conns = make(map[string]*TrackedConnection)
	if t.gauge != nil {
		t.gauge.Set(0)
	}
	t.mu.Unlock()

	if len(snapshot) == 0 {
		return 0
	}

	shutdownFrame := []byte("event: shutdown\ndata: {\"retry_ms\":5000}\n\n")

	for _, conn := range snapshot {
		if flusher, ok := conn.Writer.(http.Flusher); ok {
			_, _ = fmt.Fprint(conn.Writer, string(shutdownFrame))
			flusher.Flush()
		}
	}

	if t.drainWait > 0 {
		select {
		case <-time.After(t.drainWait):
		case <-ctx.Done():
		}
	}

	forceClosed := 0
	for _, conn := range snapshot {
		conn.Cancel()
		forceClosed++
	}
	return forceClosed
}
