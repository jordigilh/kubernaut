package handler

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/jordigilh/kubernaut-apifrontend/internal/metrics"
	"github.com/jordigilh/kubernaut-apifrontend/internal/requestid"
	"github.com/jordigilh/kubernaut-apifrontend/internal/streaming"
)

// RouterConfig holds all dependencies needed to construct the HTTP router.
type RouterConfig struct {
	MetricsRegistry    *metrics.Registry
	A2AHandler         http.Handler
	MCPHandler         http.Handler
	AgentCardHandler   http.Handler
	AuthMiddleware     func(http.Handler) http.Handler
	PreAuthMiddleware  func(http.Handler) http.Handler
	PostAuthMiddleware func(http.Handler) http.Handler
	ReadyChecker       func() bool
	MaxPayloadBytes    int64
	SSETracker         *streaming.ConnectionTracker
	Draining           *atomic.Bool
}

func (c *RouterConfig) validate() error {
	if c.MetricsRegistry == nil {
		return fmt.Errorf("MetricsRegistry is required")
	}
	if c.A2AHandler == nil {
		return fmt.Errorf("A2AHandler is required")
	}
	if c.MCPHandler == nil {
		return fmt.Errorf("MCPHandler is required")
	}
	if c.AgentCardHandler == nil {
		return fmt.Errorf("AgentCardHandler is required")
	}
	if c.AuthMiddleware == nil {
		return fmt.Errorf("AuthMiddleware is required")
	}
	if c.ReadyChecker == nil {
		return fmt.Errorf("ReadyChecker is required")
	}
	return nil
}

const defaultMaxPayloadBytes int64 = 1 << 20 // 1MB

// NewRouter creates an HTTP handler with all routes registered.
// Routes are organized into two tiers:
//   - Public (no auth): /healthz, /readyz, /metrics, /.well-known/agent-card.json
//   - Authenticated: /a2a/invoke, /mcp
func NewRouter(cfg RouterConfig) (http.Handler, error) { //nolint:gocritic // hugeParam: called once at startup
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid router config: %w", err)
	}

	maxBytes := cfg.MaxPayloadBytes
	if maxBytes <= 0 {
		maxBytes = defaultMaxPayloadBytes
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", handleHealthz)
	mux.HandleFunc("GET /readyz", readyzHandler(cfg.ReadyChecker, cfg.Draining))
	mux.Handle("GET /metrics", cfg.MetricsRegistry.Handler())
	mux.Handle("GET /.well-known/agent-card.json", cfg.AgentCardHandler)

	innerA2A := writeDeadlineMiddleware(maxBodyMiddleware(maxBytes, trackSSEConnection(cfg.SSETracker, cfg.A2AHandler)))
	innerMCP := writeDeadlineMiddleware(maxBodyMiddleware(maxBytes, trackSSEConnection(cfg.SSETracker, cfg.MCPHandler)))

	if cfg.PostAuthMiddleware != nil {
		innerA2A = cfg.PostAuthMiddleware(innerA2A)
		innerMCP = cfg.PostAuthMiddleware(innerMCP)
	}

	a2aChain := cfg.AuthMiddleware(innerA2A)
	mcpChain := cfg.AuthMiddleware(innerMCP)

	if cfg.PreAuthMiddleware != nil {
		a2aChain = cfg.PreAuthMiddleware(a2aChain)
		mcpChain = cfg.PreAuthMiddleware(mcpChain)
	}

	mux.Handle("POST /a2a/invoke", a2aChain)
	mux.Handle("POST /mcp", mcpChain)

	return metricsMiddleware(cfg.MetricsRegistry, securityHeadersMiddleware(mux)), nil
}

// maxBodyMiddleware limits request body size to prevent resource exhaustion.
func maxBodyMiddleware(maxBytes int64, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
		next.ServeHTTP(w, r)
	})
}

const defaultWriteDeadline = 60 * time.Second

// writeDeadlineMiddleware sets a per-request write deadline via
// http.ResponseController. SSE/streaming handlers can extend this by calling
// SetWriteDeadline(time.Time{}) when they upgrade to long-lived streams.
func writeDeadlineMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rc := http.NewResponseController(w)
		_ = rc.SetWriteDeadline(time.Now().Add(defaultWriteDeadline))
		next.ServeHTTP(w, r)
	})
}

// trackSSEConnection registers active streaming connections with the tracker
// for graceful shutdown. Each connection is tracked from start to completion.
func trackSSEConnection(tracker *streaming.ConnectionTracker, next http.Handler) http.Handler {
	if tracker == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithCancel(r.Context())
		connID := requestid.FromContext(r.Context())
		if connID == "" {
			connID = r.RemoteAddr
		}
		tracker.Add(&streaming.TrackedConnection{
			ID:     connID,
			Writer: w,
			Cancel: cancel,
		})
		defer tracker.Remove(connID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
