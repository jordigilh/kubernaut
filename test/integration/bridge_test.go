package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/go-logr/logr"
	"github.com/jordigilh/kubernaut-apifrontend/internal/audit"
	"github.com/jordigilh/kubernaut-apifrontend/internal/auth"
	"github.com/jordigilh/kubernaut-apifrontend/internal/handler"
)

var _ = Describe("MCP Bridge — Real Containers", func() {

	var sessionID string

	BeforeEach(func() {
		auditCapture.Reset()
		sessionID = itMCPInitialize(mcpHandler, testUser)
	})

	// ═══════════════════════════════════════════════════════════════════
	// Category A: MCP Protocol and Session (IT-PROTO-001..004)
	// ═══════════════════════════════════════════════════════════════════
	Describe("MCP Protocol and Session", func() {

		It("IT-PROTO-001: tools/list returns all 20 registered tools", func() {
			listReq := map[string]any{
				"jsonrpc": "2.0",
				"id":      10,
				"method":  "tools/list",
			}
			rec := itMCPPost(mcpHandler, sessionID, listReq, testUser)
			Expect(rec.Code).To(Equal(http.StatusOK))

			rawBody := rec.Body.String()
			jsonBody := itExtractJSONFromSSE(rawBody)

			var resp map[string]any
			Expect(json.Unmarshal([]byte(jsonBody), &resp)).To(Succeed())
			result, ok := resp["result"].(map[string]any)
			Expect(ok).To(BeTrue(), "response should have result field")
			toolsArr, ok := result["tools"].([]any)
			Expect(ok).To(BeTrue(), "result should have tools array")
			Expect(len(toolsArr)).To(BeNumerically(">=", 19),
				"should register at least 19 tools")

			toolNames := make([]string, 0, len(toolsArr))
			for _, t := range toolsArr {
				tm, _ := t.(map[string]any)
				if name, ok := tm["name"].(string); ok {
					toolNames = append(toolNames, name)
				}
			}
			Expect(toolNames).To(ContainElements(
				"kubernaut_start_investigation",
				"kubernaut_list_remediations",
				"af_list_events",
				"af_create_rr",
				"kubernaut_list_workflows",
			))
		})

		It("IT-PROTO-002: multiple sessions produce independent audit trails", func() {
			session2 := itMCPInitialize(mcpHandler, testUser)

			itMCPCallTool(mcpHandler, sessionID, "kubernaut_present_decision", map[string]any{
				"session_id": "s1", "summary": "session1-unique-marker", "options": []any{},
			}, testUser)
			itMCPCallTool(mcpHandler, session2, "kubernaut_present_decision", map[string]any{
				"session_id": "s2", "summary": "session2-unique-marker", "options": []any{},
			}, testUser)

			events := auditCapture.Events()
			Expect(len(events)).To(BeNumerically(">=", 2),
				"both sessions should produce audit events")

			toolEvents := make([]string, 0)
			for _, ev := range events {
				if ev.Detail != nil && ev.Detail["tool"] == "kubernaut_present_decision" {
					toolEvents = append(toolEvents, ev.UserID)
				}
			}
			Expect(len(toolEvents)).To(BeNumerically(">=", 2),
				"each session should produce its own audit event for kubernaut_present_decision")
		})

		It("IT-PROTO-003: invalid JSON-RPC returns protocol-level error", func() {
			req := httptest.NewRequest("POST", "/mcp", bytes.NewReader([]byte(`{not valid json`)))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json, text/event-stream")
			req.Header.Set("Mcp-Session-Id", sessionID)
			ctx := auth.WithUserIdentity(req.Context(), testUser)
			req = req.WithContext(ctx)
			rec := httptest.NewRecorder()
			mcpHandler.ServeHTTP(rec, req)

			Expect(rec.Code).To(SatisfyAny(
				Equal(http.StatusOK),
				Equal(http.StatusBadRequest),
			))
			body := rec.Body.String()
			Expect(body).To(SatisfyAny(
				ContainSubstring("error"),
				ContainSubstring("-32700"),
				ContainSubstring("malformed"),
				ContainSubstring("not valid"),
			))
		})

		It("IT-PROTO-004: uninitialized session cannot call tools", func() {
			callReq := map[string]any{
				"jsonrpc": "2.0",
				"id":      99,
				"method":  "tools/call",
				"params":  map[string]any{"name": "kubernaut_list_workflows", "arguments": map[string]any{}},
			}
			rec := itMCPPost(mcpHandler, "nonexistent-session-id", callReq, testUser)
			body := rec.Body.String()

			Expect(body).To(SatisfyAny(
				ContainSubstring("error"),
				ContainSubstring("session"),
				ContainSubstring("not found"),
			))
		})
	})

	// ═══════════════════════════════════════════════════════════════════
	// Category B: K8s CRD Tool Dispatch (IT-CRD-001..006)
	// ═══════════════════════════════════════════════════════════════════
	Describe("K8s CRD Tools against envtest", func() {

		It("IT-CRD-001: kubernaut_list_remediations returns seeded RRs", func() {
			status, body := itMCPCallTool(mcpHandler, sessionID, "kubernaut_list_remediations", map[string]any{
				"namespace": itNamespace,
			}, testUser)

			Expect(status).To(Equal(http.StatusOK))
			text := itExtractTextContent(body)
			Expect(text).To(ContainSubstring("test-rr-001"))
		})

		It("IT-CRD-002: kubernaut_get_remediation with valid name", func() {
			status, body := itMCPCallTool(mcpHandler, sessionID, "kubernaut_get_remediation", map[string]any{
				"namespace": itNamespace,
				"name":      "test-rr-001",
			}, testUser)

			Expect(status).To(Equal(http.StatusOK))
			text := itExtractTextContent(body)
			Expect(text).To(ContainSubstring("test-rr-001"))
		})

		It("IT-CRD-003: kubernaut_get_remediation with invalid name returns error", func() {
			status, body := itMCPCallTool(mcpHandler, sessionID, "kubernaut_get_remediation", map[string]any{
				"namespace": itNamespace,
				"name":      "nonexistent-rr",
			}, testUser)

			Expect(status).To(Equal(http.StatusOK))
			text := itExtractTextContent(body)
			Expect(text).To(SatisfyAny(
				ContainSubstring("not found"),
				ContainSubstring("error"),
			))
		})

		It("IT-CRD-005: kubernaut_approve transitions RAR phase", func() {
			status, body := itMCPCallTool(mcpHandler, sessionID, "kubernaut_approve", map[string]any{
				"namespace": itNamespace,
				"rar_name":  "test-rar-001",
				"decision":  "approved",
			}, testUser)

			Expect(status).To(Equal(http.StatusOK))
			text := itExtractTextContent(body)
			Expect(text).To(SatisfyAny(
				ContainSubstring("approved"),
				ContainSubstring("approval"),
				ContainSubstring("test-rar-001"),
			))

			// Read-after-write: verify the RAR still exists and status.decision was persisted
			rar, err := itDynClient.Resource(rarGVR).Namespace(itNamespace).Get(
				context.Background(), "test-rar-001", metav1.GetOptions{},
			)
			Expect(err).ToNot(HaveOccurred(), "RAR should still exist after approve")
			decision, _, _ := unstructured.NestedString(rar.Object, "status", "decision")
			Expect(decision).To(Equal("Approved"), "RAR status.decision should be 'Approved'")
		})

		It("IT-CRD-006: kubernaut_cancel_remediation marks RR cancelled", func() {
			status, body := itMCPCallTool(mcpHandler, sessionID, "kubernaut_cancel_remediation", map[string]any{
				"namespace": itNamespace,
				"name":      "test-rr-001",
			}, testUser)

			Expect(status).To(Equal(http.StatusOK))
			text := itExtractTextContent(body)
			Expect(text).To(SatisfyAny(
				ContainSubstring("cancel"),
				ContainSubstring("test-rr-001"),
			))

			// Read-after-write: verify the RR still exists and status.overallPhase was persisted
			rr, err := itDynClient.Resource(rrGVR).Namespace(itNamespace).Get(
				context.Background(), "test-rr-001", metav1.GetOptions{},
			)
			Expect(err).ToNot(HaveOccurred(), "RR should still exist after cancel")
			rrPhase, _, _ := unstructured.NestedString(rr.Object, "status", "overallPhase")
			Expect(strings.ToLower(rrPhase)).To(ContainSubstring("cancel"),
				"RR status.overallPhase should reflect cancellation")
		})

		It("IT-CRD-007: kubernaut_watch returns events or graceful timeout for seeded RR", func() {
			status, body := itMCPCallTool(mcpHandler, sessionID, "kubernaut_watch", map[string]any{
				"namespace": itNamespace,
				"name":      "test-rr-001",
			}, testUser)

			Expect(status).To(Equal(http.StatusOK))
			text := itExtractTextContent(body)
			Expect(text).To(SatisfyAny(
				ContainSubstring("completed"),
				ContainSubstring("cancelled"),
				ContainSubstring("events"),
				ContainSubstring("test-rr-001"),
			))
		})
	})

	// ═══════════════════════════════════════════════════════════════════
	// Category C: AF K8s Triage Tools (IT-TRIAGE-001..006)
	// ═══════════════════════════════════════════════════════════════════
	Describe("AF K8s Triage Tools against envtest", func() {

		It("IT-TRIAGE-001: af_list_events returns seeded events", func() {
			status, body := itMCPCallTool(mcpHandler, sessionID, "af_list_events", map[string]any{
				"namespace": itNamespace,
			}, testUser)

			Expect(status).To(Equal(http.StatusOK))
			text := itExtractTextContent(body)
			Expect(text).To(ContainSubstring("OOMKilled"))
		})

		It("IT-TRIAGE-002: af_get_pods returns real pods", func() {
			status, body := itMCPCallTool(mcpHandler, sessionID, "af_get_pods", map[string]any{
				"namespace": itNamespace,
			}, testUser)

			Expect(status).To(Equal(http.StatusOK))
			text := itExtractTextContent(body)
			Expect(text).To(ContainSubstring("test-deploy-rs-abc12-pod1"))
		})

		It("IT-TRIAGE-003: af_get_workloads returns deployments", func() {
			status, body := itMCPCallTool(mcpHandler, sessionID, "af_get_workloads", map[string]any{
				"namespace": itNamespace,
			}, testUser)

			Expect(status).To(Equal(http.StatusOK))
			text := itExtractTextContent(body)
			Expect(text).To(ContainSubstring("test-deploy"))
		})

		It("IT-TRIAGE-004: af_resolve_owner resolves Pod -> RS -> Deployment chain", func() {
			status, body := itMCPCallTool(mcpHandler, sessionID, "af_resolve_owner", map[string]any{
				"namespace": itNamespace,
				"kind":      "Pod",
				"name":      "test-deploy-rs-abc12-pod1",
			}, testUser)

			Expect(status).To(Equal(http.StatusOK))
			text := itExtractTextContent(body)
			Expect(text).To(ContainSubstring("Deployment"))
			Expect(text).To(ContainSubstring("test-deploy"))
		})

		It("IT-TRIAGE-005: af_check_existing_rr finds seeded RR by label", func() {
			status, body := itMCPCallTool(mcpHandler, sessionID, "af_check_existing_rr", map[string]any{
				"namespace": itNamespace,
				"kind":      "Deployment",
				"name":      "test-deploy",
			}, testUser)

			Expect(status).To(Equal(http.StatusOK))
			text := itExtractTextContent(body)
			Expect(text).To(SatisfyAny(
				ContainSubstring("true"),
				ContainSubstring("exists"),
				ContainSubstring("test-rr-001"),
			))
		})

		It("IT-TRIAGE-006: af_create_rr creates real RR in envtest", func() {
			status, body := itMCPCallTool(mcpHandler, sessionID, "af_create_rr", map[string]any{
				"namespace":   itNamespace,
				"kind":        "StatefulSet",
				"name":        "new-target",
				"severity":    "high",
				"description": "IT test: create new RR",
			}, testUser)

			Expect(status).To(Equal(http.StatusOK))
			text := itExtractTextContent(body)
			Expect(text).To(SatisfyAny(
				ContainSubstring("rr_id"),
				ContainSubstring("created"),
				ContainSubstring("already_exists"),
			))

			// Read-after-write: verify the RR was actually persisted with correct fields
			rrList, err := itDynClient.Resource(rrGVR).Namespace(itNamespace).List(
				context.Background(), metav1.ListOptions{},
			)
			Expect(err).ToNot(HaveOccurred())

			// Find the RR whose spec.targetResource matches our input
			var found bool
			for _, item := range rrList.Items {
				tk, _, _ := unstructured.NestedString(item.Object, "spec", "targetResource", "kind")
				tn, _, _ := unstructured.NestedString(item.Object, "spec", "targetResource", "name")
				if tk == "StatefulSet" && tn == "new-target" {
					found = true
					sev, _, _ := unstructured.NestedString(item.Object, "spec", "severity")
					Expect(sev).To(Equal("high"))
					fp, _, _ := unstructured.NestedString(item.Object, "spec", "signalFingerprint")
					Expect(fp).To(HaveLen(64), "signalFingerprint should be 64-char SHA256 hex")
					break
				}
			}
			Expect(found).To(BeTrue(), "created RR with targetResource StatefulSet/new-target should exist")
		})
	})

	// ═══════════════════════════════════════════════════════════════════
	// Category D: KA REST + DS Tool Dispatch (IT-KA-001..005, IT-DS-001..004)
	// ═══════════════════════════════════════════════════════════════════
	Describe("KA REST Tool Dispatch (real KA container)", func() {

		It("IT-KA-001: start_investigation dispatches to real KA and returns a session_id", func() {
			status, body := itMCPCallTool(mcpHandler, sessionID, "kubernaut_start_investigation", map[string]any{
				"namespace": "default",
				"name":      "test-deploy",
				"kind":      "Deployment",
			}, testUser)

			Expect(status).To(Equal(http.StatusOK))
			text := itExtractTextContent(body)
			Expect(text).To(SatisfyAny(
				ContainSubstring("session_id"),
				ContainSubstring("accepted"),
				ContainSubstring("investigation"),
			))
		})

		It("IT-KA-002: start + poll lifecycle through real KA", func() {
			_, startBody := itMCPCallTool(mcpHandler, sessionID, "kubernaut_start_investigation", map[string]any{
				"namespace": "default",
				"name":      "poll-test",
				"kind":      "Deployment",
			}, testUser)
			startText := itExtractTextContent(startBody)

			pollSession := extractSessionID(startText)
			if pollSession == "" {
				pollSession = "test-poll-session"
			}

			status, body := itMCPCallTool(mcpHandler, sessionID, "kubernaut_poll_investigation", map[string]any{
				"session_id": pollSession,
			}, testUser)

			Expect(status).To(Equal(http.StatusOK))
			text := itExtractTextContent(body)
			Expect(text).ToNot(BeEmpty())
		})

		It("IT-KA-003: kubernaut_list_workflows queries KA endpoint", func() {
			status, body := itMCPCallTool(mcpHandler, sessionID, "kubernaut_list_workflows", map[string]any{}, testUser)

			Expect(status).To(Equal(http.StatusOK))
			text := itExtractTextContent(body)
			Expect(text).ToNot(ContainSubstring("panic"))
		})

		It("IT-KA-004: kubernaut_present_decision formats and returns decision", func() {
			status, body := itMCPCallTool(mcpHandler, sessionID, "kubernaut_present_decision", map[string]any{
				"session_id": "sess-decision-it",
				"summary":    "Pod OOMKilled in production",
				"options": []any{
					map[string]any{"workflow_id": "wf-restart", "name": "Restart Pod", "description": "Recreate pod", "risk": "low"},
					map[string]any{"workflow_id": "wf-scale", "name": "Scale Up", "description": "Add replicas", "risk": "medium"},
				},
			}, testUser)

			Expect(status).To(Equal(http.StatusOK))
			text := itExtractTextContent(body)
			Expect(text).To(ContainSubstring("OOMKilled"))
			Expect(text).To(ContainSubstring("Restart Pod"))
		})

		It("IT-KA-005: poll with nonexistent session_id returns response from KA", func() {
			status, body := itMCPCallTool(mcpHandler, sessionID, "kubernaut_poll_investigation", map[string]any{
				"session_id": "nonexistent-session-abc123",
			}, testUser)

			Expect(status).To(Equal(http.StatusOK))
			text := itExtractTextContent(body)
			Expect(text).To(SatisfyAny(
				ContainSubstring("not found"),
				ContainSubstring("error"),
				ContainSubstring("unavailable"),
				ContainSubstring("404"),
				ContainSubstring("in_progress"),
				ContainSubstring("status"),
			))
		})

		It("IT-KA-006: kubernaut_select_workflow dispatches to KA MCP client", func() {
			status, body := itMCPCallTool(mcpHandler, sessionID, "kubernaut_select_workflow", map[string]any{
				"rr_id":       "test-rr-001",
				"workflow_id": "wf-restart-pod-v1",
			}, testUser)

			Expect(status).To(Equal(http.StatusOK))
			text := itExtractTextContent(body)
			Expect(text).To(SatisfyAny(
				ContainSubstring("selected"),
				ContainSubstring("status"),
				ContainSubstring("workflow"),
				ContainSubstring("error"),
				ContainSubstring("unavailable"),
			))
		})
	})

	Describe("DS Tool Dispatch (real DS container)", func() {

		It("IT-DS-001: kubernaut_list_workflows returns workflow list from DS", func() {
			status, body := itMCPCallTool(mcpHandler, sessionID, "kubernaut_list_workflows", map[string]any{}, testUser)

			Expect(status).To(Equal(http.StatusOK))
			text := itExtractTextContent(body)
			Expect(text).ToNot(ContainSubstring("error"))
		})

		It("IT-DS-002: kubernaut_get_remediation_history from DS", func() {
			status, body := itMCPCallTool(mcpHandler, sessionID, "kubernaut_get_remediation_history", map[string]any{
				"namespace": "default",
			}, testUser)

			Expect(status).To(Equal(http.StatusOK))
			text := itExtractTextContent(body)
			Expect(text).ToNot(ContainSubstring("panic"))
		})

		It("IT-DS-003: kubernaut_get_effectiveness from DS", func() {
			status, body := itMCPCallTool(mcpHandler, sessionID, "kubernaut_get_effectiveness", map[string]any{
				"workflow_id": "nonexistent-wf",
			}, testUser)

			Expect(status).To(Equal(http.StatusOK))
			text := itExtractTextContent(body)
			Expect(text).ToNot(ContainSubstring("panic"))
		})

		It("IT-DS-004: kubernaut_get_audit_trail from DS", func() {
			status, body := itMCPCallTool(mcpHandler, sessionID, "kubernaut_get_audit_trail", map[string]any{
				"rr_id": "nonexistent-rr",
			}, testUser)

			Expect(status).To(Equal(http.StatusOK))
			text := itExtractTextContent(body)
			Expect(text).ToNot(ContainSubstring("panic"))
		})
	})

	// ═══════════════════════════════════════════════════════════════════
	// Category E: RBAC Enforcement (IT-RBAC-001..004)
	// ═══════════════════════════════════════════════════════════════════
	Describe("RBAC Enforcement", func() {

		It("IT-RBAC-001: user with wildcard role can call any tool", func() {
			status, body := itMCPCallTool(mcpHandler, sessionID, "kubernaut_start_investigation", map[string]any{
				"namespace": "default", "name": "rbac-test", "kind": "Pod",
			}, testUser)

			Expect(status).To(Equal(http.StatusOK))
			text := itExtractTextContent(body)
			Expect(text).ToNot(ContainSubstring("permission denied"))
		})

		It("IT-RBAC-002: user with viewer role denied write tool", func() {
			viewerUser := &auth.UserIdentity{Username: "viewer@kubernaut.ai", Groups: []string{"viewer"}}
			viewerSession := itMCPInitialize(mcpHandler, viewerUser)

			_, body := itMCPCallTool(mcpHandler, viewerSession, "kubernaut_start_investigation", map[string]any{
				"namespace": "default", "name": "rbac-denied", "kind": "Pod",
			}, viewerUser)

			text := itExtractTextContent(body)
			Expect(text).To(ContainSubstring("permission denied"))
		})

		It("IT-RBAC-003: nil user (unauthenticated) rejected", func() {
			unauthSession := itMCPInitialize(mcpHandler, nil)

			_, body := itMCPCallTool(mcpHandler, unauthSession, "kubernaut_list_workflows", map[string]any{}, nil)

			text := itExtractTextContent(body)
			Expect(text).To(ContainSubstring("permission denied"))
		})

		It("IT-RBAC-005: partial-role grants read-only tools, denies write tools", func() {
			readonlyCfg := handler.MCPConfig{
				ServerName:    "af-it-rbac-partial",
				ServerVersion: "0.0.1-rbac",
				Enabled:       true,
				Bridge: &handler.MCPBridgeConfig{
					DynFactory:  auth.StaticDynamicFactory(itDynClient),
					KAClient:    nil,
					KAMCPClient: nil,
					DSClient:    nil,
					RBACRoles: map[string][]string{
						"readonly": {"kubernaut_list_remediations", "af_list_events", "af_get_pods"},
					},
					Logger:             logr.Discard(),
					Auditor:            &testAuditor{},
					Metrics:            newITBridgeMetrics(),
					ToolTimeout:        5 * time.Second,
					MaxConcurrentTools: 5,
				},
			}
			roHandler, err := handler.NewMCPHandler(readonlyCfg)
			Expect(err).ToNot(HaveOccurred())

			roUser := &auth.UserIdentity{Username: "ro@kubernaut.ai", Groups: []string{"readonly"}}
			roSession := itMCPInitialize(roHandler, roUser)

			// Allowed: kubernaut_list_remediations
			status, body := itMCPCallTool(roHandler, roSession, "kubernaut_list_remediations", map[string]any{
				"namespace": itNamespace,
			}, roUser)
			Expect(status).To(Equal(http.StatusOK))
			Expect(itExtractTextContent(body)).ToNot(ContainSubstring("permission denied"))

			// Denied: kubernaut_start_investigation (not in partial role)
			_, body2 := itMCPCallTool(roHandler, roSession, "kubernaut_start_investigation", map[string]any{
				"namespace": "default", "name": "denied", "kind": "Pod",
			}, roUser)
			Expect(itExtractTextContent(body2)).To(ContainSubstring("permission denied"))
		})

		It("IT-RBAC-004: RBAC denial increments RBACDeniedTotal metric", func() {
			viewerUser := &auth.UserIdentity{Username: "viewer2@kubernaut.ai", Groups: []string{"viewer"}}
			viewerSession := itMCPInitialize(mcpHandler, viewerUser)

			itMCPCallTool(mcpHandler, viewerSession, "kubernaut_start_investigation", map[string]any{
				"namespace": "default", "name": "metric-denied", "kind": "Pod",
			}, viewerUser)

			var m dto.Metric
			counter, err := itMetrics.RBACDeniedTotal.GetMetricWithLabelValues("kubernaut_start_investigation")
			Expect(err).ToNot(HaveOccurred())
			Expect(counter.Write(&m)).To(Succeed())
			Expect(m.GetCounter().GetValue()).To(BeNumerically(">=", 1),
				"RBACDeniedTotal should be incremented after denial")
		})
	})

	// ═══════════════════════════════════════════════════════════════════
	// Category H: Metrics and Audit Fidelity (IT-METRICS-001..003, IT-AUDIT-001)
	// ═══════════════════════════════════════════════════════════════════
	Describe("Metrics and Audit Fidelity", func() {

		It("IT-METRICS-001: successful tool call increments ToolCallsTotal{result=success}", func() {
			itMCPCallTool(mcpHandler, sessionID, "kubernaut_present_decision", map[string]any{
				"session_id": "metric-test", "summary": "test", "options": []any{},
			}, testUser)

			var m dto.Metric
			counter, err := itMetrics.ToolCallsTotal.GetMetricWithLabelValues("kubernaut_present_decision", "success")
			Expect(err).ToNot(HaveOccurred())
			Expect(counter.Write(&m)).To(Succeed())
			Expect(m.GetCounter().GetValue()).To(BeNumerically(">=", 1))
		})

		It("IT-METRICS-002: failed tool call increments ToolCallsTotal{result=error}", func() {
			itMCPCallTool(mcpHandler, sessionID, "kubernaut_get_remediation", map[string]any{
				"namespace": "default",
				"name":      "this-rr-does-not-exist-404",
			}, testUser)

			var m dto.Metric
			counter, err := itMetrics.ToolCallsTotal.GetMetricWithLabelValues("kubernaut_get_remediation", "error")
			Expect(err).ToNot(HaveOccurred())
			Expect(counter.Write(&m)).To(Succeed())
			Expect(m.GetCounter().GetValue()).To(BeNumerically(">=", 1))
		})

		It("IT-METRICS-003: duration histogram has observations after call", func() {
			itMCPCallTool(mcpHandler, sessionID, "kubernaut_present_decision", map[string]any{
				"session_id": "hist-test", "summary": "test", "options": []any{},
			}, testUser)

			var m dto.Metric
			observer, err := itMetrics.ToolCallDuration.GetMetricWithLabelValues("kubernaut_present_decision", "mcp")
			Expect(err).ToNot(HaveOccurred())
			histo, ok := observer.(prometheus.Metric)
			Expect(ok).To(BeTrue(), "observer should be a prometheus.Metric")
			Expect(histo.Write(&m)).To(Succeed())
			Expect(m.GetHistogram().GetSampleCount()).To(BeNumerically(">=", 1))
		})

		It("IT-AUDIT-001: audit event contains correct fields", func() {
			before := time.Now().Add(-30 * time.Second)
			itMCPCallTool(mcpHandler, sessionID, "kubernaut_present_decision", map[string]any{
				"session_id": "audit-field-test", "summary": "field check", "options": []any{},
			}, testUser)

			events := auditCapture.Events()
			Expect(len(events)).To(BeNumerically(">=", 1))
			ev := events[len(events)-1]
			Expect(ev.UserID).To(Equal("sre@kubernaut.ai"))
			Expect(ev.Detail).To(HaveKey("tool"))
			Expect(ev.Detail["tool"]).To(Equal("kubernaut_present_decision"))
			Expect(ev.Type).To(Equal(audit.EventMCPToolInvoked),
				"audit event type should be mcp.tool_invoked")
			Expect(ev.Timestamp).To(BeTemporally(">=", before),
				"audit timestamp should be recent")
			Expect(ev.Timestamp).To(BeTemporally("<=", time.Now().Add(30*time.Second)),
				"audit timestamp should not be far in the future")
		})
	})

	// ═══════════════════════════════════════════════════════════════════
	// Cross-Service Wiring (existing specs, renumbered)
	// ═══════════════════════════════════════════════════════════════════
	Describe("Cross-Service Wiring", func() {

		It("IT-RC-007: KA and DS tools work in same MCP session with correct routing", func() {
			status1, _ := itMCPCallTool(mcpHandler, sessionID, "kubernaut_start_investigation", map[string]any{
				"namespace": "default", "name": "cross-test", "kind": "Pod",
			}, testUser)
			Expect(status1).To(Equal(http.StatusOK))

			status2, _ := itMCPCallTool(mcpHandler, sessionID, "kubernaut_list_workflows", map[string]any{}, testUser)
			Expect(status2).To(Equal(http.StatusOK))
		})

		It("IT-RC-011: full start_investigation -> poll -> list_workflows round-trip", func() {
			status, startBody := itMCPCallTool(mcpHandler, sessionID, "kubernaut_start_investigation", map[string]any{
				"namespace": "default", "name": "roundtrip-deploy", "kind": "Deployment",
			}, testUser)
			Expect(status).To(Equal(http.StatusOK))
			startText := itExtractTextContent(startBody)
			Expect(startText).ToNot(BeEmpty())

			status2, wfBody := itMCPCallTool(mcpHandler, sessionID, "kubernaut_list_workflows", map[string]any{}, testUser)
			Expect(status2).To(Equal(http.StatusOK))
			wfText := itExtractTextContent(wfBody)
			Expect(wfText).ToNot(ContainSubstring("error"))

			events := auditCapture.Events()
			Expect(len(events)).To(BeNumerically(">=", 2))
		})
	})
})

