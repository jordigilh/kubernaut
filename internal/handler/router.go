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
}

func (c RouterConfig) validate() error {
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

// NewRouter creates an HTTP handler with all routes registered.
// Routes are organized into two tiers:
//   - Public (no auth): /healthz, /readyz, /metrics, /.well-known/agent-card.json
//   - Authenticated: /a2a/invoke, /mcp
func NewRouter(cfg RouterConfig) (http.Handler, error) {
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid router config: %w", err)
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", handleHealthz)
	mux.HandleFunc("GET /readyz", readyzHandler(cfg.ReadyChecker))
	mux.Handle("GET /metrics", cfg.MetricsRegistry.Handler())
	mux.Handle("GET /.well-known/agent-card.json", cfg.AgentCardHandler)

	mux.Handle("/a2a/invoke", cfg.AuthMiddleware(cfg.A2AHandler))
	mux.Handle("/mcp", cfg.AuthMiddleware(cfg.MCPHandler))

	return metricsMiddleware(cfg.MetricsRegistry, mux), nil
}
