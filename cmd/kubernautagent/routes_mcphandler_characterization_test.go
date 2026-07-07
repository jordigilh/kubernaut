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

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/client-go/rest"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	kaconfig "github.com/jordigilh/kubernaut/internal/kubernautagent/config"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	kametrics "github.com/jordigilh/kubernaut/internal/kubernautagent/metrics"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	auth "github.com/jordigilh/kubernaut/pkg/shared/auth"
)

// ============================================================================
// buildMCPHandler — characterization tests (Wave 5 Phase 0b coverage gate).
//
// buildMCPHandler was measured at 9.1% line coverage (go tool cover -func):
// only the 3 early-return guards (mcp_handler_guards_test.go) were exercised.
// These tests pin the synchronous construction path (lease manager, timeout
// manager, disconnect handler, tool registration, BootstrapMCP) BEFORE Phase 3
// decomposes it into named helpers, per the TDD coverage-before-refactor
// mandate. Async callback bodies (lease-expiry, inactivity-timeout, disconnect,
// reconnect, background reconstruction) remain exercised by existing IT/E2E
// interactive-session tests — this file is a safety net for the construction
// path, not a substitute for those.
// ============================================================================

// jsonLogCapture captures funcr.NewJSON log records for assertions on
// specific log fields (e.g. enrichment_in_select_workflow) without depending
// on exact string formatting.
type jsonLogCapture struct {
	mu    sync.Mutex
	lines []string
}

func (c *jsonLogCapture) capture(obj string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lines = append(c.lines, obj)
}

func (c *jsonLogCapture) findByMessage(t testing.TB, msg string) map[string]interface{} {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, line := range c.lines {
		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(line), &parsed); err != nil {
			continue
		}
		if m, ok := parsed["msg"].(string); ok && m == msg {
			return parsed
		}
	}
	t.Fatalf("log message %q not found in %d captured lines", msg, len(c.lines))
	return nil
}

func newMCPTestAgentMetrics() *kametrics.Metrics {
	return kametrics.NewMetricsWithRegistry(prometheus.NewRegistry())
}

func newMCPTestAutoMgr() *session.Manager {
	return session.NewManager(session.NewStore(time.Hour), logr.Discard(), nil, nil)
}

// newMCPTestDSClients builds a *dsClients backed by an httptest server. No
// request is made against it synchronously by buildMCPHandler (only the
// disconnect-triggered background reconstruction path calls DS), so an empty
// 200 handler is sufficient.
func newMCPTestDSClients(t testing.TB) *dsClients {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"workflows": []interface{}{}})
	}))
	t.Cleanup(server.Close)
	ogenC, err := ogenclient.NewClient(server.URL)
	if err != nil {
		t.Fatalf("failed to build ogen client: %v", err)
	}
	return &dsClients{ogenClient: ogenC}
}

func validMCPInteractiveConfig() *kaconfig.Config {
	var cfg kaconfig.Config
	cfg.Interactive.Enabled = true
	cfg.Interactive.SessionTTL = 30 * time.Minute
	cfg.Interactive.InactivityTimeout = 10 * time.Minute
	cfg.Interactive.MaxConcurrentSessions = 5
	cfg.Interactive.RateLimitPerUser = 10
	cfg.Interactive.DisconnectGracePeriod = 60 * time.Second
	return &cfg
}

// validMCPHandlerParams returns a fully-populated mcpHandlerParams using an
// unreachable rest.Config host (per preflight spike: ctrlclient.New makes no
// network call at construction; ReconcileOrphanedLeases fails fast/fail-open
// against a refused connection).
func validMCPHandlerParams(t testing.TB) mcpHandlerParams {
	t.Helper()
	return mcpHandlerParams{
		cfg:          validMCPInteractiveConfig(),
		infra:        &k8sInfra{kubeConfig: &rest.Config{Host: "http://127.0.0.1:1"}},
		ds:           newMCPTestDSClients(t),
		inv:          &investigator.Investigator{},
		enricher:     &enrichment.Enricher{},
		autoMgr:      newMCPTestAutoMgr(),
		authMw:       &auth.Middleware{},
		agentMetrics: newMCPTestAgentMetrics(),
		auditStore:   audit.NopAuditStore{},
		logger:       logr.Discard(),
	}
}