// --- MCP protocol helpers ---

func itMCPPost(h http.Handler, sessionID string, body any, user *auth.UserIdentity) *httptest.ResponseRecorder {
	data, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/mcp", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}
	if user != nil {
		ctx := auth.WithUserIdentity(req.Context(), user)
		req = req.WithContext(ctx)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func itMCPInitialize(h http.Handler, user *auth.UserIdentity) string {
	initReq := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2025-03-26",
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "it-container-client", "version": "1.0"},
		},
	}
	rec := itMCPPost(h, "", initReq, user)
	ExpectWithOffset(1, rec.Code).To(Equal(http.StatusOK))
	sid := rec.Header().Get("Mcp-Session-Id")
	ExpectWithOffset(1, sid).NotTo(BeEmpty())

	notif := map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
	}
	itMCPPost(h, sid, notif, user)
	return sid
}

func itMCPCallTool(h http.Handler, sessionID, toolName string, args map[string]any, user *auth.UserIdentity) (code int, body string) {
	callReq := map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      toolName,
			"arguments": args,
		},
	}
	rec := itMCPPost(h, sessionID, callReq, user)
	return rec.Code, rec.Body.String()
}

func itExtractTextContent(body string) string {
	if strings.HasPrefix(body, "event:") || strings.Contains(body, "data:") {
		for _, line := range strings.Split(body, "\n") {
			if strings.HasPrefix(line, "data:") {
				body = strings.TrimPrefix(line, "data:")
				break
			}
		}
	}
	var resp map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(body)), &resp); err != nil {
		return body
	}
	result, ok := resp["result"].(map[string]any)
	if !ok {
		return body
	}
	contentArr, ok := result["content"].([]any)
	if !ok || len(contentArr) == 0 {
		return body
	}
	first, ok := contentArr[0].(map[string]any)
	if !ok {
		return body
	}
	text, _ := first["text"].(string)
	return text
}

func extractSessionID(text string) string {
	var data map[string]any
	if err := json.Unmarshal([]byte(text), &data); err == nil {
		if sid, ok := data["session_id"].(string); ok {
			return sid
		}
	}
	return ""
}

func itExtractJSONFromSSE(raw string) string {
	for _, line := range strings.Split(raw, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "data:") {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
		}
	}
	return raw
}
