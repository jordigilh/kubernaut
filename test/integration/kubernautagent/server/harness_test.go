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

package server_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	kaserver "github.com/jordigilh/kubernaut/internal/kubernautagent/server"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	katypes "github.com/jordigilh/kubernaut/internal/kubernautagent/types"
	"github.com/jordigilh/kubernaut/pkg/agentclient"
	"github.com/jordigilh/kubernaut/pkg/shared/auth"
)

// testHarness holds a running httptest.Server with the same route stack as
// cmd/kubernautagent/main.go: chi → rate limiter → auth middleware →
// SSEHeadersMiddleware → ogen server.
type testHarness struct {
	Server      *httptest.Server
	Manager     *session.Manager
	Store       *session.Store
	AuditStore  *syncAuditRecorder
	RateLimiter *kaserver.RateLimiter
}

// harnessOption configures the test harness.
type harnessOption func(*harnessConfig)

type harnessConfig struct {
	rateLimitCfg  *kaserver.RateLimitConfig
	authUser      string
	investigator  kaserver.InvestigationRunner
}

func withRateLimit(cfg kaserver.RateLimitConfig) harnessOption {
	return func(c *harnessConfig) { c.rateLimitCfg = &cfg }
}

func withAuthUser(user string) harnessOption {
	return func(c *harnessConfig) { c.authUser = user }
}

func withInvestigator(inv kaserver.InvestigationRunner) harnessOption {
	return func(c *harnessConfig) { c.investigator = inv }
}

// newTestHarness builds the full route stack and returns a running httptest.Server.
func newTestHarness(opts ...harnessOption) *testHarness {
	cfg := &harnessConfig{}
	for _, o := range opts {
		o(cfg)
	}

	store := session.NewStore(30 * time.Minute)
	auditRec := &syncAuditRecorder{}
	mgr := session.NewManager(store, slog.Default(), auditRec, nil)

	inv := cfg.investigator
	if inv == nil {
		inv = &blockingInvestigator{}
	}

	handler := kaserver.NewHandler(mgr, inv, slog.Default(), nil)
	ogenSrv, _ := agentclient.NewServer(handler)

	r := chi.NewRouter()

	var rl *kaserver.RateLimiter
	if cfg.rateLimitCfg != nil {
		rl = kaserver.NewRateLimiter(*cfg.rateLimitCfg, nil)
	} else {
		defaultCfg := kaserver.DefaultRateLimitConfig()
		rl = kaserver.NewRateLimiter(defaultCfg, nil)
	}
	r.Use(rl.Middleware)

	if cfg.authUser != "" {
		r.Use(fakeAuthMiddleware(cfg.authUser))
	}

	r.Mount("/", kaserver.SSEHeadersMiddleware(ogenSrv))

	ts := httptest.NewServer(r)
	return &testHarness{
		Server:      ts,
		Manager:     mgr,
		Store:       store,
		AuditStore:  auditRec,
		RateLimiter: rl,
	}
}

func (h *testHarness) Close() {
	h.Server.Close()
	if h.RateLimiter != nil {
		h.RateLimiter.Stop()
	}
}

// fakeAuthMiddleware injects a fixed user identity into the request context,
// matching the production auth middleware's behavior.
func fakeAuthMiddleware(user string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), auth.UserContextKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// blockingInvestigator blocks until context is cancelled.
type blockingInvestigator struct{}

func (b *blockingInvestigator) Investigate(ctx context.Context, _ katypes.SignalContext) (*katypes.InvestigationResult, error) {
	<-ctx.Done()
	return &katypes.InvestigationResult{RCASummary: "cancelled"}, nil
}

// fastInvestigator completes immediately with a fixed result.
type fastInvestigator struct{}

func (f *fastInvestigator) Investigate(_ context.Context, _ katypes.SignalContext) (*katypes.InvestigationResult, error) {
	return &katypes.InvestigationResult{
		RCASummary: "pod OOM killed",
		Confidence: 0.9,
	}, nil
}

// syncAuditRecorder is a thread-safe audit event recorder implementing audit.AuditStore.
type syncAuditRecorder struct {
	mu     sync.Mutex
	events []*audit.AuditEvent
}

func (r *syncAuditRecorder) StoreAudit(_ context.Context, event *audit.AuditEvent) error {
	r.mu.Lock()
	r.events = append(r.events, event)
	r.mu.Unlock()
	return nil
}

func (r *syncAuditRecorder) Events() []*audit.AuditEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := make([]*audit.AuditEvent, len(r.events))
	copy(cp, r.events)
	return cp
}

func (r *syncAuditRecorder) EventsOfType(eventType string) []*audit.AuditEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	var matched []*audit.AuditEvent
	for _, e := range r.events {
		if e.EventType == eventType {
			matched = append(matched, e)
		}
	}
	return matched
}
