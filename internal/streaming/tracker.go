package streaming

import (
	"context"
	"fmt"
	"net/http"
	"sync"

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
	mu    sync.Mutex
	conns map[string]*TrackedConnection
	gauge prometheus.Gauge
}

// NewConnectionTracker creates a new tracker with an optional Prometheus gauge.
func NewConnectionTracker(gauge prometheus.Gauge) *ConnectionTracker {
	return &ConnectionTracker{
		conns: make(map[string]*TrackedConnection),
		gauge: gauge,
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

// DrainAll sends an SSE shutdown event to all connections, then cancels their
// contexts to force-close. Returns the number of connections that were force-closed.
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

	shutdownFrame := []byte("event: shutdown\ndata: {\"retry_ms\":5000}\n\n")

	forceClosed := 0
	for _, conn := range snapshot {
		select {
		case <-ctx.Done():
			forceClosed += len(snapshot) - forceClosed
			for _, c := range snapshot {
				c.Cancel()
			}
			return forceClosed
		default:
		}

		if flusher, ok := conn.Writer.(http.Flusher); ok {
			_, _ = fmt.Fprint(conn.Writer, string(shutdownFrame))
			flusher.Flush()
		}
		conn.Cancel()
		forceClosed++
	}
	return forceClosed
}
