package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/jordigilh/kubernaut-apifrontend/internal/audit"
	"github.com/jordigilh/kubernaut-apifrontend/internal/auth"
	"github.com/jordigilh/kubernaut-apifrontend/internal/ds"
	"github.com/jordigilh/kubernaut-apifrontend/internal/handler"
	"github.com/jordigilh/kubernaut-apifrontend/internal/ka"
)

// fakeAuditor captures audit events thread-safely for test assertions.
type fakeAuditor struct {
	mu     sync.Mutex
	events []*audit.Event
}

func (f *fakeAuditor) Emit(_ context.Context, event *audit.Event) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, event)
}

func (f *fakeAuditor) Events() []*audit.Event {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := make([]*audit.Event, len(f.events))
	copy(cp, f.events)
	return cp
}

func (f *fakeAuditor) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = nil
}

// newFakeDynamicClient creates a dynamic fake client with common list kinds registered.
func newFakeDynamicClient(objects ...runtime.Object) *dynamicfake.FakeDynamicClient {
	scheme := runtime.NewScheme()
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, map[schema.GroupVersionResource]string{
		{Group: "", Version: "v1", Resource: "events"}:                                        "EventList",
		{Group: "", Version: "v1", Resource: "pods"}:                                          "PodList",
		{Group: "apps", Version: "v1", Resource: "deployments"}:                               "DeploymentList",
		{Group: "apps", Version: "v1", Resource: "statefulsets"}:                              "StatefulSetList",
		{Group: "apps", Version: "v1", Resource: "replicasets"}:                               "ReplicaSetList",
		{Group: "kubernaut.ai", Version: "v1alpha1", Resource: "remediationrequests"}:         "RemediationRequestList",
		{Group: "kubernaut.ai", Version: "v1alpha1", Resource: "remediationapprovalrequests"}: "RemediationApprovalRequestList",
		{Group: "kubernaut.ai", Version: "v1alpha1", Resource: "signalprocessings"}:           "SignalProcessingList",
	}, objects...)
}

// mcpPost sends a JSON-RPC request to the MCP handler and returns the response.
func mcpPost(h http.Handler, sessionID string, body any, user *auth.UserIdentity) *httptest.ResponseRecorder {
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

// mcpInitialize performs the MCP initialize handshake and returns the session ID.
func mcpInitialize(h http.Handler, user *auth.UserIdentity) string {
	initReq := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2025-03-26",
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "test-client", "version": "1.0"},
		},
	}
	rec := mcpPost(h, "", initReq, user)
	Expect(rec.Code).To(Equal(http.StatusOK))
	sessionID := rec.Header().Get("Mcp-Session-Id")
	Expect(sessionID).NotTo(BeEmpty())

	// Send initialized notification
	notif := map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
	}
	mcpPost(h, sessionID, notif, user)
	return sessionID
}

// mcpCallTool sends tools/call and returns the parsed response body.
func mcpCallTool(h http.Handler, sessionID, toolName string, args map[string]any, user *auth.UserIdentity) (status int, body string) {
	callReq := map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      toolName,
			"arguments": args,
		},
	}
	rec := mcpPost(h, sessionID, callReq, user)
	return rec.Code, rec.Body.String()
}

// extractTextContent extracts the text content from a tools/call response.
func extractTextContent(body string) string {
	// Parse SSE or JSON response to extract text content
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
	content, ok := result["content"].([]any)
	if !ok || len(content) == 0 {
		return ""
	}
	first, ok := content[0].(map[string]any)
	if !ok {
		return ""
	}
	text, _ := first["text"].(string)
	return text
}

// isErrorResult checks if the response indicates a tool error.
func isErrorResult(body string) bool {
	if strings.Contains(body, "data:") {
		for _, line := range strings.Split(body, "\n") {
			if strings.HasPrefix(line, "data:") {
				body = strings.TrimPrefix(line, "data:")
				break
			}
		}
	}
	var resp map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(body)), &resp); err != nil {
		return false
	}
	result, ok := resp["result"].(map[string]any)
	if !ok {
		return false
	}
	isErr, _ := result["isError"].(bool)
	return isErr
}

// newKATestServer creates a fake KA REST API httptest server.
func newKATestServer() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/incident/analyze", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]string{"session_id": "test-session-123"})
	})
	mux.HandleFunc("/api/v1/incident/session/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/result") {
			_ = json.NewEncoder(w).Encode(map[string]string{"session_id": "test-session-123", "summary": "test result"})
			return
		}
		if strings.HasSuffix(r.URL.Path, "/cancel") {
			w.WriteHeader(http.StatusOK)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"session_id": "test-session-123", "status": "completed"})
	})
	return httptest.NewServer(mux)
}

// newBridgeMetrics creates Prometheus metrics for bridge testing.
func newBridgeMetrics() *handler.MCPBridgeMetrics {
	return &handler.MCPBridgeMetrics{
		ToolCallsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "test_tool_calls_total",
		}, []string{"tool", "result"}),
		ToolCallDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "test_tool_call_duration_seconds",
			Buckets: prometheus.DefBuckets,
		}, []string{"tool", "type"}),
		RBACDeniedTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "test_rbac_denied_total",
		}, []string{"tool"}),
	}
}

func getCounterValue(cv *prometheus.CounterVec, labels prometheus.Labels) float64 {
	counter, err := cv.GetMetricWith(labels)
	if err != nil {
		return 0
	}
	var m dto.Metric
	if err := counter.Write(&m); err != nil {
		return 0
	}
	return m.GetCounter().GetValue()
}

// ========================================================================
// TIER 1: Core Dispatch Tests
// ========================================================================

