/*
Copyright 2026 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package mcpclient

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Compile-time interface compliance.
var _ ResourceClient = (*ResilientClient)(nil)
var _ client.Reader = (*ResilientClient)(nil)

// ResilienceConfig holds configuration for the resilient MCP client wrapper.
type ResilienceConfig struct {
	// InitialInterval is the starting backoff interval for startup retries.
	InitialInterval time.Duration
	// MaxInterval is the maximum backoff interval.
	MaxInterval time.Duration
	// MaxElapsedTime is the total time before giving up on startup.
	MaxElapsedTime time.Duration
	// TokenRefreshTimeout bounds OAuth2 token refresh HTTP calls.
	TokenRefreshTimeout time.Duration
}

// DefaultResilienceConfig returns production-ready defaults per Phase 6 plan.
func DefaultResilienceConfig() ResilienceConfig {
	return ResilienceConfig{
		InitialInterval:     1 * time.Second,
		MaxInterval:         30 * time.Second,
		MaxElapsedTime:      5 * time.Minute,
		TokenRefreshTimeout: 10 * time.Second,
	}
}

// ResilientClient wraps Client with reconnection, retry, and readiness semantics.
type ResilientClient struct {
	endpoint string
	opts     []Option
	config   ResilienceConfig
	logger   logr.Logger

	client atomic.Pointer[Client]
	ready  atomic.Bool
}

// NewResilient creates a ResilientClient that connects with backoff and auto-reconnects.
func NewResilient(ctx context.Context, endpoint string, cfg ResilienceConfig, logger logr.Logger, opts ...Option) (*ResilientClient, error) {
	rc := &ResilientClient{
		endpoint: endpoint,
		opts:     opts,
		config:   cfg,
		logger:   logger.WithName("resilient-mcp-client"),
	}

	if err := rc.connectWithBackoff(ctx); err != nil {
		return rc, fmt.Errorf("initial connection failed after backoff: %w", err)
	}
	return rc, nil
}

// Ready returns true when the client has an active MCP session.
func (rc *ResilientClient) Ready() bool {
	return rc.ready.Load()
}

// Get implements client.Reader with automatic reconnection on transient errors.
func (rc *ResilientClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	c := rc.client.Load()
	if c == nil {
		return fmt.Errorf("MCP client not connected")
	}

	err := c.Get(ctx, key, obj, opts...)
	if err != nil && rc.isRetryableError(err) {
		rc.logger.Info("Retryable error on Get, reconnecting", "error", err)
		if reconnErr := rc.reconnect(ctx); reconnErr != nil {
			return fmt.Errorf("reconnect failed: %w (original: %w)", reconnErr, err)
		}
		c = rc.client.Load()
		return c.Get(ctx, key, obj, opts...)
	}
	return err
}

// List implements client.Reader with automatic reconnection on transient errors.
func (rc *ResilientClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	c := rc.client.Load()
	if c == nil {
		return fmt.Errorf("MCP client not connected")
	}

	err := c.List(ctx, list, opts...)
	if err != nil && rc.isRetryableError(err) {
		rc.logger.Info("Retryable error on List, reconnecting", "error", err)
		if reconnErr := rc.reconnect(ctx); reconnErr != nil {
			return fmt.Errorf("reconnect failed: %w (original: %w)", reconnErr, err)
		}
		c = rc.client.Load()
		return c.List(ctx, list, opts...)
	}
	return err
}

// Session returns the underlying MCP session. May be nil if disconnected.
//
// WARNING: The returned session becomes stale when the ResilientClient
// reconnects. Prefer SessionProvider() for long-lived per-cluster clients.
func (rc *ResilientClient) Session() *mcp.ClientSession {
	c := rc.client.Load()
	if c == nil {
		return nil
	}
	return c.Session()
}

// SessionProvider returns a function that always resolves the current active
// session. Use this instead of Session() when creating per-cluster Client
// instances that must survive ResilientClient reconnections.
func (rc *ResilientClient) SessionProvider() SessionProvider {
	return func() *mcp.ClientSession {
		c := rc.client.Load()
		if c == nil {
			return nil
		}
		return c.Session()
	}
}

// Close terminates the underlying client connection.
func (rc *ResilientClient) Close() error {
	rc.ready.Store(false)
	c := rc.client.Load()
	if c != nil {
		return c.Close()
	}
	return nil
}

func (rc *ResilientClient) connectWithBackoff(ctx context.Context) error {
	backoff := wait.Backoff{
		Duration: rc.config.InitialInterval,
		Factor:   2.0,
		Cap:      rc.config.MaxInterval,
		Steps:    int(rc.config.MaxElapsedTime/rc.config.InitialInterval) + 1,
	}

	var lastErr error
	err := wait.ExponentialBackoff(backoff, func() (bool, error) {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		default:
		}

		c, connErr := New(ctx, rc.endpoint, rc.opts...)
		if connErr != nil {
			lastErr = connErr
			rc.logger.V(1).Info("Connection attempt failed, retrying",
				"error", connErr,
				"nextInterval", backoff.Duration)
			return false, nil
		}

		rc.client.Store(c)
		rc.ready.Store(true)
		rc.logger.Info("MCP Gateway connection established", "endpoint", rc.endpoint)
		return true, nil
	})

	if err != nil {
		if lastErr != nil {
			return fmt.Errorf("%w: last attempt: %w", err, lastErr)
		}
		return err
	}
	return nil
}

func (rc *ResilientClient) reconnect(ctx context.Context) error {
	rc.ready.Store(false)

	old := rc.client.Load()
	if old != nil {
		if t := rc.findReloadableTransport(); t != nil {
			t.InvalidateToken()
			rc.logger.Info("OAuth2 token invalidated before reconnect")
		}
		_ = old.Close()
	}

	c, err := New(ctx, rc.endpoint, rc.opts...)
	if err != nil {
		rc.logger.Error(err, "Reconnection failed")
		return err
	}

	rc.client.Store(c)
	rc.ready.Store(true)
	rc.logger.Info("Reconnected to MCP Gateway")
	return nil
}

// findReloadableTransport walks the option chain to find a ReloadableOAuth2Transport.
func (rc *ResilientClient) findReloadableTransport() *ReloadableOAuth2Transport {
	cfg := &clientConfig{}
	for _, opt := range rc.opts {
		opt(cfg)
	}
	if cfg.httpClient == nil || cfg.httpClient.Transport == nil {
		return nil
	}
	if t, ok := cfg.httpClient.Transport.(*ReloadableOAuth2Transport); ok {
		return t
	}
	return nil
}

func (rc *ResilientClient) isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, mcp.ErrConnectionClosed) || errors.Is(err, mcp.ErrSessionMissing) {
		return true
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "401")
}

// WithTokenRefreshTimeout returns an Option that bounds the OAuth2 token
// refresh HTTP call, preventing indefinite hangs when the IdP is unreachable.
func WithTokenRefreshTimeout(timeout time.Duration) Option {
	return func(cfg *clientConfig) {
		if cfg.httpClient == nil {
			cfg.httpClient = &http.Client{Timeout: timeout}
		} else {
			cfg.httpClient.Timeout = timeout
		}
	}
}
