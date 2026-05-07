package handler

import (
	"fmt"
	"net/http"

	"github.com/jordigilh/kubernaut-apifrontend/internal/metrics"
)

// RouterConfig holds all dependencies needed to construct the HTTP router.
type RouterConfig struct {
	MetricsRegistry  *metrics.Registry
	A2AHandler       http.Handler
	MCPHandler       http.Handler
	AgentCardHandler http.Handler
	AuthMiddleware   func(http.Handler) http.Handler
	ReadyChecker     func() bool
	MaxPayloadBytes  int64
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
	mux.HandleFunc("GET /readyz", readyzHandler(cfg.ReadyChecker))
	mux.Handle("GET /metrics", cfg.MetricsRegistry.Handler())
	mux.Handle("GET /.well-known/agent-card.json", cfg.AgentCardHandler)

	mux.Handle("/a2a/invoke", cfg.AuthMiddleware(maxBodyMiddleware(maxBytes, cfg.A2AHandler)))
	mux.Handle("/mcp", cfg.AuthMiddleware(maxBodyMiddleware(maxBytes, cfg.MCPHandler)))

	return metricsMiddleware(cfg.MetricsRegistry, mux), nil
}

// maxBodyMiddleware limits request body size to prevent resource exhaustion.
func maxBodyMiddleware(maxBytes int64, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
		next.ServeHTTP(w, r)
	})
}