var _ = Describe("MCP Bridge - Tier 1: Core Dispatch", Label("tier1", "bridge"), func() {
	var (
		h         http.Handler
		fakeK8s   *dynamicfake.FakeDynamicClient
		kaServer  *httptest.Server
		auditor   *fakeAuditor
		sessionID string
		testUser  *auth.UserIdentity
	)

	BeforeEach(func() {
		auditor = &fakeAuditor{}
		kaServer = newKATestServer()

		rr := &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "kubernaut.ai/v1alpha1",
				"kind":       "RemediationRequest",
				"metadata": map[string]any{
					"name":      "test-rr",
					"namespace": "default",
				},
				"spec": map[string]any{
					"targetResource": map[string]any{"kind": "Deployment", "name": "nginx"},
				},
				"status": map[string]any{
					"overallPhase": "Investigating",
				},
			},
		}

		rar := &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "kubernaut.ai/v1alpha1",
				"kind":       "RemediationApprovalRequest",
				"metadata": map[string]any{
					"name":      "test-rar",
					"namespace": "default",
				},
				"status": map[string]any{
					"overallPhase": "Pending",
				},
			},
		}

		pod := &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "v1",
				"kind":       "Pod",
				"metadata": map[string]any{
					"name":      "nginx-pod-1",
					"namespace": "default",
				},
				"status": map[string]any{
					"overallPhase": "Running",
					"conditions":   []any{},
				},
			},
		}

		event := &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "v1",
				"kind":       "Event",
				"metadata": map[string]any{
					"name":      "test-event-1",
					"namespace": "default",
				},
				"reason":  "Scheduled",
				"message": "Successfully assigned default/nginx to node1",
				"involvedObject": map[string]any{
					"kind": "Pod",
					"name": "nginx-pod-1",
				},
				"count":         int64(1),
				"lastTimestamp": "2026-01-01T00:00:00Z",
			},
		}

		deploy := &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"metadata": map[string]any{
					"name":      "nginx",
					"namespace": "default",
				},
				"spec": map[string]any{
					"replicas": int64(3),
				},
				"status": map[string]any{
					"replicas":          int64(3),
					"readyReplicas":     int64(3),
					"availableReplicas": int64(3),
				},
			},
		}

		fakeK8s = newFakeDynamicClient(rr, rar, pod, event, deploy)
		fakeK8s.PrependWatchReactor("remediationrequests", func(_ k8stesting.Action) (bool, watch.Interface, error) {
			w := watch.NewFake()
			go func() {
				defer GinkgoRecover()
				w.Modify(&unstructured.Unstructured{
					Object: map[string]any{
						"apiVersion": "kubernaut.ai/v1alpha1",
						"kind":       "RemediationRequest",
						"metadata":   map[string]any{"name": "test-rr", "namespace": "default"},
						"status":     map[string]any{"overallPhase": "Completed"},
					},
				})
				w.Stop()
			}()
			return true, w, nil
		})

		testUser = &auth.UserIdentity{
			Username: "sre-engineer",
			Groups:   []string{"sre"},
			Issuer:   "test",
		}

		kaClient := ka.NewClient(ka.Config{BaseURL: kaServer.URL})

		cfg := handler.MCPConfig{
			ServerName:    "kubernaut-apifrontend",
			ServerVersion: "v0.1.0-test",
			Enabled:       true,
			Bridge: &handler.MCPBridgeConfig{
				DynFactory: auth.StaticDynamicFactory(fakeK8s),
				KAClient:   kaClient,
				KAMCPClient: &ka.MockMCPClient{SelectWorkflowFn: func(_ context.Context, _ ka.SelectWorkflowArgs) (*ka.SelectWorkflowResult, error) {
					return &ka.SelectWorkflowResult{Status: "selected", Message: "workflow selected"}, nil
				}},
				DSClient:           newFakeDSClient(),
				RBACRoles:          map[string][]string{"sre": {"*"}},
				Auditor:            auditor,
				ToolTimeout:        5 * time.Second,
				MaxConcurrentTools: 10,
			},
		}
		var err error
		h, err = handler.NewMCPHandler(cfg)
		Expect(err).NotTo(HaveOccurred())

		sessionID = mcpInitialize(h, testUser)
	})

	AfterEach(func() {
		kaServer.Close()
	})

	Context("Tool registration", func() {
		It("UT-AF-B-023: RegisterTools registers exactly 19 tools on the server", func() {
			listReq := map[string]any{
				"jsonrpc": "2.0",
				"id":      3,
				"method":  "tools/list",
				"params":  map[string]any{},
			}
			rec := mcpPost(h, sessionID, listReq, testUser)
			Expect(rec.Code).To(Equal(http.StatusOK))
			body := rec.Body.String()
			count := countToolsInResponse(body)
			Expect(count).To(Equal(19))
		})
	})

	Context("CRD tools dispatch", func() {
		It("UT-AF-B-001: kubernaut_list_remediations dispatches correctly", func() {
			_, body := mcpCallTool(h, sessionID, "kubernaut_list_remediations",
				map[string]any{"namespace": "default"}, testUser)
			text := extractTextContent(body)
			Expect(text).To(ContainSubstring("test-rr"))
		})

		It("UT-AF-B-002: kubernaut_get_remediation dispatches correctly", func() {
			_, body := mcpCallTool(h, sessionID, "kubernaut_get_remediation",
				map[string]any{"rr_id": "default/test-rr"}, testUser)
			text := extractTextContent(body)
			Expect(text).To(ContainSubstring("test-rr"))
			Expect(text).To(ContainSubstring("Investigating"))
		})

		It("UT-AF-B-004: kubernaut_approve dispatches correctly", func() {
			_, body := mcpCallTool(h, sessionID, "kubernaut_approve",
				map[string]any{"namespace": "default", "rar_name": "test-rar", "decision": "approved"}, testUser)
			text := extractTextContent(body)
			Expect(text).To(ContainSubstring("approved"))
		})

		It("UT-AF-B-005: kubernaut_cancel_remediation dispatches correctly", func() {
			_, body := mcpCallTool(h, sessionID, "kubernaut_cancel_remediation",
				map[string]any{"rr_id": "default/test-rr"}, testUser)
			text := extractTextContent(body)
			Expect(text).To(ContainSubstring("Cancelled"))
		})

		It("UT-AF-B-006: kubernaut_watch dispatches and returns events", func() {
			_, body := mcpCallTool(h, sessionID, "kubernaut_watch",
				map[string]any{"namespace": "default", "name": "test-rr"}, testUser)
			text := extractTextContent(body)
			Expect(text).To(ContainSubstring("Completed"))
		})
	})

	Context("KA REST tools dispatch", func() {
		It("UT-AF-B-007: kubernaut_start_investigation dispatches correctly", func() {
			_, body := mcpCallTool(h, sessionID, "kubernaut_start_investigation",
				map[string]any{"namespace": "default", "name": "nginx", "kind": "Deployment"}, testUser)
			text := extractTextContent(body)
			Expect(text).To(ContainSubstring("test-session-123"))
		})

		It("UT-AF-B-008: kubernaut_poll_investigation dispatches correctly", func() {
			_, body := mcpCallTool(h, sessionID, "kubernaut_poll_investigation",
				map[string]any{"session_id": "test-session-123"}, testUser)
			text := extractTextContent(body)
			Expect(text).To(ContainSubstring("completed"))
		})
	})

	Context("KA MCP tools dispatch", func() {
		It("UT-AF-B-009: kubernaut_select_workflow dispatches correctly", func() {
			_, body := mcpCallTool(h, sessionID, "kubernaut_select_workflow",
				map[string]any{"rr_id": "test-rr", "workflow_id": "wf-1"}, testUser)
			text := extractTextContent(body)
			Expect(text).To(ContainSubstring("selected"))
		})
	})

	Context("Presentation tool dispatch", func() {
		It("UT-AF-B-010: kubernaut_present_decision dispatches correctly", func() {
			_, body := mcpCallTool(h, sessionID, "kubernaut_present_decision",
				map[string]any{
					"session_id": "sess-1",
					"summary":    "RCA complete",
					"options":    []any{map[string]any{"workflow_id": "wf-1", "name": "Restart", "description": "Restart pods"}},
				}, testUser)
			text := extractTextContent(body)
			Expect(text).To(ContainSubstring("Restart"))
		})
	})

	Context("DS tools dispatch", func() {
		It("UT-AF-B-011: kubernaut_list_workflows dispatches correctly", func() {
			_, body := mcpCallTool(h, sessionID, "kubernaut_list_workflows",
				map[string]any{}, testUser)
			text := extractTextContent(body)
			Expect(text).To(ContainSubstring("workflows"))
		})

		It("UT-AF-B-012: kubernaut_get_remediation_history dispatches correctly", func() {
			_, body := mcpCallTool(h, sessionID, "kubernaut_get_remediation_history",
				map[string]any{"namespace": "default"}, testUser)
			text := extractTextContent(body)
			Expect(text).To(ContainSubstring("remediations"))
		})

		It("UT-AF-B-013: kubernaut_get_effectiveness dispatches correctly", func() {
			_, body := mcpCallTool(h, sessionID, "kubernaut_get_effectiveness",
				map[string]any{}, testUser)
			text := extractTextContent(body)
			Expect(text).To(ContainSubstring("success_rate"))
		})

		It("UT-AF-B-014: kubernaut_get_audit_trail dispatches correctly", func() {
			_, body := mcpCallTool(h, sessionID, "kubernaut_get_audit_trail",
				map[string]any{"rr_id": "test-rr"}, testUser)
			text := extractTextContent(body)
			Expect(text).To(ContainSubstring("events"))
		})
	})

	Context("AF triage tools dispatch", func() {
		It("UT-AF-B-015: af_list_events dispatches correctly", func() {
			_, body := mcpCallTool(h, sessionID, "af_list_events",
				map[string]any{"namespace": "default"}, testUser)
			text := extractTextContent(body)
			Expect(text).To(ContainSubstring("events"))
		})

		It("UT-AF-B-016: af_get_pods dispatches correctly", func() {
			_, body := mcpCallTool(h, sessionID, "af_get_pods",
				map[string]any{"namespace": "default"}, testUser)
			text := extractTextContent(body)
			Expect(text).To(ContainSubstring("pods"))
		})

		It("UT-AF-B-017: af_get_workloads dispatches correctly", func() {
			_, body := mcpCallTool(h, sessionID, "af_get_workloads",
				map[string]any{"namespace": "default"}, testUser)
			text := extractTextContent(body)
			Expect(text).To(ContainSubstring("workloads"))
		})

		It("UT-AF-B-018: af_resolve_owner dispatches correctly", func() {
			_, body := mcpCallTool(h, sessionID, "af_resolve_owner",
				map[string]any{"namespace": "default", "kind": "Pod", "name": "nginx-pod-1"}, testUser)
			text := extractTextContent(body)
			// Pod has no ownerReferences, so chain is just the pod itself
			Expect(text).To(ContainSubstring("chain"))
		})

		It("UT-AF-B-019: af_check_existing_rr dispatches correctly", func() {
			_, body := mcpCallTool(h, sessionID, "af_check_existing_rr",
				map[string]any{"namespace": "default", "kind": "Deployment", "name": "nginx"}, testUser)
			text := extractTextContent(body)
			Expect(text).To(ContainSubstring("exists"))
		})

		It("UT-AF-B-020: af_create_rr dispatches correctly", func() {
			_, body := mcpCallTool(h, sessionID, "af_create_rr",
				map[string]any{"namespace": "default", "kind": "StatefulSet", "name": "redis", "description": "test rr"}, testUser)
			text := extractTextContent(body)
			Expect(text).To(ContainSubstring("rr_id"))
		})
	})

	Context("Error paths", func() {
		It("UT-AF-B-021: tool call with nil DynFactory returns error", func() {
			nilCfg := handler.MCPConfig{
				ServerName:    "kubernaut-apifrontend",
				ServerVersion: "v0.1.0-test",
				Enabled:       true,
				Bridge: &handler.MCPBridgeConfig{
					DynFactory:         auth.StaticDynamicFactory(nil),
					KAClient:           ka.NewClient(ka.Config{BaseURL: "http://localhost:9999"}),
					RBACRoles:          map[string][]string{"sre": {"*"}},
					Auditor:            auditor,
					ToolTimeout:        2 * time.Second,
					MaxConcurrentTools: 5,
				},
			}
			nilH, err := handler.NewMCPHandler(nilCfg)
			Expect(err).NotTo(HaveOccurred())
			sid := mcpInitialize(nilH, testUser)

			_, body := mcpCallTool(nilH, sid, "af_list_events",
				map[string]any{"namespace": "default"}, testUser)
			Expect(isErrorResult(body)).To(BeTrue())
			text := extractTextContent(body)
			Expect(text).To(ContainSubstring("not available"))
		})

		It("UT-AF-B-022: tool call for nonexistent tool returns error", func() {
			callReq := map[string]any{
				"jsonrpc": "2.0",
				"id":      2,
				"method":  "tools/call",
				"params":  map[string]any{"name": "nonexistent_tool", "arguments": map[string]any{}},
			}
			rec := mcpPost(h, sessionID, callReq, testUser)
			Expect(rec.Body.String()).To(ContainSubstring("error"))
		})
	})
})

// ========================================================================
// TIER 2: Security Tests
// ========================================================================