var _ = Describe("buildMCPHandler — construction-path characterization", func() {
	var (
		ctx    context.Context
		cancel context.CancelFunc
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		DeferCleanup(cancel)
	})

	It("returns a non-nil handler and drainer for fully-wired params", func() {
		handler, drainer := buildMCPHandler(ctx, validMCPHandlerParams(GinkgoTB()))

		Expect(handler).NotTo(BeNil(), "expected non-nil http.Handler for fully-wired params")
		Expect(drainer).NotTo(BeNil(), "expected non-nil session drainer for fully-wired params")
	})

	// TestBuildMCPHandler_NilDS_DegradesGracefully proves the bug fix for a
	// production panic discovered during Wave 5 Phase 0b coverage-gate testing:
	// the ContextReconstructor branch was correctly nil-guarded (falls back to
	// noopReconstructor when ds == nil), but the workflow-catalog querier a few
	// lines later dereferenced ds unconditionally, so a nil ds (DataStorage
	// integration disabled/unreachable at startup) crashed the whole MCP
	// interactive-mode handler construction instead of degrading like every
	// other DS-optional dependency in this function (see also buildToolRegistry
	// and readinessHandler, which both guard `ds != nil` the same way).
	//
	// BR-INTERACTIVE-001 / AU-3: MCP interactive mode must remain available
	// (investigate/select-workflow/complete-no-action) even when DataStorage is
	// unavailable; workflow-catalog-dependent lookups must fail with a clear,
	// per-call error rather than taking down the whole handler at construction.
	It("degrades gracefully (non-nil handler, logged warning) when ds is nil", func() {
		capture := &jsonLogCapture{}
		logger := funcr.NewJSON(capture.capture, funcr.Options{})

		p := validMCPHandlerParams(GinkgoTB())
		p.ds = nil
		p.logger = logger

		handler, drainer := buildMCPHandler(ctx, p)

		Expect(handler).NotTo(BeNil(), "expected non-nil http.Handler when ds is nil (DS-optional degradation, not a hard dependency)")
		Expect(drainer).NotTo(BeNil(), "expected non-nil session drainer when ds is nil")

		capture.findByMessage(GinkgoTB(), "MCP interactive mode: DS unavailable — workflow catalog lookups disabled")
	})

	It("disables enrichment in select-workflow when enricher is nil", func() {
		capture := &jsonLogCapture{}
		logger := funcr.NewJSON(capture.capture, funcr.Options{})

		p := validMCPHandlerParams(GinkgoTB())
		p.enricher = nil
		p.logger = logger

		handler, drainer := buildMCPHandler(ctx, p)

		Expect(handler).NotTo(BeNil(), "expected non-nil http.Handler when enricher is nil")
		Expect(drainer).NotTo(BeNil(), "expected non-nil session drainer when enricher is nil")

		fields := capture.findByMessage(GinkgoTB(), "MCP interactive mode fully wired")
		Expect(fields["enrichment_in_select_workflow"]).To(BeFalse(),
			"expected enrichment_in_select_workflow=false when enricher is nil")
	})

	It("returns a nil handler and drainer when controller-runtime client construction fails", func() {
		p := validMCPHandlerParams(GinkgoTB())
		// A CAFile that cannot be read fails rest.HTTPClientFor synchronously
		// (confirmed via preflight spike), without any network round-trip.
		p.infra = &k8sInfra{kubeConfig: &rest.Config{
			Host: "https://127.0.0.1:1",
			TLSClientConfig: rest.TLSClientConfig{
				CAFile: "/nonexistent/path/to/definitely-does-not-exist-kubernaut-test.pem",
			},
		}}

		handler, drainer := buildMCPHandler(ctx, p)

		Expect(handler).To(BeNil(), "expected nil handler when controller-runtime client construction fails")
		Expect(drainer).To(BeNil(), "expected nil session drainer when controller-runtime client construction fails")
	})
})

// TestNoopWorkflowQuerier_ReturnsDescriptiveError proves the noop fallback
// used when ds is nil surfaces a clear, actionable error through the same
// WorkflowCatalogAdapter/tools.WorkflowCatalog path a real request would use
// (SelectWorkflowTool.Handle wraps this as "workflow catalog lookup failed: %w"
// and returns it as a normal tool error to the LLM/client), rather than a nil
// pointer panic or a silently-empty result that would look like "workflow not
// found" instead of "DataStorage unavailable".
var _ = Describe("noopWorkflowQuerier", func() {
	It("returns a descriptive error explaining DataStorage is unavailable", func() {
		q := &noopWorkflowQuerier{}

		_, err := q.ResolveWorkflowCatalogMetadata(context.Background(), "wf-123")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(Or(ContainSubstring("DataStorage"), ContainSubstring("unavailable")))
	})
})
