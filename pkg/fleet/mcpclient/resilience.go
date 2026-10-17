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
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/jordigilh/kubernaut/pkg/fleet"
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
	// ConnectTimeout bounds each individual MCP connect attempt made by
	// connectWithBackoff and doReconnect, independent of whether the
	// caller's context carries a deadline. Without this, a single hung
	// handshake (issue #1934) blocks the entire backoff loop -- and thus
	// service startup or reconnection -- forever, silently defeating
	// MaxElapsedTime/MaxInterval.
	ConnectTimeout time.Duration
	// DiscoverProbeTimeout bounds go-sdk v1.7.0+'s SEP-2575 "server/discover"
	// probe with its own sub-timeout, independent of ConnectTimeout. Without
	// this, a gateway that hangs (rather than erroring) on server/discover
	// can silently consume the entire ConnectTimeout budget before go-sdk's
	// own legacy "initialize" fallback ever gets a chance to run (issue
	// #2262) -- every connect attempt then fails identically, even though
	// the fallback handshake itself works. Zero disables the bound.
	DiscoverProbeTimeout time.Duration
}

// DefaultResilienceConfig returns production-ready defaults per Phase 6 plan.
func DefaultResilienceConfig() ResilienceConfig {
	return ResilienceConfig{
		InitialInterval:     1 * time.Second,
		MaxInterval:         30 * time.Second,
		MaxElapsedTime:      5 * time.Minute,
		TokenRefreshTimeout: 10 * time.Second,
		// Matches MaxInterval: a single attempt should never take longer
		// than the max backoff interval we'd otherwise wait between attempts.
		ConnectTimeout: 30 * time.Second,
		// Generous relative to the manually-measured ~50ms round trip
		// against a healthy gateway (issue #2262); operators can raise it
		// via ResilienceConfig if a legitimately-slow-but-working gateway
		// needs more headroom.
		DiscoverProbeTimeout: 5 * time.Second,
	}
}

// ResilienceConfigFromFleet converts a fleet.FleetResilienceConfig DTO
// (populated from Helm-chart-rendered or operator-supplied Go config, issue
// #2262 Phase 2) into a full ResilienceConfig, merging any non-zero field
// in f over DefaultResilienceConfig(). A zero value in f always means "use
// the default for this field" -- this is what makes adding this field to
// existing config structs safe: an existing deployment with no
// `resilience:` block set at all gets behavior identical to before this
// field existed.
func ResilienceConfigFromFleet(f fleet.FleetResilienceConfig) ResilienceConfig {
	cfg := DefaultResilienceConfig()
	if f.InitialInterval > 0 {
		cfg.InitialInterval = f.InitialInterval
	}
	if f.MaxInterval > 0 {
		cfg.MaxInterval = f.MaxInterval
	}
	if f.MaxElapsedTime > 0 {
		cfg.MaxElapsedTime = f.MaxElapsedTime
	}
	if f.TokenRefreshTimeout > 0 {
		cfg.TokenRefreshTimeout = f.TokenRefreshTimeout
	}
	if f.ConnectTimeout > 0 {
		cfg.ConnectTimeout = f.ConnectTimeout
	}
	if f.DiscoverProbeTimeout > 0 {
		cfg.DiscoverProbeTimeout = f.DiscoverProbeTimeout
	}
	return cfg
}

// ResilientClient wraps Client with reconnection, retry, and readiness semantics.
type ResilientClient struct {
	endpoint string
	opts     []Option
	config   ResilienceConfig
	logger   logr.Logger

	client atomic.Pointer[Client]
	ready  atomic.Bool
	// generation increments only when a newly connected session is published.
	// Consumers use it to fence bindings resolved against an older session.
	generation atomic.Uint64

	sessionMu sync.RWMutex

	lifecycleMu    sync.Mutex
	lifecycleState atomic.Uint32
	reconnectRun   *reconnectRun
}

const (
	resilientActive uint32 = iota
	resilientClosing
	resilientClosed
)

type reconnectRun struct {
	rc            *ResilientClient
	cancel        context.CancelFunc
	operationDone <-chan struct{}
	work          func() error
	done          chan struct{}
	deadlineCh    chan time.Time
	waiters       int
	mu            sync.Mutex
	result        error
	finished      bool
}

func (r *reconnectRun) addDeadline(ctx context.Context) {
	deadline, ok := ctx.Deadline()
	if !ok {
		return
	}
	select {
	case r.deadlineCh <- deadline:
	default:
		// A reconnect caller's deadline is advisory to the shared operation;
		// its own ctx.Done path still returns the exact caller error.
	}
}