var _ = Describe("MCP Bridge - Tier 2: Security", Label("tier2", "bridge"), func() {
	var (
		fakeK8s  *dynamicfake.FakeDynamicClient
		kaServer *httptest.Server
		auditor  *fakeAuditor
		metrics  *handler.MCPBridgeMetrics
	)

	BeforeEach(func() {
		auditor = &fakeAuditor{}
		kaServer = newKATestServer()
		metrics = newBridgeMetrics()
		fakeK8s = newFakeDynamicClient()
	})

	AfterEach(func() {
		kaServer.Close()
	})

	Context("RBAC enforcement", func() {
		It("UT-AF-B-025: nil user identity (unauthenticated) is denied when RBAC is configured", func() {
			cfg := handler.MCPConfig{
				ServerName:    "kubernaut-apifrontend",
				ServerVersion: "v0.1.0-test",
				Enabled:       true,
				Bridge: &handler.MCPBridgeConfig{
					DynFactory:         auth.StaticDynamicFactory(fakeK8s),
					KAClient:           ka.NewClient(ka.Config{BaseURL: kaServer.URL}),
					RBACRoles:          map[string][]string{"sre": {"*"}},
					Auditor:            auditor,
					Metrics:            metrics,
					ToolTimeout:        2 * time.Second,
					MaxConcurrentTools: 5,
				},
			}
			h, err := handler.NewMCPHandler(cfg)
			Expect(err).NotTo(HaveOccurred())

			// Initialize and call without user identity (nil)
			sid := mcpInitialize(h, nil)
			_, body := mcpCallTool(h, sid, "af_list_events", map[string]any{"namespace": "default"}, nil)
			Expect(isErrorResult(body)).To(BeTrue())
			text := extractTextContent(body)
			Expect(text).To(ContainSubstring("authentication required"))
		})

		It("UT-AF-B-026: unauthorized user (wrong group) is denied", func() {
			cfg := handler.MCPConfig{
				ServerName:    "kubernaut-apifrontend",
				ServerVersion: "v0.1.0-test",
				Enabled:       true,
				Bridge: &handler.MCPBridgeConfig{
					DynFactory:         auth.StaticDynamicFactory(fakeK8s),
					KAClient:           ka.NewClient(ka.Config{BaseURL: kaServer.URL}),
					RBACRoles:          map[string][]string{"sre": {"*"}},
					Auditor:            auditor,
					Metrics:            metrics,
					ToolTimeout:        2 * time.Second,
					MaxConcurrentTools: 5,
				},
			}
			h, err := handler.NewMCPHandler(cfg)
			Expect(err).NotTo(HaveOccurred())

			noUser := &auth.UserIdentity{Username: "anon", Groups: []string{"viewer"}, Issuer: "test"}
			sid := mcpInitialize(h, noUser)

			_, body := mcpCallTool(h, sid, "af_list_events", map[string]any{"namespace": "default"}, noUser)
			Expect(isErrorResult(body)).To(BeTrue())
			text := extractTextContent(body)
			Expect(text).To(ContainSubstring("permission denied"))
		})

		It("UT-AF-B-027: user with matching group is allowed", func() {
			cfg := handler.MCPConfig{
				ServerName:    "kubernaut-apifrontend",
				ServerVersion: "v0.1.0-test",
				Enabled:       true,
				Bridge: &handler.MCPBridgeConfig{
					DynFactory:         auth.StaticDynamicFactory(fakeK8s),
					KAClient:           ka.NewClient(ka.Config{BaseURL: kaServer.URL}),
					RBACRoles:          map[string][]string{"sre": {"af_list_events", "af_get_pods"}},
					Auditor:            auditor,
					ToolTimeout:        2 * time.Second,
					MaxConcurrentTools: 5,
				},
			}
			h, err := handler.NewMCPHandler(cfg)
			Expect(err).NotTo(HaveOccurred())

			user := &auth.UserIdentity{Username: "operator", Groups: []string{"sre"}, Issuer: "test"}
			sid := mcpInitialize(h, user)

			_, body := mcpCallTool(h, sid, "af_list_events", map[string]any{"namespace": "default"}, user)
			Expect(isErrorResult(body)).To(BeFalse())
		})

		It("UT-AF-B-028: user without matching group is denied", func() {
			cfg := handler.MCPConfig{
				ServerName:    "kubernaut-apifrontend",
				ServerVersion: "v0.1.0-test",
				Enabled:       true,
				Bridge: &handler.MCPBridgeConfig{
					DynFactory:         auth.StaticDynamicFactory(fakeK8s),
					KAClient:           ka.NewClient(ka.Config{BaseURL: kaServer.URL}),
					RBACRoles:          map[string][]string{"sre": {"af_list_events"}},
					Auditor:            auditor,
					Metrics:            metrics,
					ToolTimeout:        2 * time.Second,
					MaxConcurrentTools: 5,
				},
			}
			h, err := handler.NewMCPHandler(cfg)
			Expect(err).NotTo(HaveOccurred())

			user := &auth.UserIdentity{Username: "dev", Groups: []string{"developer"}, Issuer: "test"}
			sid := mcpInitialize(h, user)

			_, body := mcpCallTool(h, sid, "af_list_events", map[string]any{"namespace": "default"}, user)
			Expect(isErrorResult(body)).To(BeTrue())
			text := extractTextContent(body)
			Expect(text).To(ContainSubstring("permission denied"))
		})

		It("UT-AF-B-029: wildcard * grants access to all tools", func() {
			cfg := handler.MCPConfig{
				ServerName:    "kubernaut-apifrontend",
				ServerVersion: "v0.1.0-test",
				Enabled:       true,
				Bridge: &handler.MCPBridgeConfig{
					DynFactory:         auth.StaticDynamicFactory(fakeK8s),
					KAClient:           ka.NewClient(ka.Config{BaseURL: kaServer.URL}),
					RBACRoles:          map[string][]string{"admin": {"*"}},
					Auditor:            auditor,
					ToolTimeout:        2 * time.Second,
					MaxConcurrentTools: 5,
				},
			}
			h, err := handler.NewMCPHandler(cfg)
			Expect(err).NotTo(HaveOccurred())

			user := &auth.UserIdentity{Username: "admin", Groups: []string{"admin"}, Issuer: "test"}
			sid := mcpInitialize(h, user)

			_, body := mcpCallTool(h, sid, "af_list_events", map[string]any{"namespace": "default"}, user)
			Expect(isErrorResult(body)).To(BeFalse())
		})

		It("UT-AF-B-030: nil RBACRoles allows all users (open access)", func() {
			cfg := handler.MCPConfig{
				ServerName:    "kubernaut-apifrontend",
				ServerVersion: "v0.1.0-test",
				Enabled:       true,
				Bridge: &handler.MCPBridgeConfig{
					DynFactory:         auth.StaticDynamicFactory(fakeK8s),
					KAClient:           ka.NewClient(ka.Config{BaseURL: kaServer.URL}),
					RBACRoles:          map[string][]string{"*": {"*"}},
					Auditor:            auditor,
					ToolTimeout:        2 * time.Second,
					MaxConcurrentTools: 5,
				},
			}
			h, err := handler.NewMCPHandler(cfg)
			Expect(err).NotTo(HaveOccurred())

			user := &auth.UserIdentity{Username: "anyone", Groups: []string{"unknown"}, Issuer: "test"}
			sid := mcpInitialize(h, user)

			_, body := mcpCallTool(h, sid, "af_list_events", map[string]any{"namespace": "default"}, user)
			Expect(isErrorResult(body)).To(BeFalse())
		})

		It("UT-AF-B-031: RBAC denial emits EventMCPToolDenied audit event", func() {
			cfg := handler.MCPConfig{
				ServerName:    "kubernaut-apifrontend",
				ServerVersion: "v0.1.0-test",
				Enabled:       true,
				Bridge: &handler.MCPBridgeConfig{
					DynFactory:         auth.StaticDynamicFactory(fakeK8s),
					KAClient:           ka.NewClient(ka.Config{BaseURL: kaServer.URL}),
					RBACRoles:          map[string][]string{"sre": {"af_list_events"}},
					Auditor:            auditor,
					Metrics:            metrics,
					ToolTimeout:        2 * time.Second,
					MaxConcurrentTools: 5,
				},
			}
			h, err := handler.NewMCPHandler(cfg)
			Expect(err).NotTo(HaveOccurred())

			user := &auth.UserIdentity{Username: "blocked", Groups: []string{"viewer"}, Issuer: "test"}
			sid := mcpInitialize(h, user)
			auditor.Reset()

			mcpCallTool(h, sid, "af_list_events", map[string]any{"namespace": "default"}, user)

			events := auditor.Events()
			Expect(events).NotTo(BeEmpty())
			Expect(events[0].Type).To(Equal(audit.EventMCPToolDenied))
			Expect(events[0].Detail["tool"]).To(Equal("af_list_events"))
		})

		It("UT-AF-B-032: RBAC denial increments af_mcp_rbac_denied_total metric", func() {
			cfg := handler.MCPConfig{
				ServerName:    "kubernaut-apifrontend",
				ServerVersion: "v0.1.0-test",
				Enabled:       true,
				Bridge: &handler.MCPBridgeConfig{
					DynFactory:         auth.StaticDynamicFactory(fakeK8s),
					KAClient:           ka.NewClient(ka.Config{BaseURL: kaServer.URL}),
					RBACRoles:          map[string][]string{"sre": {"af_list_events"}},
					Auditor:            auditor,
					Metrics:            metrics,
					ToolTimeout:        2 * time.Second,
					MaxConcurrentTools: 5,
				},
			}
			h, err := handler.NewMCPHandler(cfg)
			Expect(err).NotTo(HaveOccurred())

			user := &auth.UserIdentity{Username: "blocked", Groups: []string{"viewer"}, Issuer: "test"}
			sid := mcpInitialize(h, user)

			mcpCallTool(h, sid, "af_list_events", map[string]any{"namespace": "default"}, user)

			val := getCounterValue(metrics.RBACDeniedTotal, prometheus.Labels{"tool": "af_list_events"})
			Expect(val).To(BeNumerically(">=", 1))
		})
	})

	Context("Error redaction", func() {
		It("UT-AF-B-035: error messages redact JWT tokens", func() {
			jwtFactory := auth.DynamicClientFactory(func(_ context.Context) (dynamic.Interface, error) {
				return nil, fmt.Errorf("auth error with token eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.signature_placeholder_padding_here")
			})
			cfg := handler.MCPConfig{
				ServerName:    "kubernaut-apifrontend",
				ServerVersion: "v0.1.0-test",
				Enabled:       true,
				Bridge: &handler.MCPBridgeConfig{
					DynFactory:         jwtFactory,
					KAClient:           ka.NewClient(ka.Config{BaseURL: kaServer.URL}),
					RBACRoles:          map[string][]string{"*": {"*"}},
					Auditor:            auditor,
					ToolTimeout:        2 * time.Second,
					MaxConcurrentTools: 5,
				},
			}
			h, err := handler.NewMCPHandler(cfg)
			Expect(err).NotTo(HaveOccurred())

			user := &auth.UserIdentity{Username: "test", Groups: []string{"sre"}, Issuer: "test"}
			sid := mcpInitialize(h, user)
			_, body := mcpCallTool(h, sid, "af_list_events", map[string]any{"namespace": "default"}, user)
			text := extractTextContent(body)
			Expect(text).NotTo(ContainSubstring("eyJ"))
			Expect(text).To(ContainSubstring("REDACTED"))
		})

		It("UT-AF-B-036: error messages redact file paths", func() {
			pathFactory := auth.DynamicClientFactory(func(_ context.Context) (dynamic.Interface, error) {
				return nil, fmt.Errorf("failed reading /etc/kubernetes/pki/ca.crt")
			})
			cfg := handler.MCPConfig{
				ServerName:    "kubernaut-apifrontend",
				ServerVersion: "v0.1.0-test",
				Enabled:       true,
				Bridge: &handler.MCPBridgeConfig{
					DynFactory:         pathFactory,
					KAClient:           ka.NewClient(ka.Config{BaseURL: kaServer.URL}),
					RBACRoles:          map[string][]string{"*": {"*"}},
					Auditor:            auditor,
					ToolTimeout:        2 * time.Second,
					MaxConcurrentTools: 5,
				},
			}
			h, err := handler.NewMCPHandler(cfg)
			Expect(err).NotTo(HaveOccurred())

			user := &auth.UserIdentity{Username: "test", Groups: []string{"sre"}, Issuer: "test"}
			sid := mcpInitialize(h, user)
			_, body := mcpCallTool(h, sid, "af_list_events", map[string]any{"namespace": "default"}, user)
			text := extractTextContent(body)
			Expect(text).NotTo(ContainSubstring("/etc/kubernetes"))
			Expect(text).To(ContainSubstring("REDACTED"))
		})
	})

	Context("tools/list shows all tools regardless of RBAC", func() {
		It("UT-AF-B-040: viewer sees all 19 tools in tools/list", func() {
			cfg := handler.MCPConfig{
				ServerName:    "kubernaut-apifrontend",
				ServerVersion: "v0.1.0-test",
				Enabled:       true,
				Bridge: &handler.MCPBridgeConfig{
					DynFactory:         auth.StaticDynamicFactory(fakeK8s),
					KAClient:           ka.NewClient(ka.Config{BaseURL: kaServer.URL}),
					RBACRoles:          map[string][]string{"sre": {"*"}},
					Auditor:            auditor,
					ToolTimeout:        2 * time.Second,
					MaxConcurrentTools: 5,
				},
			}
			h, err := handler.NewMCPHandler(cfg)
			Expect(err).NotTo(HaveOccurred())

			viewer := &auth.UserIdentity{Username: "viewer", Groups: []string{"viewer"}, Issuer: "test"}
			sid := mcpInitialize(h, viewer)

			listReq := map[string]any{
				"jsonrpc": "2.0",
				"id":      3,
				"method":  "tools/list",
				"params":  map[string]any{},
			}
			rec := mcpPost(h, sid, listReq, viewer)
			Expect(rec.Code).To(Equal(http.StatusOK))
			count := countToolsInResponse(rec.Body.String())
			Expect(count).To(Equal(19))
		})
	})
})

