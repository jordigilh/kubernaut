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
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/langchaingo"
	wfclient "github.com/jordigilh/kubernaut/pkg/workflowexecution/client"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// discoverySignalResolver returns a signal context that triggers the oomkilled
// scenario in the real Mock LLM container. The Phase 3 prompt template emits
// "Signal Name: OOMKilled" which the Mock LLM's signalScenario matcher detects.
type discoverySignalResolver struct{}

func (d *discoverySignalResolver) ResolveSignalContext(_ context.Context, _ string) (*katypes.SignalContext, error) {
	return &katypes.SignalContext{
		Name:         "OOMKilled",
		Severity:     "critical",
		Environment:  "Production",
		Priority:     "P0",
		ResourceKind: "Deployment",
		Namespace:    "production",
		ResourceName: "api-server",
	}, nil
}

// discoveryHTTPCompleter captures CompleteUserDriving calls in-memory.
// Retained as a stub (#1174): the production HTTPSessionCompleter is
// session.Manager, which requires the full gateway HTTP long-poll bridge.
// Wiring that in IT is disproportionate; the stub validates method calls
// and result payloads, which is sufficient for behavioral assurance.
type discoveryHTTPCompleter struct {
	mu              sync.Mutex
	completedID     string
	completedResult *katypes.InvestigationResult
}

func (c *discoveryHTTPCompleter) CompleteUserDriving(id string, result *katypes.InvestigationResult) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.completedID = id
	c.completedResult = result
	return nil
}

func (c *discoveryHTTPCompleter) FindUserDrivingByRemediationID(_ string) (string, bool) {
	return "http-sess-discovery", true
}

func (c *discoveryHTTPCompleter) ForceCompleteByRemediationID(_ string, result *katypes.InvestigationResult) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.completedResult = result
	return nil
}