func (r *reconnectRun) leave() {
	r.mu.Lock()
	if r.waiters > 0 {
		r.waiters--
	}
	last := r.waiters == 0 && !r.finished
	r.mu.Unlock()
	if last {
		r.cancel()
	}
}

func (r *reconnectRun) finish(err error) {
	r.mu.Lock()
	if r.finished {
		r.mu.Unlock()
		return
	}
	r.result = err
	r.finished = true
	close(r.done)
	r.mu.Unlock()

	r.rc.lifecycleMu.Lock()
	if r.rc.reconnectRun == r {
		r.rc.reconnectRun = nil
	}
	r.rc.lifecycleMu.Unlock()
}

func (r *reconnectRun) resultError() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.result
}

func (r *reconnectRun) manageDeadline() {
	var timer *time.Timer
	var timerC <-chan time.Time
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()
	for {
		select {
		case deadline := <-r.deadlineCh:
			remaining := time.Until(deadline)
			if remaining <= 0 {
				r.cancel()
				continue
			}
			if timer == nil {
				timer = time.NewTimer(remaining)
			} else {
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(remaining)
			}
			timerC = timer.C
		case <-timerC:
			r.cancel()
			timerC = nil
		case <-r.operationDone:
			return
		case <-r.done:
			return
		}
	}
}

// NewResilient creates a ResilientClient that connects with backoff and auto-reconnects.
func NewResilient(ctx context.Context, endpoint string, cfg ResilienceConfig, logger logr.Logger, opts ...Option) (*ResilientClient, error) {
	// The construction context bounds the initial handshake only. Reconnects
	// after startup need a provider lifetime parent rather than inheriting a
	// one-shot startup deadline.
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

// ResilienceConfig returns the backoff/timeout configuration this client was
// constructed with. Primarily for tests proving a chart-shaped
// fleet.FleetResilienceConfig override (issue #2262 Phase 2) actually
// reaches the real NewResilient call at each service's production wiring
// point (e.g. cmd/gateway/main.go's registerAdapters), without relying on
// timing-sensitive assertions against an unreachable/hanging endpoint.
func (rc *ResilientClient) ResilienceConfig() ResilienceConfig {
	return rc.config
}

// Get implements client.Reader with automatic reconnection on transient errors.
func (rc *ResilientClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if rc.isClosed() {
		return ErrSessionClosed
	}
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
	if rc.isClosed() {
		return ErrSessionClosed
	}
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
	if rc.isClosed() {
		return nil
	}
	c, _ := rc.loadPublished()
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
		if rc.isClosed() {
			return nil
		}
		c, _ := rc.loadPublished()
		if c == nil {
			return nil
		}
		return c.Session()
	}
}

// SessionHandleProvider returns the current provider-owned session together
// with its publication generation. A zero Session means the gateway is
// currently disconnected; callers must return a bounded transient error rather
// than retaining or closing a stale session.
func (rc *ResilientClient) SessionHandleProvider() SessionHandleProvider {
	return func() SessionHandle {
		if rc.isClosed() {
			return SessionHandle{Generation: rc.generation.Load()}
		}
		c, generation := rc.loadPublished()
		if c == nil {
			return SessionHandle{Generation: generation}
		}
		return SessionHandle{
			Session:    c.Session(),
			Generation: generation,
		}
	}
}

// Close terminates the underlying client connection.
func (rc *ResilientClient) Close() error {
	rc.lifecycleMu.Lock()
	switch rc.lifecycleState.Load() {
	case resilientClosed:
		rc.lifecycleMu.Unlock()
		return nil
	case resilientClosing:
		rc.lifecycleMu.Unlock()
		return nil
	default:
		rc.lifecycleState.Store(resilientClosing)
		run := rc.reconnectRun
		rc.lifecycleMu.Unlock()
		if run != nil {
			run.cancel()
		}
	}

	rc.ready.Store(false)
	rc.sessionMu.Lock()
	c := rc.client.Swap(nil)
	rc.sessionMu.Unlock()
	var closeErr error
	if c != nil {
		closeErr = c.Close()
	}
	rc.lifecycleState.Store(resilientClosed)
	return closeErr
}

// CloseContext fences future publication and closes the current provider-owned
// session before the caller's deadline. The MCP SDK's Client.Close has no
// context parameter, so the context is used to avoid starting work after the
// caller has already expired; the SDK close itself is normally non-blocking.
func (rc *ResilientClient) CloseContext(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return rc.Close()
}

func (rc *ResilientClient) isClosed() bool {
	return rc.lifecycleState.Load() != resilientActive
}

func (rc *ResilientClient) loadPublished() (*Client, uint64) {
	rc.sessionMu.RLock()
	defer rc.sessionMu.RUnlock()
	return rc.client.Load(), rc.generation.Load()
}

