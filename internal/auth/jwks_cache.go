package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/prometheus/client_golang/prometheus"
	gobreaker "github.com/sony/gobreaker/v2"
)

// NewCircuitBreakerStateGauge creates a fresh circuit breaker state gauge.
// Call this from the metrics registry to avoid package-level state.
func NewCircuitBreakerStateGauge() *prometheus.GaugeVec {
	return prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "af",
		Name:      "circuit_breaker_state",
		Help:      "JWKS circuit breaker state per issuer (0=closed, 1=half-open, 2=open).",
	}, []string{"dependency"})
}

// JWKSCache provides JWKS key caching with circuit breaker support per issuer.
// Per ARCHITECTURE.md: 3 consecutive failures -> open, 30s half-open timeout, 1 success -> close.
// Fail-open for existing sessions (cached keys available), fail-closed for new sessions.
type JWKSCache struct {
	mu       sync.RWMutex
	entries  map[string]*jwksCacheEntry
	client   *http.Client
	breakers map[string]*gobreaker.CircuitBreaker[*jose.JSONWebKeySet]
}

type jwksCacheEntry struct {
	keys      *jose.JSONWebKeySet
	fetchedAt time.Time
}

// JWKSCacheOption configures JWKSCache behavior.
type JWKSCacheOption func(*jwksCacheConfig)

type jwksCacheConfig struct {
	cbTimeout time.Duration
	cbGauge   *prometheus.GaugeVec
}

// WithCBTimeout sets the circuit breaker half-open timeout. Default is 30s.
func WithCBTimeout(d time.Duration) JWKSCacheOption {
	return func(c *jwksCacheConfig) { c.cbTimeout = d }
}

// WithCBGauge sets a Prometheus gauge for circuit breaker state reporting.
func WithCBGauge(g *prometheus.GaugeVec) JWKSCacheOption {
	return func(c *jwksCacheConfig) { c.cbGauge = g }
}

// NewJWKSCache creates a JWKS cache with circuit breakers per issuer.
func NewJWKSCache(client *http.Client, issuers []string, opts ...JWKSCacheOption) *JWKSCache {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}

	cfg := &jwksCacheConfig{cbTimeout: 30 * time.Second}
	for _, opt := range opts {
		opt(cfg)
	}

	cache := &JWKSCache{
		entries:  make(map[string]*jwksCacheEntry, len(issuers)),
		client:   client,
		breakers: make(map[string]*gobreaker.CircuitBreaker[*jose.JSONWebKeySet], len(issuers)),
	}

	for _, issuer := range issuers {
		settings := gobreaker.Settings{
			Name:        fmt.Sprintf("jwks-%s", issuer),
			MaxRequests: 1,
			Interval:    0,
			Timeout:     cfg.cbTimeout,
			ReadyToTrip: func(counts gobreaker.Counts) bool {
				return counts.ConsecutiveFailures >= 3
			},
		}
		if cfg.cbGauge != nil {
			depLabel := fmt.Sprintf("jwks_%s", issuer)
			settings.OnStateChange = func(_ string, _, to gobreaker.State) {
				cfg.cbGauge.WithLabelValues(depLabel).Set(float64(to))
			}
		}
		cache.breakers[issuer] = gobreaker.NewCircuitBreaker[*jose.JSONWebKeySet](settings)
	}

	return cache
}

// GetKeys returns the JWKS for the given issuer, fetching if necessary.
// If the circuit breaker is open and a cached version exists, returns the cached version (fail-open).
// If the circuit breaker is open and no cache exists, returns ErrCircuitOpen (fail-closed).
func (c *JWKSCache) GetKeys(ctx context.Context, issuerURL string) (*jose.JSONWebKeySet, error) {
	cb, ok := c.breakers[issuerURL]
	if !ok {
		return nil, fmt.Errorf("no circuit breaker configured for issuer: %s", issuerURL)
	}

	result, err := cb.Execute(func() (*jose.JSONWebKeySet, error) {
		return c.fetchJWKS(ctx, issuerURL)
	})
	if err == nil {
		c.mu.Lock()
		c.entries[issuerURL] = &jwksCacheEntry{keys: result, fetchedAt: time.Now()}
		c.mu.Unlock()
		return result, nil
	}

	// Circuit breaker rejected or fetch failed -- try cached keys
	c.mu.RLock()
	entry, hasCached := c.entries[issuerURL]
	c.mu.RUnlock()

	if hasCached && entry.keys != nil {
		return entry.keys, nil
	}

	if cb.State() == gobreaker.StateOpen {
		return nil, ErrCircuitOpen
	}

	return nil, fmt.Errorf("JWKS fetch failed for %s: %w", issuerURL, err)
}

func (c *JWKSCache) fetchJWKS(ctx context.Context, issuerURL string) (*jose.JSONWebKeySet, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, issuerURL, http.NoBody)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		// #54: always close response body, discard error since we already have data or error
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("JWKS endpoint returned %d", resp.StatusCode)
	}

	var keySet jose.JSONWebKeySet
	if err := json.NewDecoder(resp.Body).Decode(&keySet); err != nil {
		return nil, fmt.Errorf("decode JWKS: %w", err)
	}

	return &keySet, nil
}
