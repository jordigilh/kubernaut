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
	"fmt"
	"net/http/httptest"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	mcptools "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/tools"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// discoveryRunner provides canned RCA extraction and workflow discovery responses
// for the discover_workflows flow, while still forwarding interactive turns
// to the real Mock LLM for message handling.
type discoveryRunner struct {
	goldenPathRunner
	rcaResult      *katypes.InvestigationResult
	workflowResult *katypes.InvestigationResult
}

func (r *discoveryRunner) RunRCAExtraction(_ context.Context, _ []mcptools.LLMMessage, _ string) (*katypes.InvestigationResult, error) {
	return r.rcaResult, nil
}

func (r *discoveryRunner) RunWorkflowDiscovery(_ context.Context, _ katypes.SignalContext, _ *katypes.InvestigationResult, _ *prompt.EnrichmentData, _ string) (*katypes.InvestigationResult, error) {
	return r.workflowResult, nil
}

// discoverySignalResolver provides canned signal context for tests.
type discoverySignalResolver struct{}

func (d *discoverySignalResolver) ResolveSignalContext(_ context.Context, _ string) (*katypes.SignalContext, error) {
	return &katypes.SignalContext{Severity: "critical"}, nil
}

func (d *discoverySignalResolver) ResolveEnrichmentData(_ context.Context, _ string) (*prompt.EnrichmentData, error) {
	return &prompt.EnrichmentData{}, nil
}

// discoveryHTTPCompleter captures CompleteUserDriving calls.
type discoveryHTTPCompleter struct {
	completedID     string
	completedResult *katypes.InvestigationResult
}

func (c *discoveryHTTPCompleter) CompleteUserDriving(id string, result *katypes.InvestigationResult) error {
	c.completedID = id
	c.completedResult = result
	return nil
}

func (c *discoveryHTTPCompleter) FindUserDrivingByRemediationID(_ string) (string, bool) {
	return "http-sess-discovery", true
}

func (c *discoveryHTTPCompleter) ForceCompleteByRemediationID(_ string, result *katypes.InvestigationResult) error {
	c.completedResult = result
	return nil
}

// callTool calls an arbitrary MCP tool by name with the given args.
func callTool(sess *mcpsdk.ClientSession, toolName string, args map[string]any) (*mcpsdk.CallToolResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return sess.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      toolName,
		Arguments: args,
	})
}

