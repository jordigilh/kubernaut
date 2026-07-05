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

// Package readiness provides a shared, periodically-probed readiness gate
// for Fleet-dependent services. When Fleet is enabled, a service's
// scope-checker backend (FMC/ACM) and/or MCP Gateway connection must be
// reachable for the service to correctly evaluate fleet-scoped requests. A
// Gate aggregates the relevant Probers for a service and exposes a single
// Ready()/Check() signal that plugs into the service's existing /readyz
// surface (mgr.AddReadyzCheck for controller-runtime services, or a custom
// HTTP readiness handler for GW/AF/KA), causing Kubernetes to remove the
// pod from Service endpoints entirely while any Fleet dependency is down.
//
// Authority: ADR-068 (Fleet Federation Architecture), BR-INTEGRATION-065,
// BR-FLEET-054. See the tracking issue for the fail-closed-via-readiness
// decision (config completeness fails closed at startup via Validate();
// runtime unreachability fails closed via this package's periodic probing).
package readiness

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
)

// Prober checks whether a single Fleet dependency is currently reachable.
type Prober interface {
	// Probe returns nil when the dependency is healthy, or an error
	// describing why it is not.
	Probe(ctx context.Context) error
}

// Pinger checks connectivity to a scope-checker backend. Both
// pkg/fleet/fmc.HTTPClient and pkg/fleet/acm.Client satisfy this
// structurally; defined locally to avoid a readiness -> fmc/acm dependency
// edge (matches the existing pkg/fleet/fmc.Pinger shape).
type Pinger interface {
	Ping(ctx context.Context) error
}

// DefaultInterval is how often a Gate re-probes its Probers when the
// caller passes a non-positive interval to NewGate.
const DefaultInterval = 30 * time.Second

// errNotReady is returned by Check when no probe has recorded a specific
// error yet (should not normally be observable, since Start probes
// synchronously before returning).
var errNotReady = errors.New("fleet dependency not ready")

// Gate aggregates one or more Probers into a single readiness signal.
// It probes synchronously once on Start (so /readyz is correct from boot)
// and then periodically on a ticker until Stop is called or the Start
// context is cancelled.
type Gate struct {
	probers  []Prober
	interval time.Duration
	logger   logr.Logger

	mu      sync.RWMutex
	ready   bool
	lastErr error

	stopCh   chan struct{}
	doneCh   chan struct{}
	stopOnce sync.Once
	started  atomic.Bool
}

// NewGate creates a Gate that periodically probes every given Prober.
// A non-positive interval falls back to DefaultInterval.
func NewGate(interval time.Duration, logger logr.Logger, probers ...Prober) *Gate {
	if interval <= 0 {
		interval = DefaultInterval
	}
	return &Gate{
		probers:  probers,
		interval: interval,
		logger:   logger.WithName("fleet-readiness"),
		stopCh:   make(chan struct{}),
		doneCh:   make(chan struct{}),
	}
}

// Start probes every Prober synchronously once (so Ready()/Check() are
// correct immediately after Start returns), then begins the periodic probe
// loop in the background. The loop stops when ctx is cancelled or Stop is
// called.
func (g *Gate) Start(ctx context.Context) {
	g.probeOnce(ctx)
	g.started.Store(true)
	go g.loop(ctx)
}

// Stop halts the periodic probe loop and waits for it to exit. Safe to call
// multiple times or before Start.
func (g *Gate) Stop() {
	g.stopOnce.Do(func() { close(g.stopCh) })
	if g.started.Load() {
		<-g.doneCh
	}
}

// Ready reports whether the last probe cycle succeeded for every Prober.
func (g *Gate) Ready() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.ready
}

// Check adapts Ready() to the func(*http.Request) error shape expected by
// controller-runtime's mgr.AddReadyzCheck (healthz.Checker) and by the
// custom readiness handlers in GW/AF/KA.
func (g *Gate) Check(_ *http.Request) error {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.ready {
		return nil
	}
	if g.lastErr != nil {
		return g.lastErr
	}
	return errNotReady
}

func (g *Gate) loop(ctx context.Context) {
	defer close(g.doneCh)
	ticker := time.NewTicker(g.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-g.stopCh:
			return
		case <-ticker.C:
			g.probeOnce(ctx)
		}
	}
}

func (g *Gate) probeOnce(ctx context.Context) {
	for _, p := range g.probers {
		if err := p.Probe(ctx); err != nil {
			g.setState(false, err)
			g.logger.Info("Fleet dependency probe failed, readiness NotReady", "error", err)
			return
		}
	}
	g.setState(true, nil)
}

func (g *Gate) setState(ready bool, err error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.ready = ready
	g.lastErr = err
}