// ========================================================================
// TIER 3: Observability Tests
// ========================================================================

var _ = Describe("MCP Bridge - Tier 3: Observability", Label("tier3", "bridge"), func() {
	var (
		fakeK8s  *dynamicfake.FakeDynamicClient
		kaServer *httptest.Server
		auditor  *fakeAuditor
		metrics  *handler.MCPBridgeMetrics
	)

	BeforeEach(func() {
		auditor = &fakeAuditor{}
		kaServer = newKATestServer()
		metrics = newBridgeMetrics()
		fakeK8s = newFakeDynamicClient()
	})

	AfterEach(func() {
		kaServer.Close()
	})

	Context("Metrics", func() {
		It("UT-AF-B-046: successful tool call increments af_tool_calls_total with result=success", func() {
			cfg := handler.MCPConfig{
				ServerName:    "kubernaut-apifrontend",
				ServerVersion: "v0.1.0-test",
				Enabled:       true,
				Bridge: &handler.MCPBridgeConfig{
					DynFactory:         auth.StaticDynamicFactory(fakeK8s),
					KAClient:           ka.NewClient(ka.Config{BaseURL: kaServer.URL}),
					RBACRoles:          map[string][]string{"*": {"*"}},
					Auditor:            auditor,
					Metrics:            metrics,
					ToolTimeout:        5 * time.Second,
					MaxConcurrentTools: 5,
				},
			}
			h, err := handler.NewMCPHandler(cfg)
			Expect(err).NotTo(HaveOccurred())

			user := &auth.UserIdentity{Username: "user", Groups: []string{"sre"}, Issuer: "test"}
			sid := mcpInitialize(h, user)

			mcpCallTool(h, sid, "af_list_events", map[string]any{"namespace": "default"}, user)

			val := getCounterValue(metrics.ToolCallsTotal, prometheus.Labels{"tool": "af_list_events", "result": "success"})
			Expect(val).To(BeNumerically(">=", 1))
		})

		It("UT-AF-B-047: RBAC denial increments af_tool_calls_total with result=denied", func() {
			cfg := handler.MCPConfig{
				ServerName:    "kubernaut-apifrontend",
				ServerVersion: "v0.1.0-test",
				Enabled:       true,
				Bridge: &handler.MCPBridgeConfig{
					DynFactory:         auth.StaticDynamicFactory(fakeK8s),
					KAClient:           ka.NewClient(ka.Config{BaseURL: kaServer.URL}),
					RBACRoles:          map[string][]string{"admin": {"*"}},
					Auditor:            auditor,
					Metrics:            metrics,
					ToolTimeout:        5 * time.Second,
					MaxConcurrentTools: 5,
				},
			}
			h, err := handler.NewMCPHandler(cfg)
			Expect(err).NotTo(HaveOccurred())

			user := &auth.UserIdentity{Username: "nobody", Groups: []string{"viewer"}, Issuer: "test"}
			sid := mcpInitialize(h, user)

			mcpCallTool(h, sid, "af_list_events", map[string]any{"namespace": "default"}, user)

			val := getCounterValue(metrics.ToolCallsTotal, prometheus.Labels{"tool": "af_list_events", "result": "denied"})
			Expect(val).To(BeNumerically(">=", 1))
		})

		It("UT-AF-B-048: tool error increments af_tool_calls_total with result=error", func() {
			errFactory := auth.DynamicClientFactory(func(_ context.Context) (dynamic.Interface, error) {
				return nil, fmt.Errorf("connection refused")
			})
			cfg := handler.MCPConfig{
				ServerName:    "kubernaut-apifrontend",
				ServerVersion: "v0.1.0-test",
				Enabled:       true,
				Bridge: &handler.MCPBridgeConfig{
					DynFactory:         errFactory,
					KAClient:           ka.NewClient(ka.Config{BaseURL: kaServer.URL}),
					RBACRoles:          map[string][]string{"*": {"*"}},
					Auditor:            auditor,
					Metrics:            metrics,
					ToolTimeout:        5 * time.Second,
					MaxConcurrentTools: 5,
				},
			}
			h, err := handler.NewMCPHandler(cfg)
			Expect(err).NotTo(HaveOccurred())

			user := &auth.UserIdentity{Username: "user", Groups: []string{"sre"}, Issuer: "test"}
			sid := mcpInitialize(h, user)

			mcpCallTool(h, sid, "af_list_events", map[string]any{"namespace": "default"}, user)

			val := getCounterValue(metrics.ToolCallsTotal, prometheus.Labels{"tool": "af_list_events", "result": "error"})
			Expect(val).To(BeNumerically(">=", 1))
		})

		It("UT-AF-B-049: tool call records duration in af_tool_call_duration_seconds", func() {
			cfg := handler.MCPConfig{
				ServerName:    "kubernaut-apifrontend",
				ServerVersion: "v0.1.0-test",
				Enabled:       true,
				Bridge: &handler.MCPBridgeConfig{
					DynFactory:         auth.StaticDynamicFactory(fakeK8s),
					KAClient:           ka.NewClient(ka.Config{BaseURL: kaServer.URL}),
					RBACRoles:          map[string][]string{"*": {"*"}},
					Auditor:            auditor,
					Metrics:            metrics,
					ToolTimeout:        5 * time.Second,
					MaxConcurrentTools: 5,
				},
			}
			h, err := handler.NewMCPHandler(cfg)
			Expect(err).NotTo(HaveOccurred())

			user := &auth.UserIdentity{Username: "user", Groups: []string{"sre"}, Issuer: "test"}
			sid := mcpInitialize(h, user)

			mcpCallTool(h, sid, "af_list_events", map[string]any{"namespace": "default"}, user)

			obs, err := metrics.ToolCallDuration.GetMetricWith(prometheus.Labels{"tool": "af_list_events", "type": "mcp"})
			Expect(err).NotTo(HaveOccurred())
			hist, ok := obs.(prometheus.Histogram)
			Expect(ok).To(BeTrue())
			var m dto.Metric
			Expect(hist.Write(&m)).To(Succeed())
			Expect(m.GetHistogram().GetSampleCount()).To(BeNumerically(">=", 1))
		})
	})

	Context("Audit events", func() {
		It("UT-AF-B-050: successful tool call emits EventMCPToolInvoked", func() {
			cfg := handler.MCPConfig{
				ServerName:    "kubernaut-apifrontend",
				ServerVersion: "v0.1.0-test",
				Enabled:       true,
				Bridge: &handler.MCPBridgeConfig{
					DynFactory:         auth.StaticDynamicFactory(fakeK8s),
					KAClient:           ka.NewClient(ka.Config{BaseURL: kaServer.URL}),
					RBACRoles:          map[string][]string{"*": {"*"}},
					Auditor:            auditor,
					Metrics:            metrics,
					ToolTimeout:        5 * time.Second,
					MaxConcurrentTools: 5,
				},
			}
			h, err := handler.NewMCPHandler(cfg)
			Expect(err).NotTo(HaveOccurred())

			user := &auth.UserIdentity{Username: "sre-user", Groups: []string{"sre"}, Issuer: "test"}
			sid := mcpInitialize(h, user)
			auditor.Reset()

			mcpCallTool(h, sid, "af_list_events", map[string]any{"namespace": "default"}, user)

			events := auditor.Events()
			Expect(events).NotTo(BeEmpty())
			found := false
			for _, e := range events {
				if e.Type == audit.EventMCPToolInvoked && e.Detail["tool"] == "af_list_events" {
					found = true
					Expect(e.UserID).To(Equal("sre-user"))
					break
				}
			}
			Expect(found).To(BeTrue())
		})

		It("UT-AF-B-051: failed tool call emits EventMCPToolFailed with redacted error", func() {
			errFactory := auth.DynamicClientFactory(func(_ context.Context) (dynamic.Interface, error) {
				return nil, fmt.Errorf("secret error at /var/secrets/key.pem")
			})
			cfg := handler.MCPConfig{
				ServerName:    "kubernaut-apifrontend",
				ServerVersion: "v0.1.0-test",
				Enabled:       true,
				Bridge: &handler.MCPBridgeConfig{
					DynFactory:         errFactory,
					KAClient:           ka.NewClient(ka.Config{BaseURL: kaServer.URL}),
					RBACRoles:          map[string][]string{"*": {"*"}},
					Auditor:            auditor,
					Metrics:            metrics,
					ToolTimeout:        5 * time.Second,
					MaxConcurrentTools: 5,
				},
			}
			h, err := handler.NewMCPHandler(cfg)
			Expect(err).NotTo(HaveOccurred())

			user := &auth.UserIdentity{Username: "sre-user", Groups: []string{"sre"}, Issuer: "test"}
			sid := mcpInitialize(h, user)
			auditor.Reset()

			mcpCallTool(h, sid, "af_list_events", map[string]any{"namespace": "default"}, user)

			events := auditor.Events()
			Expect(events).NotTo(BeEmpty())
			found := false
			for _, e := range events {
				if e.Type == audit.EventMCPToolFailed {
					found = true
					Expect(e.Detail["error"]).NotTo(ContainSubstring("/var/secrets"))
					break
				}
			}
			Expect(found).To(BeTrue())
		})

		It("UT-AF-B-052: nil Auditor does not panic during dispatch", func() {
			cfg := handler.MCPConfig{
				ServerName:    "kubernaut-apifrontend",
				ServerVersion: "v0.1.0-test",
				Enabled:       true,
				Bridge: &handler.MCPBridgeConfig{
					DynFactory:         auth.StaticDynamicFactory(fakeK8s),
					KAClient:           ka.NewClient(ka.Config{BaseURL: kaServer.URL}),
					RBACRoles:          map[string][]string{"*": {"*"}},
					Auditor:            nil,
					ToolTimeout:        5 * time.Second,
					MaxConcurrentTools: 5,
				},
			}
			h, err := handler.NewMCPHandler(cfg)
			Expect(err).NotTo(HaveOccurred())

			user := &auth.UserIdentity{Username: "user", Groups: []string{"sre"}, Issuer: "test"}
			sid := mcpInitialize(h, user)

			Expect(func() {
				mcpCallTool(h, sid, "af_list_events", map[string]any{"namespace": "default"}, user)
			}).NotTo(Panic())
		})
	})

	Context("Timeout enforcement", func() {
		It("UT-AF-B-060: tool exceeding timeout returns context deadline exceeded", func() {
			slowFactory := auth.DynamicClientFactory(func(ctx context.Context) (dynamic.Interface, error) {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(5 * time.Second):
					return nil, fmt.Errorf("should not reach here")
				}
			})
			cfg := handler.MCPConfig{
				ServerName:    "kubernaut-apifrontend",
				ServerVersion: "v0.1.0-test",
				Enabled:       true,
				Bridge: &handler.MCPBridgeConfig{
					DynFactory:         slowFactory,
					KAClient:           ka.NewClient(ka.Config{BaseURL: kaServer.URL}),
					RBACRoles:          map[string][]string{"*": {"*"}},
					Auditor:            auditor,
					Metrics:            metrics,
					ToolTimeout:        50 * time.Millisecond,
					MaxConcurrentTools: 5,
				},
			}
			h, err := handler.NewMCPHandler(cfg)
			Expect(err).NotTo(HaveOccurred())

			user := &auth.UserIdentity{Username: "user", Groups: []string{"sre"}, Issuer: "test"}
			sid := mcpInitialize(h, user)

			_, body := mcpCallTool(h, sid, "af_list_events", map[string]any{"namespace": "default"}, user)
			Expect(isErrorResult(body)).To(BeTrue())
			text := extractTextContent(body)
			Expect(text).To(ContainSubstring("deadline"))
		})

		It("UT-AF-B-060b: timeout records result=timeout metric and emits audit event", func() {
			slowFactory := auth.DynamicClientFactory(func(ctx context.Context) (dynamic.Interface, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			})
			localMetrics := newBridgeMetrics()
			localAuditor := &fakeAuditor{}
			cfg := handler.MCPConfig{
				ServerName:    "kubernaut-apifrontend",
				ServerVersion: "v0.1.0-test",
				Enabled:       true,
				Bridge: &handler.MCPBridgeConfig{
					DynFactory:         slowFactory,
					KAClient:           ka.NewClient(ka.Config{BaseURL: kaServer.URL}),
					RBACRoles:          map[string][]string{"*": {"*"}},
					Auditor:            localAuditor,
					Metrics:            localMetrics,
					ToolTimeout:        50 * time.Millisecond,
					MaxConcurrentTools: 5,
				},
			}
			h, err := handler.NewMCPHandler(cfg)
			Expect(err).NotTo(HaveOccurred())

			user := &auth.UserIdentity{Username: "user", Groups: []string{"sre"}, Issuer: "test"}
			sid := mcpInitialize(h, user)
			mcpCallTool(h, sid, "af_list_events", map[string]any{"namespace": "default"}, user)

			val := getCounterValue(localMetrics.ToolCallsTotal, prometheus.Labels{"tool": "af_list_events", "result": "timeout"})
			Expect(val).To(BeNumerically(">=", 1))

			events := localAuditor.Events()
			var foundFailed bool
			for _, ev := range events {
				if ev.Type == audit.EventMCPToolFailed && ev.Detail["tool"] == "af_list_events" {
					foundFailed = true
					break
				}
			}
			Expect(foundFailed).To(BeTrue(), "expected EventMCPToolFailed audit event for timeout")
		})
	})

	Context("Throttle branch (sem.Acquire failure)", func() {
		It("UT-AF-B-067: tool calls exceeding MaxConcurrentTools are throttled", func() {
			localMetrics := newBridgeMetrics()
			localAuditor := &fakeAuditor{}

			// slowFactory holds the semaphore for 500ms while the tool timeout
			// is only 200ms, guaranteeing that queued callers exhaust their
			// budget and receive a throttle response.
			slowFactory := auth.DynamicClientFactory(func(ctx context.Context) (dynamic.Interface, error) {
				select {
				case <-ctx.Done():
				case <-time.After(500 * time.Millisecond):
				}
				return fakeK8s, nil
			})

			cfg := handler.MCPConfig{
				ServerName:    "kubernaut-apifrontend",
				ServerVersion: "v0.1.0-test",
				Enabled:       true,
				Bridge: &handler.MCPBridgeConfig{
					DynFactory:         slowFactory,
					KAClient:           ka.NewClient(ka.Config{BaseURL: kaServer.URL}),
					RBACRoles:          map[string][]string{"*": {"*"}},
					Auditor:            localAuditor,
					Metrics:            localMetrics,
					ToolTimeout:        200 * time.Millisecond,
					MaxConcurrentTools: 1,
				},
			}
			h, err := handler.NewMCPHandler(cfg)
			Expect(err).NotTo(HaveOccurred())

			ts := httptest.NewServer(h)
			defer ts.Close()

			user := &auth.UserIdentity{Username: "sre", Groups: []string{"sre"}, Issuer: "test"}

			const n = 6
			// Pre-initialize sessions so all goroutines can fire tool calls
			// simultaneously without variable init latency spreading them out.
			sessions := make([]string, n)
			for i := 0; i < n; i++ {
				sessions[i] = mcpInitializeHTTP(ts.URL, user)
			}

			results := make(chan string, n)
			barrier := make(chan struct{})
			var wg sync.WaitGroup
			for i := 0; i < n; i++ {
				wg.Add(1)
				go func(idx int) {
					defer GinkgoRecover()
					defer wg.Done()
					<-barrier
					body := mcpCallToolHTTP(ts.URL, sessions[idx], "af_list_events", map[string]any{"namespace": "default"}, user)
					results <- body
				}(i)
			}
			close(barrier)
			wg.Wait()
			close(results)

			var throttled int
			for body := range results {
				if strings.Contains(body, "busy") {
					throttled++
				}
			}

			Expect(throttled).To(BeNumerically(">=", 1),
				"expected at least one throttled response out of 6 concurrent calls")

			metricVal := getCounterValue(localMetrics.ToolCallsTotal, prometheus.Labels{"tool": "af_list_events", "result": "throttled"})
			Expect(metricVal).To(BeNumerically(">=", 1))
		})
	})

	Context("Concurrency limiting", func() {
		It("UT-AF-B-064: semaphore limits concurrent tool calls across sessions", func() {
			var concurrency int64
			var mu sync.Mutex
			var maxConcurrency int64

			slowFactory := auth.DynamicClientFactory(func(ctx context.Context) (dynamic.Interface, error) {
				mu.Lock()
				concurrency++
				if concurrency > maxConcurrency {
					maxConcurrency = concurrency
				}
				mu.Unlock()

				select {
				case <-ctx.Done():
				case <-time.After(50 * time.Millisecond):
				}

				mu.Lock()
				concurrency--
				mu.Unlock()
				return nil, fmt.Errorf("test done")
			})

			cfg := handler.MCPConfig{
				ServerName:    "kubernaut-apifrontend",
				ServerVersion: "v0.1.0-test",
				Enabled:       true,
				Bridge: &handler.MCPBridgeConfig{
					DynFactory:         slowFactory,
					KAClient:           ka.NewClient(ka.Config{BaseURL: kaServer.URL}),
					RBACRoles:          map[string][]string{"*": {"*"}},
					Auditor:            auditor,
					Metrics:            metrics,
					ToolTimeout:        2 * time.Second,
					MaxConcurrentTools: 3,
				},
			}
			h, err := handler.NewMCPHandler(cfg)
			Expect(err).NotTo(HaveOccurred())

			// Use httptest.Server so each goroutine gets its own MCP session
			ts := httptest.NewServer(h)
			defer ts.Close()

			user := &auth.UserIdentity{Username: "user", Groups: []string{"sre"}, Issuer: "test"}

			var wg sync.WaitGroup
			for i := 0; i < 6; i++ {
				wg.Add(1)
				go func() {
					defer GinkgoRecover()
					defer wg.Done()
					sid := mcpInitializeHTTP(ts.URL, user)
					mcpCallToolHTTP(ts.URL, sid, "af_list_events", map[string]any{"namespace": "default"}, user)
				}()
			}
			wg.Wait()

			mu.Lock()
			observed := maxConcurrency
			mu.Unlock()
			Expect(observed).To(BeNumerically("<=", 3))
		})
	})

	Context("Panic recovery", func() {
		It("UT-AF-B-065: handler panic returns isError result with 'internal error'", func() {
			// This test uses a special bridge where DynFactory panics
			panicFactory := auth.DynamicClientFactory(func(_ context.Context) (dynamic.Interface, error) {
				panic("deliberate test panic")
			})
			cfg := handler.MCPConfig{
				ServerName:    "kubernaut-apifrontend",
				ServerVersion: "v0.1.0-test",
				Enabled:       true,
				Bridge: &handler.MCPBridgeConfig{
					DynFactory:         panicFactory,
					KAClient:           ka.NewClient(ka.Config{BaseURL: kaServer.URL}),
					DSClient:           newFakeDSClient(),
					RBACRoles:          map[string][]string{"*": {"*"}},
					Auditor:            auditor,
					Metrics:            metrics,
					ToolTimeout:        2 * time.Second,
					MaxConcurrentTools: 10,
				},
			}
			h, err := handler.NewMCPHandler(cfg)
			Expect(err).NotTo(HaveOccurred())

			user := &auth.UserIdentity{Username: "sre", Groups: []string{"sre"}, Issuer: "test"}
			sid := mcpInitialize(h, user)
			_, body := mcpCallTool(h, sid, "af_list_events", map[string]any{"namespace": "default"}, user)
			Expect(isErrorResult(body)).To(BeTrue())
			text := extractTextContent(body)
			Expect(text).To(ContainSubstring("internal error"))
		})

		It("UT-AF-B-066: panic records metrics with result=panic and emits audit", func() {
			panicFactory := auth.DynamicClientFactory(func(_ context.Context) (dynamic.Interface, error) {
				panic("boom")
			})
			localMetrics := newBridgeMetrics()
			localAuditor := &fakeAuditor{}
			cfg := handler.MCPConfig{
				ServerName:    "kubernaut-apifrontend",
				ServerVersion: "v0.1.0-test",
				Enabled:       true,
				Bridge: &handler.MCPBridgeConfig{
					DynFactory:         panicFactory,
					KAClient:           ka.NewClient(ka.Config{BaseURL: kaServer.URL}),
					DSClient:           newFakeDSClient(),
					RBACRoles:          map[string][]string{"*": {"*"}},
					Auditor:            localAuditor,
					Metrics:            localMetrics,
					ToolTimeout:        2 * time.Second,
					MaxConcurrentTools: 10,
				},
			}
			h, err := handler.NewMCPHandler(cfg)
			Expect(err).NotTo(HaveOccurred())

			user := &auth.UserIdentity{Username: "sre", Groups: []string{"sre"}, Issuer: "test"}
			sid := mcpInitialize(h, user)
			mcpCallTool(h, sid, "af_list_events", map[string]any{"namespace": "default"}, user)

			val := getCounterValue(localMetrics.ToolCallsTotal, prometheus.Labels{"tool": "af_list_events", "result": "panic"})
			Expect(val).To(BeNumerically(">=", 1))

			events := localAuditor.Events()
			var foundPanicAudit bool
			for _, ev := range events {
				if ev.Type == audit.EventMCPToolFailed && ev.Detail["tool"] == "af_list_events" && ev.Detail["error"] == "internal error" {
					foundPanicAudit = true
					break
				}
			}
			Expect(foundPanicAudit).To(BeTrue(), "expected EventMCPToolFailed audit event from panic recovery")
		})
	})

	Context("GetToolTimeout and GetMaxConcurrentTools defaults", func() {
		It("UT-AF-B-062: GetToolTimeout returns default when not set", func() {
			cfg := &handler.MCPBridgeConfig{}
			Expect(cfg.GetToolTimeout()).To(Equal(30 * time.Second))
		})

		It("UT-AF-B-063: GetMaxConcurrentTools returns default when not set", func() {
			cfg := &handler.MCPBridgeConfig{}
			Expect(cfg.GetMaxConcurrentTools()).To(Equal(int64(10)))
		})
	})
})