func (rc *ResilientClient) publish(c *Client) error {
	rc.lifecycleMu.Lock()
	defer rc.lifecycleMu.Unlock()
	if rc.lifecycleState.Load() != resilientActive {
		_ = c.Close()
		return ErrSessionClosed
	}
	rc.sessionMu.Lock()
	rc.client.Store(c)
	rc.generation.Add(1)
	rc.sessionMu.Unlock()
	rc.ready.Store(true)
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

		attemptCtx, cancel := context.WithTimeout(ctx, rc.config.ConnectTimeout)
		defer cancel()
		c, connErr := New(attemptCtx, rc.endpoint, rc.connectOpts()...)
		if connErr != nil {
			lastErr = connErr
			rc.logger.V(1).Info("Connection attempt failed, retrying",
				"error", connErr,
				"nextInterval", backoff.Duration)
			return false, nil
		}

		if publishErr := rc.publish(c); publishErr != nil {
			lastErr = publishErr
			return false, publishErr
		}
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

// Reconnect forces a fresh connection to the MCP Gateway, closing any
// existing (possibly dead) session first. Exposed so session-provider
// Clients (NewFromSessionProvider) can repair a session that died from a
// protocol-level error -- pass this method as WithReconnect when
// constructing per-cluster readers from SessionProvider().
func (rc *ResilientClient) Reconnect(ctx context.Context) error {
	return rc.reconnect(ctx)
}

func (rc *ResilientClient) reconnect(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	rc.lifecycleMu.Lock()
	if rc.lifecycleState.Load() != resilientActive {
		rc.lifecycleMu.Unlock()
		return ErrSessionClosed
	}
	run := rc.reconnectRun
	if run == nil {
		opCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
		run = &reconnectRun{
			rc:            rc,
			cancel:        cancel,
			operationDone: opCtx.Done(),
			work:          func() error { return rc.doReconnect(opCtx) },
			done:          make(chan struct{}),
			deadlineCh:    make(chan time.Time, 8),
			waiters:       1,
		}
		rc.reconnectRun = run
		run.addDeadline(ctx)
		go run.manageDeadline()
		go func() { run.finish(run.work()) }()
	} else {
		run.mu.Lock()
		run.waiters++
		run.mu.Unlock()
		run.addDeadline(ctx)
	}
	rc.lifecycleMu.Unlock()

	defer run.leave()
	select {
	case <-run.done:
		return run.resultError()
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (rc *ResilientClient) doReconnect(ctx context.Context) error {
	if rc.isClosed() {
		return ErrSessionClosed
	}
	rc.ready.Store(false)

	old := rc.client.Load()
	if old != nil {
		if t := rc.findReloadableTransport(); t != nil {
			t.InvalidateToken() //nolint:contextcheck // 401-retry token invalidation rebuilds the long-lived token source, independent of any single request
			rc.logger.Info("OAuth2 token invalidated before reconnect")
		}
		_ = old.Close()
	}

	attemptCtx, cancel := context.WithTimeout(ctx, rc.config.ConnectTimeout)
	defer cancel()
	c, err := New(attemptCtx, rc.endpoint, rc.connectOpts()...)
	if err != nil {
		rc.logger.Error(err, "Reconnection failed")
		return err
	}

	if err := rc.publish(c); err != nil {
		return err
	}
	rc.logger.Info("Reconnected to MCP Gateway")
	return nil
}

// connectOpts returns rc.opts with the DiscoverProbeTimeout bound and its
// logger appended, so every New() call made by connectWithBackoff and
// doReconnect gets the #2262 fix with zero changes required at any of the 8
// cmd/*/main.go call sites that construct rc.opts. Appending (rather than
// mutating rc.opts in place) is safe: since len(rc.opts) == cap(rc.opts) for
// every slice built from a variadic ...Option parameter, append always
// allocates a fresh backing array here, so concurrent callers (e.g. a
// reconnect racing a readiness prober) never share or corrupt each other's
// temporary slice.
func (rc *ResilientClient) connectOpts() []Option {
	return append(rc.opts,
		WithDiscoverProbeTimeout(rc.config.DiscoverProbeTimeout),
		WithDiscoverProbeLogger(rc.logger),
	)
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
	return isRetryableSessionError(err)
}

// isRetryableSessionError classifies errors that indicate a dead or
// unauthenticated MCP session that reconnecting can repair: closed
// connections, missing sessions, network-level failures, and 401s. Shared by
// ResilientClient.Get/List and session-provider Client.callTool so both
// retry-on-reconnect paths use identical semantics.
func isRetryableSessionError(err error) bool {
	failure := ClassifyTransportError(err)
	return failure != nil && failure.Action() == ActionReconnectAndRetryRead
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
