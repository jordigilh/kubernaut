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

package mcp_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	mcpadapters "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/adapters"
	mcptools "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/tools"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/langchaingo"
)

// ---------------------------------------------------------------------------
// Context-capturing runner: wraps InvestigatorRunnerAdapter and records
// whether SignalContext was attached to the context on each call.
// ---------------------------------------------------------------------------

type contextCapturingRunner struct {
	inner mcptools.InvestigatorRunner

	mu              sync.Mutex
	capturedSignals []katypes.SignalContext
	capturedTargets []katypes.RemediationTarget
}

func (r *contextCapturingRunner) RunInteractiveTurn(ctx context.Context, messages []mcptools.LLMMessage, correlationID string) (string, error) {
	if signal, ok := katypes.SignalContextFromContext(ctx); ok {
		r.mu.Lock()
		r.capturedSignals = append(r.capturedSignals, signal)
		r.mu.Unlock()
	}
	return r.inner.RunInteractiveTurn(ctx, messages, correlationID)
}

func (r *contextCapturingRunner) RunRCAExtraction(ctx context.Context, messages []mcptools.LLMMessage, correlationID string) (*katypes.InvestigationResult, error) {
	return r.inner.RunRCAExtraction(ctx, messages, correlationID)
}

func (r *contextCapturingRunner) RunWorkflowDiscovery(ctx context.Context, signal katypes.SignalContext, rcaResult *katypes.InvestigationResult, enrichData *prompt.EnrichmentData, correlationID string) (*katypes.InvestigationResult, error) {
	if rcaResult != nil {
		r.mu.Lock()
		r.capturedTargets = append(r.capturedTargets, rcaResult.RemediationTarget)
		r.mu.Unlock()
	}
	return r.inner.RunWorkflowDiscovery(ctx, signal, rcaResult, enrichData, correlationID)
}

func (r *contextCapturingRunner) RunFullInvestigation(ctx context.Context, signal katypes.SignalContext) (*katypes.InvestigationResult, error) {
	return r.inner.RunFullInvestigation(ctx, signal)
}

func (r *contextCapturingRunner) getSignals() []katypes.SignalContext {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]katypes.SignalContext, len(r.capturedSignals))
	copy(out, r.capturedSignals)
	return out
}

func (r *contextCapturingRunner) getTargets() []katypes.RemediationTarget {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]katypes.RemediationTarget, len(r.capturedTargets))
	copy(out, r.capturedTargets)
	return out
}

// newCapturingMCPStack builds an MCP stack using a contextCapturingRunner that
// wraps the real investigator. Used to assert context propagation and argument
// values through the production dispatch path.
func newCapturingMCPStack(k8sClient client.Client, namespace string, opts realStackOpts, resolver mcptools.SignalContextResolver) (*realMCPTestStack, *contextCapturingRunner) {
	stack := &realMCPTestStack{
		K8sClient: k8sClient,
		Namespace: namespace,
	}

	logrLogger := logr.Discard()

	llmAdapter, err := langchaingo.New("openai", sharedMockLLMEndpoint, "test-model", "test-key")
	Expect(err).ToNot(HaveOccurred())
	stack.LLMClient = llmAdapter

	promptBuilder, buildErr := prompt.NewBuilder()
	Expect(buildErr).ToNot(HaveOccurred())

	inv := investigator.New(investigator.Config{
		Client:        llmAdapter,
		Builder:       promptBuilder,
		ResultParser:  parser.NewResultParser(),
		AuditStore:    audit.NopAuditStore{},
		Logger:        logrLogger,
		MaxTurns:      15,
		ModelName:     "test-model",
		ScopeResolver: itScopeResolver(),
	})
	realRunner := mcpadapters.NewInvestigatorRunnerAdapter(inv)
	capturingRunner := &contextCapturingRunner{inner: realRunner}

	recon := mcpinternal.NewDSContextReconstructor(sharedDSClient, 5*time.Second, logrLogger)

	leaseOpts := []mcpinternal.LeaseOption{
		mcpinternal.WithSessionTTL(opts.sessionTTL),
	}
	stack.SessionMgr = mcpinternal.NewLeaseSessionManagerConcrete(k8sClient, namespace, logrLogger, leaseOpts...)
	stack.RateLimiter = mcpinternal.NewSessionRateLimiter(opts.maxPerMinute, opts.maxMessageSize)
	stack.Notifier = mcpinternal.NewSessionNotifier()
	warningIntervals := opts.warningIntervals
	if warningIntervals == nil {
		warningIntervals = []time.Duration{opts.inactivityTimeout - 1*time.Second}
	}
	stack.TimeoutMgr = mcpinternal.NewTimeoutManager(
		opts.inactivityTimeout,
		warningIntervals,
		func(sessionID string) {
			stack.addExpired(sessionID)
			_ = stack.SessionMgr.Release(sessionID, "inactivity_timeout")
		},
	)
	stack.EventStore = mcpinternal.NewDelegatingEventStore()

	investigateOpts := []mcptools.InvestigateOption{
		mcptools.WithRateLimiter(stack.RateLimiter),
		mcptools.WithTimeoutTracker(stack.TimeoutMgr),
		mcptools.WithNotifyFunc(stack.Notifier.Notify),
	}
	if resolver != nil {
		investigateOpts = append(investigateOpts, mcptools.WithSignalContextResolver(resolver))
	}
	investigateTool := mcptools.NewInvestigateTool(stack.SessionMgr, capturingRunner, recon, mcptools.NopAutonomousManager{}, investigateOpts...)

	toolDeps := mcpinternal.ToolDeps{
		Investigate: mcptools.InvestigateRegistration(investigateTool, stack.EventStore, stack.Notifier, logr.Discard()),
	}

	handler, srv := mcpinternal.BootstrapMCP(mcpinternal.MCPDeps{
		AuthMiddleware: fakeAuthMiddlewareWithUserInfo,
		Tools:          toolDeps,
		EventStore:     stack.EventStore,
	})
	stack.MCPServer = srv

	r := chi.NewRouter()
	r.Use(fakeAuthMiddlewareWithUserInfo)
	r.Handle("/mcp", handler)
	r.Handle("/mcp/*", handler)
	stack.Server = httptest.NewServer(r)

	return stack, capturingRunner
}