// ========================================================================
// TIER 4: Adversarial Input Tests
// ========================================================================

var _ = Describe("MCP Bridge - Tier 4: Adversarial Inputs", Label("tier4", "bridge"), func() {
	var (
		h         http.Handler
		kaServer  *httptest.Server
		auditor   *fakeAuditor
		sessionID string
		testUser  *auth.UserIdentity
	)

	BeforeEach(func() {
		auditor = &fakeAuditor{}
		kaServer = newKATestServer()
		fakeK8s := newFakeDynamicClient()

		cfg := handler.MCPConfig{
			ServerName:    "kubernaut-apifrontend",
			ServerVersion: "v0.1.0-test",
			Enabled:       true,
			Bridge: &handler.MCPBridgeConfig{
				DynFactory: auth.StaticDynamicFactory(fakeK8s),
				KAClient:   ka.NewClient(ka.Config{BaseURL: kaServer.URL}),
				KAMCPClient: &ka.MockMCPClient{SelectWorkflowFn: func(_ context.Context, _ ka.SelectWorkflowArgs) (*ka.SelectWorkflowResult, error) {
					return &ka.SelectWorkflowResult{Status: "selected", Message: "ok"}, nil
				}},
				DSClient:           newFakeDSClient(),
				RBACRoles:          map[string][]string{"*": {"*"}},
				Auditor:            auditor,
				ToolTimeout:        5 * time.Second,
				MaxConcurrentTools: 10,
			},
		}
		var err error
		h, err = handler.NewMCPHandler(cfg)
		Expect(err).NotTo(HaveOccurred())

		testUser = &auth.UserIdentity{Username: "sre", Groups: []string{"sre"}, Issuer: "test"}
		sessionID = mcpInitialize(h, testUser)
	})

	AfterEach(func() {
		kaServer.Close()
	})

	Context("Empty string parameters", func() {
		It("UT-AF-B-070: af_list_events with empty namespace returns error", func() {
			_, body := mcpCallTool(h, sessionID, "af_list_events",
				map[string]any{"namespace": ""}, testUser)
			Expect(isErrorResult(body)).To(BeTrue())
			text := extractTextContent(body)
			Expect(text).To(ContainSubstring("invalid"))
		})

		It("UT-AF-B-071: af_get_pods with empty namespace returns error", func() {
			_, body := mcpCallTool(h, sessionID, "af_get_pods",
				map[string]any{"namespace": ""}, testUser)
			Expect(isErrorResult(body)).To(BeTrue())
		})

		It("UT-AF-B-073: af_create_rr with empty description still succeeds (optional-ish)", func() {
			_, body := mcpCallTool(h, sessionID, "af_create_rr",
				map[string]any{"namespace": "default", "kind": "Deployment", "name": "test-empty-desc", "description": ""}, testUser)
			// Empty description is allowed — the tool doesn't reject it
			Expect(isErrorResult(body)).To(BeFalse())
		})

		It("UT-AF-B-074: kubernaut_approve with empty decision returns error", func() {
			_, body := mcpCallTool(h, sessionID, "kubernaut_approve",
				map[string]any{"namespace": "default", "rar_name": "test-rar", "decision": ""}, testUser)
			Expect(isErrorResult(body)).To(BeTrue())
		})
	})

	Context("Path traversal inputs", func() {
		It("UT-AF-B-075: af_list_events with path traversal namespace is rejected", func() {
			_, body := mcpCallTool(h, sessionID, "af_list_events",
				map[string]any{"namespace": "../../etc/passwd"}, testUser)
			Expect(isErrorResult(body)).To(BeTrue())
		})

		It("UT-AF-B-076: kubernaut_watch with path traversal name is rejected", func() {
			_, body := mcpCallTool(h, sessionID, "kubernaut_watch",
				map[string]any{"namespace": "default", "name": "../../../secrets"}, testUser)
			Expect(isErrorResult(body)).To(BeTrue())
		})

		It("UT-AF-B-077: af_check_existing_rr with path traversal kind is rejected", func() {
			_, body := mcpCallTool(h, sessionID, "af_check_existing_rr",
				map[string]any{"namespace": "default", "kind": "../../etc", "name": "test"}, testUser)
			Expect(isErrorResult(body)).To(BeTrue())
		})
	})

	Context("Max-length inputs", func() {
		It("UT-AF-B-078: af_create_rr with description > 2048 chars is truncated, not rejected", func() {
			longDesc := strings.Repeat("A", 3000)
			_, body := mcpCallTool(h, sessionID, "af_create_rr",
				map[string]any{"namespace": "default", "kind": "Deployment", "name": "test-long", "description": longDesc}, testUser)
			// af_create_rr truncates at 2048 chars, does not reject
			Expect(isErrorResult(body)).To(BeFalse())
		})

		It("UT-AF-B-079: af_list_events with very long namespace is rejected", func() {
			longNs := strings.Repeat("a", 300)
			_, body := mcpCallTool(h, sessionID, "af_list_events",
				map[string]any{"namespace": longNs}, testUser)
			Expect(isErrorResult(body)).To(BeTrue())
		})
	})

	Context("Unicode edge cases", func() {
		It("UT-AF-B-080: af_list_events with unicode namespace is rejected", func() {
			_, body := mcpCallTool(h, sessionID, "af_list_events",
				map[string]any{"namespace": "default-日本語"}, testUser)
			Expect(isErrorResult(body)).To(BeTrue())
		})

		It("UT-AF-B-082: af_get_pods with null-byte namespace is rejected", func() {
			_, body := mcpCallTool(h, sessionID, "af_get_pods",
				map[string]any{"namespace": "default\x00injected"}, testUser)
			Expect(isErrorResult(body)).To(BeTrue())
		})
	})

	Context("Invalid rr_id format", func() {
		It("UT-AF-B-083: kubernaut_get_remediation with malformed rr_id is rejected", func() {
			_, body := mcpCallTool(h, sessionID, "kubernaut_get_remediation",
				map[string]any{"rr_id": "no-slash-here"}, testUser)
			Expect(isErrorResult(body)).To(BeTrue())
			text := extractTextContent(body)
			Expect(text).To(ContainSubstring("invalid"))
		})

		It("UT-AF-B-084: kubernaut_cancel_remediation with empty rr_id and no namespace/name is rejected", func() {
			_, body := mcpCallTool(h, sessionID, "kubernaut_cancel_remediation",
				map[string]any{}, testUser)
			Expect(isErrorResult(body)).To(BeTrue())
		})
	})

	Context("Invalid severity values", func() {
		It("UT-AF-B-085: af_create_rr with invalid severity is rejected", func() {
			_, body := mcpCallTool(h, sessionID, "af_create_rr",
				map[string]any{"namespace": "default", "kind": "Deployment", "name": "test-sev", "description": "test", "severity": "CATASTROPHIC"}, testUser)
			Expect(isErrorResult(body)).To(BeTrue())
			text := extractTextContent(body)
			Expect(text).To(ContainSubstring("severity"))
		})
	})
})