var _ = Describe("Interactive Workflow Discovery — IT flows", Label("integration", "discovery"), func() {

	const (
		discoveredWorkflowID = "restart-pod-v2"
		alternativeWfID      = "rollback-deploy-v1"
	)

	var (
		stack     *realMCPTestStack
		nsName    string
		runner    *discoveryRunner
		completer *discoveryHTTPCompleter
	)

	BeforeEach(func() {
		nsName = uniqueNamespace("disc")
		createNamespace(context.Background(), sharedK8sClient, nsName)

		runner = &discoveryRunner{
			goldenPathRunner: goldenPathRunner{
				response: "I found OOM in deployment/web",
				delay:    50 * time.Millisecond,
			},
			rcaResult: &katypes.InvestigationResult{
				RCASummary: "Pod OOM due to memory leak in deployment/web",
				Confidence: 0.92,
				Severity:   "critical",
			},
			workflowResult: &katypes.InvestigationResult{
				RCASummary: "Pod OOM due to memory leak in deployment/web",
				WorkflowID: discoveredWorkflowID,
				Confidence: 0.88,
			},
		}

		completer = &discoveryHTTPCompleter{}

		opts := defaultRealStackOpts()
		opts.customRunner = runner

		stack = newRealMCPTestStackWithDiscovery(sharedK8sClient, nsName, opts, runner, completer)
	})

	AfterEach(func() {
		stack.Close()
	})

	Describe("IT-KA-DISC-001: start -> message -> discover_workflows -> select_workflow (auto-complete)", func() {
		It("should complete the full discovery flow and auto-complete the HTTP session", func() {
			sess, err := connectMCP(stack.Server, "alice@acme.io")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = sess.Close() }()

			By("starting a session")
			result, err := callInvestigate(sess, map[string]any{
				"rr_id":  "rr-disc-001",
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())
			output, err := decodeOutput(result)
			Expect(err).NotTo(HaveOccurred())
			Expect(output["status"]).To(Equal("started"))

			By("sending a message")
			result, err = callInvestigate(sess, map[string]any{
				"rr_id":   "rr-disc-001",
				"action":  "message",
				"message": "Why is this pod restarting?",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())

			By("discovering workflows")
			result, err = callInvestigate(sess, map[string]any{
				"rr_id":  "rr-disc-001",
				"action": "discover_workflows",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())
			output, err = decodeOutput(result)
			Expect(err).NotTo(HaveOccurred())
			Expect(output["status"]).To(Equal("workflows_discovered"))

			By("selecting a workflow")
			result, err = callTool(sess, "kubernaut_select_workflow", map[string]any{
				"rr_id":       "rr-disc-001",
				"workflow_id": discoveredWorkflowID,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())
			output, err = decodeOutput(result)
			Expect(err).NotTo(HaveOccurred())
			Expect(output["status"]).To(Equal("workflow_selected"))

			By("verifying auto-complete wrote to HTTP session")
			Expect(completer.completedID).To(Equal("http-sess-discovery"))
			Expect(completer.completedResult).NotTo(BeNil())
			Expect(completer.completedResult.WorkflowID).To(Equal(discoveredWorkflowID))
		})
	})

	Describe("IT-KA-DISC-002: start -> message -> complete_no_action", func() {
		It("should complete with no workflow and auto-complete the HTTP session", func() {
			sess, err := connectMCP(stack.Server, "alice@acme.io")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = sess.Close() }()

			By("starting a session")
			result, err := callInvestigate(sess, map[string]any{
				"rr_id":  "rr-disc-002",
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())

			By("sending a message")
			result, err = callInvestigate(sess, map[string]any{
				"rr_id":   "rr-disc-002",
				"action":  "message",
				"message": "Looks like a transient issue",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())

			By("completing with no action")
			result, err = callTool(sess, "kubernaut_complete_no_action", map[string]any{
				"rr_id":  "rr-disc-002",
				"reason": "false alarm",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())
			output, err := decodeOutput(result)
			Expect(err).NotTo(HaveOccurred())
			Expect(output["status"]).To(Equal("completed_no_action"))

			By("verifying auto-complete wrote to HTTP session")
			Expect(completer.completedID).To(Equal("http-sess-discovery"))
		})
	})

	Describe("IT-KA-DISC-003: invalidation -> discover -> message -> select rejected -> discover -> select accepted", func() {
		It("should reject stale select_workflow after message and accept after re-discovery", func() {
			sess, err := connectMCP(stack.Server, "alice@acme.io")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = sess.Close() }()

			By("starting a session")
			result, err := callInvestigate(sess, map[string]any{
				"rr_id":  "rr-disc-003",
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())

			By("discovering workflows")
			result, err = callInvestigate(sess, map[string]any{
				"rr_id":  "rr-disc-003",
				"action": "discover_workflows",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())

			By("sending a message (invalidates discovery)")
			result, err = callInvestigate(sess, map[string]any{
				"rr_id":   "rr-disc-003",
				"action":  "message",
				"message": "Wait, let me look deeper",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())

			By("attempting select_workflow (should be rejected)")
			result, err = callTool(sess, "kubernaut_select_workflow", map[string]any{
				"rr_id":       "rr-disc-003",
				"workflow_id": discoveredWorkflowID,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeTrue(), "select_workflow should fail after message invalidated discovery")

			By("re-discovering workflows")
			result, err = callInvestigate(sess, map[string]any{
				"rr_id":  "rr-disc-003",
				"action": "discover_workflows",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())

			By("selecting workflow after re-discovery (should succeed)")
			result, err = callTool(sess, "kubernaut_select_workflow", map[string]any{
				"rr_id":       "rr-disc-003",
				"workflow_id": discoveredWorkflowID,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())
			output, err := decodeOutput(result)
			Expect(err).NotTo(HaveOccurred())
			Expect(output["status"]).To(Equal("workflow_selected"))
		})
	})

	Describe("IT-KA-DISC-004: select_workflow rejects workflow_id not in discovery results", func() {
		It("should reject an unknown workflow_id after discovery", func() {
			sess, err := connectMCP(stack.Server, "alice@acme.io")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = sess.Close() }()

			By("starting a session")
			result, err := callInvestigate(sess, map[string]any{
				"rr_id":  "rr-disc-004",
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())

			By("discovering workflows")
			result, err = callInvestigate(sess, map[string]any{
				"rr_id":  "rr-disc-004",
				"action": "discover_workflows",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())

			By("selecting a workflow_id not in discovery results")
			result, err = callTool(sess, "kubernaut_select_workflow", map[string]any{
				"rr_id":       "rr-disc-004",
				"workflow_id": "totally-unknown-wf",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeTrue(),
				"select_workflow must reject workflow_id not from discovery results")
		})
	})

	Describe("IT-KA-DISC-005: complete_no_action rejects non-driver", func() {
		It("should reject complete_no_action from a user who is not the driver", func() {
			driverSess, err := connectMCP(stack.Server, "alice@acme.io")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = driverSess.Close() }()

			By("alice starts a session")
			result, err := callInvestigate(driverSess, map[string]any{
				"rr_id":  "rr-disc-005",
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())

			By("bob connects and tries complete_no_action")
			bobSess, err := connectMCP(stack.Server, "bob@acme.io")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = bobSess.Close() }()

			result, err = callTool(bobSess, "kubernaut_complete_no_action", map[string]any{
				"rr_id":  "rr-disc-005",
				"reason": "not my session",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeTrue(),
				"complete_no_action must reject non-driver callers")
		})
	})

	Describe("IT-KA-DISC-006: complete_no_action after discover_workflows uses stored RCA", func() {
		It("should complete with RCA from the extraction step", func() {
			sess, err := connectMCP(stack.Server, "alice@acme.io")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = sess.Close() }()

			By("starting a session")
			result, err := callInvestigate(sess, map[string]any{
				"rr_id":  "rr-disc-006",
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())

			By("sending a message")
			result, err = callInvestigate(sess, map[string]any{
				"rr_id":   "rr-disc-006",
				"action":  "message",
				"message": "pod keeps crashing",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())

			By("discovering workflows (stores RCA)")
			result, err = callInvestigate(sess, map[string]any{
				"rr_id":  "rr-disc-006",
				"action": "discover_workflows",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())

			By("completing with no action (should use stored RCA)")
			completer.completedResult = nil
			result, err = callTool(sess, "kubernaut_complete_no_action", map[string]any{
				"rr_id":  "rr-disc-006",
				"reason": "user prefers manual fix",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())
			output, err := decodeOutput(result)
			Expect(err).NotTo(HaveOccurred())
			Expect(output["status"]).To(Equal("completed_no_action"))

			Expect(completer.completedResult).NotTo(BeNil())
			Expect(completer.completedResult.RCASummary).To(ContainSubstring("OOM"),
				"should propagate the RCA summary from discover_workflows extraction")
		})
	})

	Describe("IT-KA-DISC-007: complete_no_action without prior messages or discovery", func() {
		It("should complete with minimal result and default reason", func() {
			sess, err := connectMCP(stack.Server, "alice@acme.io")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = sess.Close() }()

			By("starting a session")
			result, err := callInvestigate(sess, map[string]any{
				"rr_id":  "rr-disc-007",
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())

			By("immediately completing with no action")
			completer.completedResult = nil
			result, err = callTool(sess, "kubernaut_complete_no_action", map[string]any{
				"rr_id": "rr-disc-007",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())
			output, err := decodeOutput(result)
			Expect(err).NotTo(HaveOccurred())
			Expect(output["status"]).To(Equal("completed_no_action"))

			Expect(completer.completedResult).NotTo(BeNil())
			Expect(completer.completedResult.Reason).To(Equal("no action needed"),
				"should use default reason when none provided")
			Expect(completer.completedResult.RCASummary).To(ContainSubstring("without workflow"),
				"should use minimal RCA summary when no prior RCA exists")
		})
	})
})

// newRealMCPTestStackWithDiscovery builds a test stack with select_workflow and complete_no_action
// tools wired up, using custom runner and HTTP completer for discovery testing.
func newRealMCPTestStackWithDiscovery(k8sClient client.Client, namespace string, opts realStackOpts, runner *discoveryRunner, completer *discoveryHTTPCompleter) *realMCPTestStack {
	stack := &realMCPTestStack{
		K8sClient: k8sClient,
		Namespace: namespace,
	}

	logrLogger := logr.Discard()

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

	resolver := &discoverySignalResolver{}

	investigateOpts := []mcptools.InvestigateOption{
		mcptools.WithRateLimiter(stack.RateLimiter),
		mcptools.WithTimeoutTracker(stack.TimeoutMgr),
		mcptools.WithNotifyFunc(stack.Notifier.Notify),
		mcptools.WithSignalContextResolver(resolver),
	}
	investigateTool := mcptools.NewInvestigateTool(stack.SessionMgr, runner, recon, mcptools.NopAutonomousManager{}, investigateOpts...)

	// Wire catalog that returns the discovered workflow
	catalog := &discoveryMockCatalog{
		workflows: map[string]*mcptools.CatalogWorkflow{
			"restart-pod-v2": {
				WorkflowID:      "restart-pod-v2",
				WorkflowName:    "Restart Pod",
				ExecutionEngine: "argo",
				ExecutionBundle: "oci://restart:v2",
				Version:         "v2.0",
			},
			"rollback-deploy-v1": {
				WorkflowID:      "rollback-deploy-v1",
				WorkflowName:    "Rollback Deploy",
				ExecutionEngine: "argo",
				ExecutionBundle: "oci://rollback:v1",
				Version:         "v1.0",
			},
		},
	}

	selectTool := mcptools.NewSelectWorkflowTool(catalog, stack.SessionMgr,
		mcptools.WithHTTPSessionCompleter(completer),
		mcptools.WithMutexProvider(investigateTool),
	)

	completeNoActionTool := mcptools.NewCompleteNoActionTool(stack.SessionMgr,
		mcptools.WithCompleteNoActionHTTPCompleter(completer),
		mcptools.WithCompleteNoActionMutexProvider(investigateTool),
	)

	toolDeps := mcpinternal.ToolDeps{
		Investigate:      mcptools.InvestigateRegistration(investigateTool, stack.EventStore, stack.Notifier),
		SelectWorkflow:   mcptools.SelectWorkflowRegistration(selectTool),
		CompleteNoAction: mcptools.CompleteNoActionRegistration(completeNoActionTool),
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

	return stack
}

// discoveryMockCatalog returns workflows by ID.
type discoveryMockCatalog struct {
	workflows map[string]*mcptools.CatalogWorkflow
}

func (c *discoveryMockCatalog) GetWorkflowByID(_ context.Context, wfID string) (*mcptools.CatalogWorkflow, error) {
	wf, ok := c.workflows[wfID]
	if !ok {
		return nil, fmt.Errorf("workflow %q not found", wfID)
	}
	return wf, nil
}