// newAutonomousMCPStack builds an MCP stack with a real session.Manager for
// autonomous investigation. Used to test start_autonomous through MCP dispatch.
func newAutonomousMCPStack(k8sClient client.Client, namespace string, opts realStackOpts, resolver mcptools.SignalContextResolver) (*realMCPTestStack, *session.Manager) {
	stack := &realMCPTestStack{
		K8sClient: k8sClient,
		Namespace: namespace,
	}

	logrLogger := logr.Discard()

	llmAdapter, err := langchaingo.New("openai", sharedMockLLMEndpoint, "test-model", "test-key")
	Expect(err).ToNot(HaveOccurred())
	stack.LLMClient = llmAdapter

	promptBuilder, buildErr := prompt.NewBuilder()
	Expect(buildErr).ToNot(HaveOccurred())

	inv := investigator.New(investigator.Config{
		Client:        llmAdapter,
		Builder:       promptBuilder,
		ResultParser:  parser.NewResultParser(),
		AuditStore:    audit.NopAuditStore{},
		Logger:        logrLogger,
		MaxTurns:      15,
		ModelName:     "test-model",
		ScopeResolver: itScopeResolver(),
	})
	runner := mcpadapters.NewInvestigatorRunnerAdapter(inv)

	recon := mcpinternal.NewDSContextReconstructor(sharedDSClient, 5*time.Second, logrLogger)

	leaseOpts := []mcpinternal.LeaseOption{
		mcpinternal.WithSessionTTL(opts.sessionTTL),
	}
	stack.SessionMgr = mcpinternal.NewLeaseSessionManagerConcrete(k8sClient, namespace, logrLogger, leaseOpts...)
	stack.RateLimiter = mcpinternal.NewSessionRateLimiter(opts.maxPerMinute, opts.maxMessageSize)
	stack.Notifier = mcpinternal.NewSessionNotifier()
	warningIntervals := opts.warningIntervals
	if warningIntervals == nil {
		warningIntervals = []time.Duration{opts.inactivityTimeout - 1*time.Second}
	}
	stack.TimeoutMgr = mcpinternal.NewTimeoutManager(
		opts.inactivityTimeout,
		warningIntervals,
		func(sessionID string) {
			stack.addExpired(sessionID)
			_ = stack.SessionMgr.Release(sessionID, "inactivity_timeout")
		},
	)
	stack.EventStore = mcpinternal.NewDelegatingEventStore()

	sessionStore := session.NewStore(30 * time.Minute)
	autoMgr := session.NewManager(sessionStore, logrLogger, audit.NopAuditStore{}, nil)

	investigateOpts := []mcptools.InvestigateOption{
		mcptools.WithRateLimiter(stack.RateLimiter),
		mcptools.WithTimeoutTracker(stack.TimeoutMgr),
		mcptools.WithNotifyFunc(stack.Notifier.Notify),
	}
	if resolver != nil {
		investigateOpts = append(investigateOpts, mcptools.WithSignalContextResolver(resolver))
	}
	investigateTool := mcptools.NewInvestigateTool(stack.SessionMgr, runner, recon, autoMgr, investigateOpts...)

	toolDeps := mcpinternal.ToolDeps{
		Investigate: mcptools.InvestigateRegistration(investigateTool, stack.EventStore, stack.Notifier, logr.Discard()),
	}

	handler, srv := mcpinternal.BootstrapMCP(mcpinternal.MCPDeps{
		AuthMiddleware: fakeAuthMiddlewareWithUserInfo,
		Tools:          toolDeps,
		EventStore:     stack.EventStore,
	})
	stack.MCPServer = srv

	r := chi.NewRouter()
	r.Use(fakeAuthMiddlewareWithUserInfo)
	r.Handle("/mcp", handler)
	r.Handle("/mcp/*", handler)
	stack.Server = httptest.NewServer(r)

	return stack, autoMgr
}