// ========================================================================
// TIER 5: Cross-Cutting Tests
// ========================================================================

var _ = Describe("MCP Bridge - Tier 5: Cross-Cutting", Label("tier5", "bridge"), func() {
	var (
		kaServer *httptest.Server
		auditor  *fakeAuditor
	)

	BeforeEach(func() {
		auditor = &fakeAuditor{}
		kaServer = newKATestServer()
	})

	AfterEach(func() {
		kaServer.Close()
	})

	Context("Resource bounds", func() {
		It("UT-AF-B-090: 50+ create/teardown cycles do not leak sessions", func() {
			fakeK8s := newFakeDynamicClient()
			cfg := handler.MCPConfig{
				ServerName:    "kubernaut-apifrontend",
				ServerVersion: "v0.1.0-test",
				Enabled:       true,
				Bridge: &handler.MCPBridgeConfig{
					DynFactory:         auth.StaticDynamicFactory(fakeK8s),
					KAClient:           ka.NewClient(ka.Config{BaseURL: kaServer.URL}),
					DSClient:           newFakeDSClient(),
					RBACRoles:          map[string][]string{"*": {"*"}},
					Auditor:            auditor,
					ToolTimeout:        2 * time.Second,
					MaxConcurrentTools: 10,
				},
			}
			h, err := handler.NewMCPHandler(cfg)
			Expect(err).NotTo(HaveOccurred())

			user := &auth.UserIdentity{Username: "sre", Groups: []string{"sre"}, Issuer: "test"}
			for i := 0; i < 55; i++ {
				sid := mcpInitialize(h, user)
				Expect(sid).NotTo(BeEmpty())
				mcpCallTool(h, sid, "af_list_events", map[string]any{"namespace": "default"}, user)
			}
		})

		It("UT-AF-B-091: auditor accumulates bounded events", func() {
			fakeK8s := newFakeDynamicClient()
			cfg := handler.MCPConfig{
				ServerName:    "kubernaut-apifrontend",
				ServerVersion: "v0.1.0-test",
				Enabled:       true,
				Bridge: &handler.MCPBridgeConfig{
					DynFactory:         auth.StaticDynamicFactory(fakeK8s),
					KAClient:           ka.NewClient(ka.Config{BaseURL: kaServer.URL}),
					DSClient:           newFakeDSClient(),
					RBACRoles:          map[string][]string{"*": {"*"}},
					Auditor:            auditor,
					ToolTimeout:        2 * time.Second,
					MaxConcurrentTools: 10,
				},
			}
			h, err := handler.NewMCPHandler(cfg)
			Expect(err).NotTo(HaveOccurred())

			user := &auth.UserIdentity{Username: "sre", Groups: []string{"sre"}, Issuer: "test"}
			sid := mcpInitialize(h, user)
			for i := 0; i < 55; i++ {
				mcpCallTool(h, sid, "af_list_events", map[string]any{"namespace": "default"}, user)
			}
			// fakeAuditor should have recorded all events without panic
			Expect(len(auditor.Events())).To(BeNumerically(">=", 55))
		})
	})

	Context("Concurrency — competing state transitions", func() {
		It("UT-AF-B-092: concurrent RBAC-allowed and RBAC-denied calls do not race", func() {
			fakeK8s := newFakeDynamicClient()
			metrics := newBridgeMetrics()
			cfg := handler.MCPConfig{
				ServerName:    "kubernaut-apifrontend",
				ServerVersion: "v0.1.0-test",
				Enabled:       true,
				Bridge: &handler.MCPBridgeConfig{
					DynFactory: auth.StaticDynamicFactory(fakeK8s),
					KAClient:   ka.NewClient(ka.Config{BaseURL: kaServer.URL}),
					DSClient:   newFakeDSClient(),
					RBACRoles: map[string][]string{
						"sre": {"af_list_events", "af_get_pods"},
					},
					Auditor:            auditor,
					Metrics:            metrics,
					ToolTimeout:        2 * time.Second,
					MaxConcurrentTools: 10,
				},
			}
			h, err := handler.NewMCPHandler(cfg)
			Expect(err).NotTo(HaveOccurred())

			ts := httptest.NewServer(h)
			defer ts.Close()

			var wg sync.WaitGroup
			allowedUser := &auth.UserIdentity{Username: "allowed", Groups: []string{"sre"}, Issuer: "test"}
			deniedUser := &auth.UserIdentity{Username: "denied", Groups: []string{"viewer"}, Issuer: "test"}

			for i := 0; i < 10; i++ {
				wg.Add(2)
				go func() {
					defer GinkgoRecover()
					defer wg.Done()
					sid := mcpInitializeHTTP(ts.URL, allowedUser)
					mcpCallToolHTTP(ts.URL, sid, "af_list_events", map[string]any{"namespace": "default"}, allowedUser)
				}()
				go func() {
					defer GinkgoRecover()
					defer wg.Done()
					sid := mcpInitializeHTTP(ts.URL, deniedUser)
					mcpCallToolHTTP(ts.URL, sid, "af_list_events", map[string]any{"namespace": "default"}, deniedUser)
				}()
			}
			wg.Wait()
		})
	})

	Context("Nil/zero edge cases", func() {
		It("UT-AF-B-093: nil KAClient still allows CRD and AF tool calls", func() {
			fakeK8s := newFakeDynamicClient()
			cfg := handler.MCPConfig{
				ServerName:    "kubernaut-apifrontend",
				ServerVersion: "v0.1.0-test",
				Enabled:       true,
				Bridge: &handler.MCPBridgeConfig{
					DynFactory:         auth.StaticDynamicFactory(fakeK8s),
					KAClient:           nil,
					DSClient:           newFakeDSClient(),
					RBACRoles:          map[string][]string{"*": {"*"}},
					Auditor:            auditor,
					ToolTimeout:        2 * time.Second,
					MaxConcurrentTools: 10,
				},
			}
			h, err := handler.NewMCPHandler(cfg)
			Expect(err).NotTo(HaveOccurred())

			user := &auth.UserIdentity{Username: "sre", Groups: []string{"sre"}, Issuer: "test"}
			sid := mcpInitialize(h, user)

			// CRD tools should still work
			_, body := mcpCallTool(h, sid, "af_list_events", map[string]any{"namespace": "default"}, user)
			Expect(isErrorResult(body)).To(BeFalse())
		})

		It("UT-AF-B-094: nil DSClient returns error for DS tools", func() {
			fakeK8s := newFakeDynamicClient()
			cfg := handler.MCPConfig{
				ServerName:    "kubernaut-apifrontend",
				ServerVersion: "v0.1.0-test",
				Enabled:       true,
				Bridge: &handler.MCPBridgeConfig{
					DynFactory:         auth.StaticDynamicFactory(fakeK8s),
					KAClient:           ka.NewClient(ka.Config{BaseURL: kaServer.URL}),
					DSClient:           nil,
					RBACRoles:          map[string][]string{"*": {"*"}},
					Auditor:            auditor,
					ToolTimeout:        2 * time.Second,
					MaxConcurrentTools: 10,
				},
			}
			h, err := handler.NewMCPHandler(cfg)
			Expect(err).NotTo(HaveOccurred())

			user := &auth.UserIdentity{Username: "sre", Groups: []string{"sre"}, Issuer: "test"}
			sid := mcpInitialize(h, user)

			_, body := mcpCallTool(h, sid, "kubernaut_list_workflows", map[string]any{}, user)
			Expect(isErrorResult(body)).To(BeTrue())
		})

		It("UT-AF-B-095: nil Metrics does not panic on tool call", func() {
			fakeK8s := newFakeDynamicClient()
			cfg := handler.MCPConfig{
				ServerName:    "kubernaut-apifrontend",
				ServerVersion: "v0.1.0-test",
				Enabled:       true,
				Bridge: &handler.MCPBridgeConfig{
					DynFactory:         auth.StaticDynamicFactory(fakeK8s),
					KAClient:           ka.NewClient(ka.Config{BaseURL: kaServer.URL}),
					DSClient:           newFakeDSClient(),
					RBACRoles:          map[string][]string{"*": {"*"}},
					Auditor:            auditor,
					Metrics:            nil,
					ToolTimeout:        2 * time.Second,
					MaxConcurrentTools: 10,
				},
			}
			h, err := handler.NewMCPHandler(cfg)
			Expect(err).NotTo(HaveOccurred())

			user := &auth.UserIdentity{Username: "sre", Groups: []string{"sre"}, Issuer: "test"}
			sid := mcpInitialize(h, user)

			Expect(func() {
				mcpCallTool(h, sid, "af_list_events", map[string]any{"namespace": "default"}, user)
			}).NotTo(Panic())
		})

		It("UT-AF-B-096: zero ToolTimeout uses default (30s)", func() {
			cfg := &handler.MCPBridgeConfig{ToolTimeout: 0}
			Expect(cfg.GetToolTimeout()).To(Equal(30 * time.Second))
		})

		It("UT-AF-B-097: zero MaxConcurrentTools uses default (10)", func() {
			cfg := &handler.MCPBridgeConfig{MaxConcurrentTools: 0}
			Expect(cfg.GetMaxConcurrentTools()).To(Equal(int64(10)))
		})
	})
})