func (c *discoveryHTTPCompleter) getCompletedResult() *katypes.InvestigationResult {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.completedResult
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

	var (
		stack     *realMCPTestStack
		nsName    string
		completer *discoveryHTTPCompleter
	)

	// DS-assigned UUIDs resolved from sharedWorkflowUUIDs (#1174).
	recommendedWfID := func() string { return sharedWorkflowUUIDs["oomkill-increase-memory-v1:production"] }
	alternativeWfID := func() string { return sharedWorkflowUUIDs["generic-restart-v1:production"] }

	BeforeEach(func() {
		nsName = uniqueNamespace("disc")
		createNamespace(context.Background(), sharedK8sClient, nsName)

		completer = &discoveryHTTPCompleter{}

		stack = newRealMCPTestStackWithDiscovery(sharedK8sClient, nsName, defaultRealStackOpts(), completer)
	})

	AfterEach(func() {
		stack.Close()
	})

	Describe("IT-KA-DISC-001: start -> message -> discover_workflows -> select_workflow (auto-complete) (#1169)", func() {
		It("should complete the full discovery flow with per-workflow parameters and auto-complete the HTTP session", func() {
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
			if result.IsError {
				for _, c := range result.Content {
					if tc, ok := c.(*mcpsdk.TextContent); ok {
						GinkgoWriter.Printf("DISC-001 discover_workflows error: %s\n", tc.Text)
					}
				}
			}
			Expect(result.IsError).To(BeFalse())
			output, err = decodeOutput(result)
			Expect(err).NotTo(HaveOccurred())
			Expect(output["status"]).To(Equal("workflows_discovered"))

			By("verifying discovery response contains parameters from real Mock LLM (#1169)")
			responseStr, ok := output["response"].(string)
			Expect(ok).To(BeTrue(), "response field must be a JSON string")
			var discovery map[string]interface{}
			Expect(json.Unmarshal([]byte(responseStr), &discovery)).To(Succeed(),
				"inner response JSON must parse for parameter inspection")

			rec, _ := discovery["recommended"].(map[string]interface{})
			Expect(rec).NotTo(BeNil(), "recommended workflow must be present in discovery response")
			recParams, _ := rec["parameters"].(map[string]interface{})
			Expect(recParams).To(HaveKeyWithValue("MEMORY_LIMIT_NEW", "512Mi"),
				"recommended workflow must surface LLM-provided parameters (#1169)")

			alts, _ := discovery["alternatives"].([]interface{})
			Expect(alts).To(HaveLen(1), "one alternative must be present in discovery response")
			alt0, _ := alts[0].(map[string]interface{})
			alt0Params, _ := alt0["parameters"].(map[string]interface{})
			Expect(alt0Params).To(HaveKeyWithValue("REPLICA_COUNT", "3"),
				"alternative workflow must surface LLM-provided parameters (#1169)")

			By("selecting the recommended workflow")
			result, err = callTool(sess, "kubernaut_select_workflow", map[string]any{
				"rr_id":       "rr-disc-001",
				"workflow_id": recommendedWfID(),
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())
			output, err = decodeOutput(result)
			Expect(err).NotTo(HaveOccurred())
			Expect(output["status"]).To(Equal("workflow_selected"))

			By("verifying auto-complete wrote recommended parameters to HTTP session (#1169)")
			Eventually(completer.getCompletedResult, 5*time.Second, 100*time.Millisecond).ShouldNot(BeNil())
			cr := completer.getCompletedResult()
			Expect(cr.WorkflowID).To(Equal(recommendedWfID()))
			Expect(cr.Parameters).To(HaveKeyWithValue("MEMORY_LIMIT_NEW", "512Mi"),
				"recommended parameters must flow through to the HTTP session completer (#1169)")
			Expect(cr.Parameters).NotTo(HaveKey("REPLICA_COUNT"),
				"alternative parameters must not leak to the recommended workflow's completion (#1169)")

			By("verifying TARGET_RESOURCE_* parameters survive discovery param merge (IT-KA-DISC-001 extension)")
			Expect(cr.Parameters).To(HaveKeyWithValue("TARGET_RESOURCE_NAME", "api-server"),
				"TARGET_RESOURCE_NAME must be injected from RemediationTarget after buildFinalResult")
			Expect(cr.Parameters).To(HaveKeyWithValue("TARGET_RESOURCE_KIND", "Deployment"),
				"TARGET_RESOURCE_KIND must be injected from RemediationTarget after buildFinalResult")
			Expect(cr.Parameters).To(HaveKeyWithValue("TARGET_RESOURCE_NAMESPACE", "production"),
				"TARGET_RESOURCE_NAMESPACE must be injected from RemediationTarget after buildFinalResult")

			By("verifying TARGET_RESOURCE_API_VERSION is auto-resolved for non-ambiguous kinds [BR-WORKFLOW-004]")
			Expect(cr.Parameters).To(HaveKeyWithValue("TARGET_RESOURCE_API_VERSION", "apps/v1"),
				"BR-WORKFLOW-004: TARGET_RESOURCE_API_VERSION must be auto-resolved via ScopeResolver for unambiguous Deployment kind")
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

			By("sending initial message to build conversation context")
			result, err = callInvestigate(sess, map[string]any{
				"rr_id":   "rr-disc-003",
				"action":  "message",
				"message": "Pod keeps crashing with OOMKilled",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())

			By("discovering workflows")
			result, err = callInvestigate(sess, map[string]any{
				"rr_id":  "rr-disc-003",
				"action": "discover_workflows",
			})
			Expect(err).NotTo(HaveOccurred())
			if result.IsError {
				for _, c := range result.Content {
					if tc, ok := c.(*mcpsdk.TextContent); ok {
						GinkgoWriter.Printf("DISC-003 discover_workflows error: %s\n", tc.Text)
					}
				}
			}
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
				"workflow_id": recommendedWfID(),
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
				"workflow_id": recommendedWfID(),
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

			By("sending initial message to build conversation context")
			result, err = callInvestigate(sess, map[string]any{
				"rr_id":   "rr-disc-004",
				"action":  "message",
				"message": "Pod keeps crashing with OOMKilled",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())

			By("discovering workflows")
			result, err = callInvestigate(sess, map[string]any{
				"rr_id":  "rr-disc-004",
				"action": "discover_workflows",
			})
			Expect(err).NotTo(HaveOccurred())
			if result.IsError {
				for _, c := range result.Content {
					if tc, ok := c.(*mcpsdk.TextContent); ok {
						GinkgoWriter.Printf("DISC-004 discover_workflows error: %s\n", tc.Text)
					}
				}
			}
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
			if result.IsError {
				for _, c := range result.Content {
					if tc, ok := c.(*mcpsdk.TextContent); ok {
						GinkgoWriter.Printf("DISC-006 discover_workflows error: %s\n", tc.Text)
					}
				}
			}
			Expect(result.IsError).To(BeFalse())

			By("completing with no action (should use stored RCA)")
			func() { completer.mu.Lock(); defer completer.mu.Unlock(); completer.completedResult = nil }()
			result, err = callTool(sess, "kubernaut_complete_no_action", map[string]any{
				"rr_id":  "rr-disc-006",
				"reason": "user prefers manual fix",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())
			output, err := decodeOutput(result)
			Expect(err).NotTo(HaveOccurred())
			Expect(output["status"]).To(Equal("completed_no_action"))

			Eventually(completer.getCompletedResult, 5*time.Second, 100*time.Millisecond).ShouldNot(BeNil())
			cr := completer.getCompletedResult()
			Expect(cr.RCASummary).To(
				Equal("Unable to determine specific root cause"),
				"must propagate exact RCA from extraction step, not the complete_no_action fallback")
			Expect(cr.RCASummary).NotTo(
				Equal("Investigation completed without workflow selection"),
				"must NOT use the complete_no_action no-discovery fallback")
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
			func() { completer.mu.Lock(); defer completer.mu.Unlock(); completer.completedResult = nil }()
			result, err = callTool(sess, "kubernaut_complete_no_action", map[string]any{
				"rr_id": "rr-disc-007",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())
			output, err := decodeOutput(result)
			Expect(err).NotTo(HaveOccurred())
			Expect(output["status"]).To(Equal("completed_no_action"))

			Eventually(completer.getCompletedResult, 5*time.Second, 100*time.Millisecond).ShouldNot(BeNil())
			cr := completer.getCompletedResult()
			Expect(cr.Reason).To(Equal("no action needed"),
				"should use default reason when none provided")
			Expect(cr.RCASummary).To(ContainSubstring("without workflow"),
				"should use minimal RCA summary when no prior RCA exists")
		})
	})

	Describe("IT-KA-DISC-008: select alternative workflow propagates per-workflow parameters (#1169)", func() {
		It("should deliver alternative parameters to the completer instead of recommended parameters", func() {
			sess, err := connectMCP(stack.Server, "alice@acme.io")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = sess.Close() }()

			By("starting a session")
			result, err := callInvestigate(sess, map[string]any{
				"rr_id":  "rr-disc-008",
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())

			By("sending initial message to build conversation context")
			result, err = callInvestigate(sess, map[string]any{
				"rr_id":   "rr-disc-008",
				"action":  "message",
				"message": "Pod keeps crashing with OOMKilled",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())

			By("discovering workflows")
			result, err = callInvestigate(sess, map[string]any{
				"rr_id":  "rr-disc-008",
				"action": "discover_workflows",
			})
			Expect(err).NotTo(HaveOccurred())
			if result.IsError {
				for _, c := range result.Content {
					if tc, ok := c.(*mcpsdk.TextContent); ok {
						GinkgoWriter.Printf("DISC-008 discover_workflows error: %s\n", tc.Text)
					}
				}
			}
			Expect(result.IsError).To(BeFalse())

			By("selecting the alternative workflow")
			func() { completer.mu.Lock(); defer completer.mu.Unlock(); completer.completedResult = nil }()
			result, err = callTool(sess, "kubernaut_select_workflow", map[string]any{
				"rr_id":       "rr-disc-008",
				"workflow_id": alternativeWfID(),
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())
			output, err := decodeOutput(result)
			Expect(err).NotTo(HaveOccurred())
			Expect(output["status"]).To(Equal("workflow_selected"))

			By("verifying completer received alternative parameters (#1169)")
			Eventually(completer.getCompletedResult, 5*time.Second, 100*time.Millisecond).ShouldNot(BeNil(),
				"HTTP session completion must occur for alternative workflow selection")
			cr := completer.getCompletedResult()
			Expect(cr.WorkflowID).To(Equal(alternativeWfID()),
				"the selected alternative workflow ID must be on the completed result")
			Expect(cr.Parameters).To(HaveKeyWithValue("REPLICA_COUNT", "3"),
				"alternative workflow parameters must flow through to the completer (#1169)")
			Expect(cr.Parameters).NotTo(HaveKey("MEMORY_LIMIT_NEW"),
				"recommended parameters must not leak when an alternative is selected (#1169)")
		})
	})

	Describe("IT-KA-DISC-009: cross-resource RCA — Kind synced from RCA target to signal (#1374)", func() {
		It("should update signal Kind to match RCA target Kind [BR-INTERACTIVE-010, BR-WORKFLOW-004]", func() {
			// Use a signal resolver that returns ResourceKind: "Pod" while the
			// mock LLM's OOMKilled scenario returns RemediationTarget.Kind: "Deployment".
			// After SyncSignalFromRCA, the signal Kind should be "Deployment".
			crossResourceResolver := &crossResourceSignalResolver{}
			crossResourceStack := newRealMCPTestStackWithDiscoveryAndResolver(
				sharedK8sClient, nsName, defaultRealStackOpts(), completer, crossResourceResolver,
			)
			defer crossResourceStack.Close()

			sess, err := connectMCP(crossResourceStack.Server, "alice@acme.io")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = sess.Close() }()

			By("starting a session")
			result, err := callInvestigate(sess, map[string]any{
				"rr_id":  "rr-disc-009",
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())

			By("sending a message to build conversation context")
			result, err = callInvestigate(sess, map[string]any{
				"rr_id":   "rr-disc-009",
				"action":  "message",
				"message": "Pod keeps crashing with OOMKilled",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())

			By("discovering workflows — signal Kind must be synced from RCA target")
			result, err = callInvestigate(sess, map[string]any{
				"rr_id":  "rr-disc-009",
				"action": "discover_workflows",
			})
			Expect(err).NotTo(HaveOccurred())
			if result.IsError {
				for _, c := range result.Content {
					if tc, ok := c.(*mcpsdk.TextContent); ok {
						GinkgoWriter.Printf("DISC-009 discover_workflows error: %s\n", tc.Text)
					}
				}
			}
			Expect(result.IsError).To(BeFalse())

			By("selecting the recommended workflow")
			func() { completer.mu.Lock(); defer completer.mu.Unlock(); completer.completedResult = nil }()
			result, err = callTool(sess, "kubernaut_select_workflow", map[string]any{
				"rr_id":       "rr-disc-009",
				"workflow_id": recommendedWfID(),
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())

			By("verifying TARGET_RESOURCE_KIND reflects RCA target, not original signal [BR-WORKFLOW-004]")
			Eventually(completer.getCompletedResult, 5*time.Second, 100*time.Millisecond).ShouldNot(BeNil())
			cr := completer.getCompletedResult()
			Expect(cr.Parameters).To(HaveKeyWithValue("TARGET_RESOURCE_KIND", "Deployment"),
				"#1374: TARGET_RESOURCE_KIND must reflect RCA target (Deployment), not original signal (Pod)")
			Expect(cr.Parameters).To(HaveKeyWithValue("TARGET_RESOURCE_API_VERSION", "apps/v1"),
				"#1374: TARGET_RESOURCE_API_VERSION must be set from RCA target")
		})
	})

	Describe("IT-KA-DISC-010: same-resource RCA — GVK unchanged [BR-INTERACTIVE-010]", func() {
		It("should preserve signal Kind when RCA target agrees with signal", func() {
			sess, err := connectMCP(stack.Server, "alice@acme.io")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = sess.Close() }()

			By("starting a session")
			result, err := callInvestigate(sess, map[string]any{
				"rr_id":  "rr-disc-010",
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())

			By("sending a message")
			result, err = callInvestigate(sess, map[string]any{
				"rr_id":   "rr-disc-010",
				"action":  "message",
				"message": "Pod keeps crashing with OOMKilled",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())

			By("discovering workflows — same-resource RCA, no Kind change")
			result, err = callInvestigate(sess, map[string]any{
				"rr_id":  "rr-disc-010",
				"action": "discover_workflows",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())

			By("selecting the recommended workflow")
			func() { completer.mu.Lock(); defer completer.mu.Unlock(); completer.completedResult = nil }()
			result, err = callTool(sess, "kubernaut_select_workflow", map[string]any{
				"rr_id":       "rr-disc-010",
				"workflow_id": recommendedWfID(),
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())

			By("verifying GVK unchanged when signal and RCA agree [BR-INTERACTIVE-010]")
			Eventually(completer.getCompletedResult, 5*time.Second, 100*time.Millisecond).ShouldNot(BeNil())
			cr := completer.getCompletedResult()
			Expect(cr.Parameters).To(HaveKeyWithValue("TARGET_RESOURCE_KIND", "Deployment"),
				"same-resource RCA: Kind should remain Deployment")
			Expect(cr.Parameters).To(HaveKeyWithValue("TARGET_RESOURCE_API_VERSION", "apps/v1"),
				"same-resource RCA: apiVersion should remain apps/v1")
		})
	})
})

// crossResourceSignalResolver returns a signal with ResourceKind "Pod" to
// simulate a cross-resource scenario where the alert targets a Pod but the
// RCA identifies a Deployment as the root cause (#1374).
type crossResourceSignalResolver struct{}

func (d *crossResourceSignalResolver) ResolveSignalContext(_ context.Context, _ string) (*katypes.SignalContext, error) {
	return &katypes.SignalContext{
		Name:         "OOMKilled",
		Severity:     "critical",
		Environment:  "Production",
		Priority:     "P0",
		ResourceKind: "Pod",
		Namespace:    "production",
		ResourceName: "api-server-pod-xyz",
	}, nil
}

// newRealMCPTestStackWithDiscoveryAndResolver is a variant of
// newRealMCPTestStackWithDiscovery that accepts a custom signal resolver,
// enabling cross-resource RCA testing (#1374).
func newRealMCPTestStackWithDiscoveryAndResolver(k8sClient client.Client, namespace string, opts realStackOpts, completer *discoveryHTTPCompleter, resolver mcptools.SignalContextResolver) *realMCPTestStack {
	stack := &realMCPTestStack{
		K8sClient: k8sClient,
		Namespace: namespace,
	}

	logrLogger := logr.Discard()

	llmAdapter, err := langchaingo.New("openai", sharedMockLLMEndpoint, "test-model", "test-key")
	Expect(err).ToNot(HaveOccurred(), "langchaingo adapter should build against Mock LLM at %s", sharedMockLLMEndpoint)
	stack.LLMClient = llmAdapter

	promptBuilder, buildErr := prompt.NewBuilder()
	Expect(buildErr).ToNot(HaveOccurred(), "prompt builder should build")

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

	wfQuerier := wfclient.NewOgenWorkflowQuerier(sharedDSClient)
	catalog := mcpadapters.NewWorkflowCatalogAdapter(wfQuerier)

	investigateOpts := []mcptools.InvestigateOption{
		mcptools.WithRateLimiter(stack.RateLimiter),
		mcptools.WithTimeoutTracker(stack.TimeoutMgr),
		mcptools.WithNotifyFunc(stack.Notifier.Notify),
		mcptools.WithSignalContextResolver(resolver),
		mcptools.WithWorkflowCatalog(catalog),
	}
	investigateTool := mcptools.NewInvestigateTool(stack.SessionMgr, runner, recon, mcptools.NopAutonomousManager{}, investigateOpts...)

	selectTool := mcptools.NewSelectWorkflowTool(catalog, stack.SessionMgr,
		mcptools.WithHTTPSessionCompleter(completer),
		mcptools.WithMutexProvider(investigateTool),
	)

	completeNoActionTool := mcptools.NewCompleteNoActionTool(stack.SessionMgr,
		mcptools.WithCompleteNoActionHTTPCompleter(completer),
		mcptools.WithCompleteNoActionMutexProvider(investigateTool),
	)

	toolDeps := mcpinternal.ToolDeps{
		Investigate:      mcptools.InvestigateRegistration(investigateTool, stack.EventStore, stack.Notifier, logr.Discard()),
		SelectWorkflow:   mcptools.SelectWorkflowRegistration(selectTool, logr.Discard()),
		CompleteNoAction: mcptools.CompleteNoActionRegistration(completeNoActionTool, logr.Discard()),
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

// newRealMCPTestStackWithDiscovery builds a test stack with select_workflow and
// complete_no_action tools wired up, using the REAL investigator via langchaingo
// against the shared Podman Mock LLM container. No stubbed runners.
func newRealMCPTestStackWithDiscovery(k8sClient client.Client, namespace string, opts realStackOpts, completer *discoveryHTTPCompleter) *realMCPTestStack {
	stack := &realMCPTestStack{
		K8sClient: k8sClient,
		Namespace: namespace,
	}

	logrLogger := logr.Discard()

	// Real LLM client via langchaingo -> Podman Mock LLM
	llmAdapter, err := langchaingo.New("openai", sharedMockLLMEndpoint, "test-model", "test-key")
	Expect(err).ToNot(HaveOccurred(), "langchaingo adapter should build against Mock LLM at %s", sharedMockLLMEndpoint)
	stack.LLMClient = llmAdapter

	promptBuilder, buildErr := prompt.NewBuilder()
	Expect(buildErr).ToNot(HaveOccurred(), "prompt builder should build")

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

	resolver := &discoverySignalResolver{}

	// Real catalog backed by DataStorage (#1174). Workflow UUIDs are resolved
	// via the override file mounted into the Mock LLM container.
	wfQuerier := wfclient.NewOgenWorkflowQuerier(sharedDSClient)
	catalog := mcpadapters.NewWorkflowCatalogAdapter(wfQuerier)

	investigateOpts := []mcptools.InvestigateOption{
		mcptools.WithRateLimiter(stack.RateLimiter),
		mcptools.WithTimeoutTracker(stack.TimeoutMgr),
		mcptools.WithNotifyFunc(stack.Notifier.Notify),
		mcptools.WithSignalContextResolver(resolver),
		mcptools.WithWorkflowCatalog(catalog),
	}
	investigateTool := mcptools.NewInvestigateTool(stack.SessionMgr, runner, recon, mcptools.NopAutonomousManager{}, investigateOpts...)

	selectTool := mcptools.NewSelectWorkflowTool(catalog, stack.SessionMgr,
		mcptools.WithHTTPSessionCompleter(completer),
		mcptools.WithMutexProvider(investigateTool),
	)

	completeNoActionTool := mcptools.NewCompleteNoActionTool(stack.SessionMgr,
		mcptools.WithCompleteNoActionHTTPCompleter(completer),
		mcptools.WithCompleteNoActionMutexProvider(investigateTool),
	)

	toolDeps := mcpinternal.ToolDeps{
		Investigate:      mcptools.InvestigateRegistration(investigateTool, stack.EventStore, stack.Notifier, logr.Discard()),
		SelectWorkflow:   mcptools.SelectWorkflowRegistration(selectTool, logr.Discard()),
		CompleteNoAction: mcptools.CompleteNoActionRegistration(completeNoActionTool, logr.Discard()),
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