// ---------------------------------------------------------------------------
// Pyramid Invariant Wiring Proofs (#1374, #1376)
// ---------------------------------------------------------------------------

var _ = Describe("Pyramid Invariant: KA MCP Wiring Proofs", Label("integration", "wiring"), func() {

	Describe("IT-KA-1374-F9-002: SignalContext propagated on message action [BR-WORKFLOW-004, #1374 F9]", func() {
		It("should attach SignalContext to context before RunInteractiveTurn", func() {
			nsName := uniqueNamespace("f9-signal-ctx")
			createNamespace(context.Background(), sharedK8sClient, nsName)

			resolver := &discoverySignalResolver{}
			stack, capRunner := newCapturingMCPStack(sharedK8sClient, nsName, defaultRealStackOpts(), resolver)
			defer stack.Close()

			By("connecting and starting session")
			sess, err := connectMCP(stack.Server, "alice@acme.io")
			Expect(err).NotTo(HaveOccurred())

			_, err = callInvestigate(sess, map[string]any{
				"rr_id":  "rr-f9-signal",
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred())

			By("sending a message through MCP dispatch")
			result, err := callInvestigate(sess, map[string]any{
				"rr_id":   "rr-f9-signal",
				"action":  "message",
				"message": "What is the root cause of this OOMKilled event?",
			})
			Expect(err).NotTo(HaveOccurred())

			output, decErr := decodeOutput(result)
			Expect(decErr).NotTo(HaveOccurred())
			Expect(output["status"]).To(Equal("message_received"))

			By("verifying SignalContext was captured from context")
			signals := capRunner.getSignals()
			Expect(signals).To(HaveLen(1),
				"F9 wiring: SignalContextFromContext must return signal during RunInteractiveTurn")
			Expect(signals[0].ResourceKind).To(Equal("Deployment"))
			Expect(signals[0].Namespace).To(Equal("production"))
			Expect(signals[0].Name).To(Equal("OOMKilled"))
		})
	})

	Describe("IT-KA-1374-PF02-001: RemediationTarget cleared after conversation extraction [#1374]", func() {
		It("should clear RemediationTarget before RunWorkflowDiscovery when RCA is extracted from conversation", func() {
			nsName := uniqueNamespace("pf02-target-clear")
			createNamespace(context.Background(), sharedK8sClient, nsName)

			resolver := &discoverySignalResolver{}
			stack, capRunner := newCapturingMCPStack(sharedK8sClient, nsName, defaultRealStackOpts(), resolver)
			defer stack.Close()

			By("connecting and starting session")
			sess, err := connectMCP(stack.Server, "alice@acme.io")
			Expect(err).NotTo(HaveOccurred())

			_, err = callInvestigate(sess, map[string]any{
				"rr_id":  "rr-pf02-clear",
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred())

			By("sending messages to build conversation context")
			_, err = callInvestigate(sess, map[string]any{
				"rr_id":   "rr-pf02-clear",
				"action":  "message",
				"message": "The api-server pod is getting OOMKilled repeatedly",
			})
			Expect(err).NotTo(HaveOccurred())

			By("calling discover_workflows (forces RCA extraction from conversation)")
			result, err := callInvestigate(sess, map[string]any{
				"rr_id":  "rr-pf02-clear",
				"action": "discover_workflows",
			})
			Expect(err).NotTo(HaveOccurred())

			output, decErr := decodeOutput(result)
			Expect(decErr).NotTo(HaveOccurred())
			Expect(output["status"]).To(Equal("workflows_discovered"))

			By("verifying RemediationTarget was empty when passed to RunWorkflowDiscovery")
			targets := capRunner.getTargets()
			Expect(targets).To(HaveLen(1),
				"RunWorkflowDiscovery should have been called once")
			Expect(targets[0]).To(Equal(katypes.RemediationTarget{}),
				"PF02: RemediationTarget must be cleared after extraction from conversation "+
					"to prevent SyncSignalFromRCA from overwriting authoritative signal identity")
		})
	})

	Describe("IT-KA-1374-F4-001: start_autonomous through real MCP stack [BR-WORKFLOW-004, #1374 F4]", func() {
		It("should launch autonomous investigation via real session.Manager", func() {
			nsName := uniqueNamespace("f4-autonomous")
			createNamespace(context.Background(), sharedK8sClient, nsName)

			resolver := &discoverySignalResolver{}
			stack, autoMgr := newAutonomousMCPStack(sharedK8sClient, nsName, defaultRealStackOpts(), resolver)
			defer stack.Close()

			By("connecting to MCP")
			sess, err := connectMCP(stack.Server, "alice@acme.io")
			Expect(err).NotTo(HaveOccurred())

			By("calling start_autonomous through MCP dispatch")
			result, err := callInvestigate(sess, map[string]any{
				"rr_id":  "rr-f4-auto",
				"action": "start_autonomous",
			})
			Expect(err).NotTo(HaveOccurred())

			output, decErr := decodeOutput(result)
			Expect(decErr).NotTo(HaveOccurred())
			Expect(output["status"]).To(Equal("autonomous_started"),
				"F4: start_autonomous must dispatch through real session.Manager")
			sessionID, ok := output["session_id"].(string)
			Expect(ok).To(BeTrue())
			Expect(sessionID).NotTo(BeEmpty(),
				"F4: real session.Manager must return a non-empty session ID")

			By("verifying investigation completes in session store")
			Eventually(func() bool {
				result, found := autoMgr.GetLatestRCAResultByRemediationID("rr-f4-auto")
				return found && result != nil
			}, 30*time.Second, 500*time.Millisecond).Should(BeTrue(),
				"F4: autonomous investigation must complete via RunFullInvestigation")

			By("verifying investigation produced a valid RCA")
			result2, found := autoMgr.GetLatestRCAResultByRemediationID("rr-f4-auto")
			Expect(found).To(BeTrue())
			Expect(result2.RCASummary).NotTo(BeEmpty(),
				"F4: RunFullInvestigation should produce an RCA summary via the real investigator")
		})
	})

	Describe("IT-KA-1374-F5F6-001: Enrichment and DetectedLabelsJSON through MCP discovery [BR-WORKFLOW-004, #1374 F5/F6]", func() {
		It("should propagate enrichment data and DetectedLabelsJSON from investigator through MCP", func() {
			nsName := uniqueNamespace("f5f6-enrichment")
			createNamespace(context.Background(), sharedK8sClient, nsName)

			resolver := &discoverySignalResolver{}
			stack, capRunner := newCapturingMCPStack(sharedK8sClient, nsName, defaultRealStackOpts(), resolver)
			defer stack.Close()

			By("connecting and starting session")
			sess, err := connectMCP(stack.Server, "alice@acme.io")
			Expect(err).NotTo(HaveOccurred())

			_, err = callInvestigate(sess, map[string]any{
				"rr_id":  "rr-f5f6-enrichment",
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred())

			By("sending a message to build conversation context")
			_, err = callInvestigate(sess, map[string]any{
				"rr_id":   "rr-f5f6-enrichment",
				"action":  "message",
				"message": "The api-server pod is getting OOMKilled",
			})
			Expect(err).NotTo(HaveOccurred())

			By("calling discover_workflows through MCP dispatch")
			result, err := callInvestigate(sess, map[string]any{
				"rr_id":  "rr-f5f6-enrichment",
				"action": "discover_workflows",
			})
			Expect(err).NotTo(HaveOccurred())

			output, decErr := decodeOutput(result)
			Expect(decErr).NotTo(HaveOccurred())
			Expect(output["status"]).To(Equal("workflows_discovered"))

			By("verifying RunWorkflowDiscovery was called (production dispatch proof)")
			targets := capRunner.getTargets()
			Expect(targets).To(HaveLen(1),
				"F5: RunWorkflowDiscovery must be called through MCP dispatch path")

			By("verifying discovery result contains workflow data")
			discoveryRaw, ok := output["discovery"]
			Expect(ok).To(BeTrue(), "F5: MCP response must include discovery result")

			discoveryJSON, err := json.Marshal(discoveryRaw)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(discoveryJSON)).To(BeNumerically(">", 2),
				"F5/F6: discovery result must contain workflow selection data from the investigator pipeline")
		})
	})
})