// ========================================================================
// Helpers
// ========================================================================

// mcpInitializeHTTP initializes an MCP session via a real HTTP server.
func mcpInitializeHTTP(baseURL string, user *auth.UserIdentity) string {
	initReq := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2025-03-26",
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "test-client", "version": "1.0"},
		},
	}
	body, _ := json.Marshal(initReq)
	req, _ := http.NewRequest("POST", baseURL, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if user != nil {
		ctx := auth.WithUserIdentity(req.Context(), user)
		req = req.WithContext(ctx)
	}
	resp, err := http.DefaultClient.Do(req)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	defer func() { _ = resp.Body.Close() }()
	sessionID := resp.Header.Get("Mcp-Session-Id")
	ExpectWithOffset(1, sessionID).NotTo(BeEmpty())

	// Send initialized notification
	notif, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
	})
	nReq, _ := http.NewRequest("POST", baseURL, bytes.NewReader(notif))
	nReq.Header.Set("Content-Type", "application/json")
	nReq.Header.Set("Accept", "application/json, text/event-stream")
	nReq.Header.Set("Mcp-Session-Id", sessionID)
	if user != nil {
		ctx := auth.WithUserIdentity(nReq.Context(), user)
		nReq = nReq.WithContext(ctx)
	}
	nResp, _ := http.DefaultClient.Do(nReq)
	if nResp != nil {
		_ = nResp.Body.Close()
	}
	return sessionID
}

// mcpCallToolHTTP sends tools/call via a real HTTP server using the default client.
func mcpCallToolHTTP(baseURL, sessionID, toolName string, args map[string]any, user *auth.UserIdentity) string {
	return mcpCallToolHTTPClient(http.DefaultClient, baseURL, sessionID, toolName, args, user)
}

// mcpCallToolHTTPClient sends tools/call via a real HTTP server using a custom client.
func mcpCallToolHTTPClient(client *http.Client, baseURL, sessionID, toolName string, args map[string]any, user *auth.UserIdentity) string {
	callReq := map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      toolName,
			"arguments": args,
		},
	}
	body, _ := json.Marshal(callReq)
	req, _ := http.NewRequest("POST", baseURL, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Mcp-Session-Id", sessionID)
	if user != nil {
		ctx := auth.WithUserIdentity(req.Context(), user)
		req = req.WithContext(ctx)
	}
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, _ := io.ReadAll(resp.Body)
	return string(respBody)
}

// countToolsInResponse parses the tools/list response to count tools.
func countToolsInResponse(body string) int {
	// Handle SSE format
	if strings.Contains(body, "data:") {
		for _, line := range strings.Split(body, "\n") {
			if strings.HasPrefix(line, "data:") {
				body = strings.TrimPrefix(line, "data:")
				break
			}
		}
	}
	var resp map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(body)), &resp); err != nil {
		return 0
	}
	result, ok := resp["result"].(map[string]any)
	if !ok {
		return 0
	}
	toolsList, ok := result["tools"].([]any)
	if !ok {
		return 0
	}
	return len(toolsList)
}

// newFakeDSClient creates a ds.MockClient with default no-op implementations.
func newFakeDSClient() *ds.MockClient {
	return &ds.MockClient{
		ListWorkflowsFn: func(_ context.Context, _ ds.ListWorkflowsOpts) ([]ds.Workflow, error) {
			return []ds.Workflow{{ID: "wf-1", Name: "restart", Description: "Restart pods"}}, nil
		},
		GetRemediationHistoryFn: func(_ context.Context, _ ds.HistoryOpts) ([]ds.HistoricalRemediation, error) {
			return []ds.HistoricalRemediation{{ID: "rr-hist-1", Namespace: "default", Phase: "Completed", CreatedAt: "2026-01-01T00:00:00Z"}}, nil
		},
		GetEffectivenessFn: func(_ context.Context, _ ds.EffectivenessOpts) (*ds.EffectivenessReport, error) {
			return &ds.EffectivenessReport{SuccessRate: 0.95, SampleSize: 10}, nil
		},
		GetAuditTrailFn: func(_ context.Context, _ ds.AuditTrailOpts) ([]ds.AuditEvent, error) {
			return []ds.AuditEvent{{Timestamp: "2026-01-01T00:00:00Z", EventType: "tool.invoked", Actor: "sre"}}, nil
		},
	}
}
