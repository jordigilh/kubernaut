package handler_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/ds"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/handler"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
)

var _ = Describe("MCP Bridge Integration (httptest backends)", func() {

	var (
		kaServer  *httptest.Server
		h         http.Handler
		sessionID string
		testUser  *auth.UserIdentity
		auditor   *fakeAuditor
	)

	setupStackWithKAHandler := func(kaHandler http.Handler, dsClient ds.Client) {
		kaServer = httptest.NewServer(kaHandler)

		kaClient := ka.NewClient(ka.Config{
			BaseURL:            kaServer.URL,
			Timeout:            5 * time.Second,
			CBFailureThreshold: 5,
			CBMaxRequests:      3,
			CBInterval:         10 * time.Second,
			CBTimeout:          100 * time.Millisecond,
			RetryMax:           1,
			RetryInitBackoff:   1 * time.Millisecond,
			RetryMaxBackoff:    5 * time.Millisecond,
			RetryableStatuses:  []int{503},
		})

		auditor = &fakeAuditor{}
		testUser = &auth.UserIdentity{Username: "sre@kubernaut.ai", Groups: []string{"sre"}}
		fakeK8s := newFakeDynamicClient()

		cfg := handler.MCPConfig{
			ServerName:    "af-it",
			ServerVersion: "0.0.1-test",
			Enabled:       true,
			Bridge: &handler.MCPBridgeConfig{
				K8sClient: fakeK8s,
				KAClient:   kaClient,
				KAMCPClient: &ka.MockMCPClient{
					SelectWorkflowFn: func(_ context.Context, _ ka.SelectWorkflowArgs) (*ka.SelectWorkflowResult, error) {
						return &ka.SelectWorkflowResult{Status: "selected", Message: "workflow selected"}, nil
					},
				},
				DSClient:           dsClient,
				Authorizer:         &mapAuthorizer{roles: map[string][]string{"sre": {"*"}}},
				Logger:             logr.Discard(),
				Auditor:            auditor,
				Metrics:            newBridgeMetrics(),
				ToolTimeout:        5 * time.Second,
				MaxConcurrentTools: 10,
			},
		}

		var err error
		h, err = handler.NewMCPHandler(cfg)
		Expect(err).NotTo(HaveOccurred())

		sessionID = mcpInitialize(h, testUser)
	}

	AfterEach(func() {
		if kaServer != nil {
			kaServer.Close()
		}
	})

	Describe("KA REST Tool Dispatch (real HTTP)", func() {

		It("IT-BRIDGE-001: kubernaut_investigate dispatches POST to KA /analyze", func() {
			var capturedPath, capturedMethod string
			setupStackWithKAHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost && r.URL.Path == "/api/v1/incident/analyze" {
					capturedPath = r.URL.Path
					capturedMethod = r.Method
					w.WriteHeader(http.StatusAccepted)
					_ = json.NewEncoder(w).Encode(map[string]string{"session_id": "it-sess-001"})
					return
				}
				if strings.HasSuffix(r.URL.Path, "/stream") {
					w.Header().Set("Content-Type", "text/event-stream")
					w.WriteHeader(http.StatusOK)
					_, _ = fmt.Fprintf(w, "event: complete\ndata: {\"type\":\"complete\",\"summary\":\"done\"}\n\n")
					return
				}
				w.WriteHeader(http.StatusNotFound)
			}), newFakeDSClient())

			status, body := mcpCallTool(h, sessionID, "kubernaut_investigate", map[string]any{
				"namespace": "production",
				"name":      "api-gw",
				"kind":      "Deployment",
			}, testUser)

			Expect(status).To(Equal(http.StatusOK))
			Expect(capturedPath).To(Equal("/api/v1/incident/analyze"))
			Expect(capturedMethod).To(Equal(http.MethodPost))
			text := extractTextContent(body)
			Expect(text).To(SatisfyAny(
				ContainSubstring("it-sess-001"),
				ContainSubstring("completed"),
				ContainSubstring("done"),
			))
		})

		It("IT-BRIDGE-002: kubernaut_investigate with completed session_id returns summary", func() {
			setupStackWithKAHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/v1/incident/session/sess-002":
					_ = json.NewEncoder(w).Encode(ka.SessionStatus{SessionID: "sess-002", Status: "completed"})
				case "/api/v1/incident/session/sess-002/result":
					_ = json.NewEncoder(w).Encode(ka.IncidentResponse{SessionID: "sess-002", Summary: "Pod OOMKilled"})
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}), newFakeDSClient())

			status, body := mcpCallTool(h, sessionID, "kubernaut_investigate", map[string]any{
				"session_id": "sess-002",
			}, testUser)

			Expect(status).To(Equal(http.StatusOK))
			text := extractTextContent(body)
			Expect(text).To(ContainSubstring("completed"))
		})

		It("IT-BRIDGE-003: kubernaut_investigate with in_progress is interrupted by tool timeout", func() {
			setupStackWithKAHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/stream") {
					w.Header().Set("Content-Type", "text/event-stream")
					w.WriteHeader(http.StatusOK)
					// Block without sending complete — tool timeout should fire.
					<-r.Context().Done()
					return
				}
				_ = json.NewEncoder(w).Encode(ka.SessionStatus{SessionID: "sess-003", Status: "investigating"})
			}), newFakeDSClient())

			status, body := mcpCallTool(h, sessionID, "kubernaut_investigate", map[string]any{
				"session_id": "sess-003",
			}, testUser)

			Expect(status).To(Equal(http.StatusOK))
			text := extractTextContent(body)
			// Tool timeout (5s) interrupts the blocking SSE stream before completion.
			Expect(text).To(SatisfyAny(
				ContainSubstring("in_progress"),
				ContainSubstring("deadline"),
				ContainSubstring("timeout"),
				ContainSubstring("investigation stream"),
				ContainSubstring("investigation service"),
			))
		})
	})

	Describe("DS Tool Dispatch", func() {

		It("IT-BRIDGE-004: kubernaut_list_workflows dispatches to DS and returns workflow list", func() {
			mockDS := &ds.MockClient{
				ListWorkflowsFn: func(_ context.Context, _ ds.ListWorkflowsOpts) ([]ds.Workflow, error) {
					return []ds.Workflow{
						{ID: "wf-restart", Name: "Restart Pod", Kind: "Deployment"},
						{ID: "wf-scale", Name: "Scale Up", Kind: "Deployment"},
					}, nil
				},
				GetRemediationHistoryFn: func(_ context.Context, _ ds.HistoryOpts) ([]ds.HistoricalRemediation, error) {
					return nil, nil
				},
				GetEffectivenessFn: func(_ context.Context, _ ds.EffectivenessOpts) (*ds.EffectivenessReport, error) {
					return &ds.EffectivenessReport{}, nil
				},
				GetAuditTrailFn: func(_ context.Context, _ ds.AuditTrailOpts) ([]ds.AuditEvent, error) {
					return nil, nil
				},
			}

			setupStackWithKAHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			}), mockDS)

			status, body := mcpCallTool(h, sessionID, "kubernaut_list_workflows", map[string]any{}, testUser)

			Expect(status).To(Equal(http.StatusOK))
			text := extractTextContent(body)
			Expect(text).To(ContainSubstring("wf-restart"))
			Expect(text).To(ContainSubstring("Scale Up"))
		})

		It("IT-BRIDGE-005: kubernaut_get_remediation_history dispatches to DS", func() {
			mockDS := &ds.MockClient{
				ListWorkflowsFn: func(_ context.Context, _ ds.ListWorkflowsOpts) ([]ds.Workflow, error) {
					return nil, nil
				},
				GetRemediationHistoryFn: func(_ context.Context, _ ds.HistoryOpts) ([]ds.HistoricalRemediation, error) {
					return []ds.HistoricalRemediation{
						{ID: "rem-001", Namespace: "prod", Phase: "Succeeded", Workflow: "restart"},
					}, nil
				},
				GetEffectivenessFn: func(_ context.Context, _ ds.EffectivenessOpts) (*ds.EffectivenessReport, error) {
					return &ds.EffectivenessReport{}, nil
				},
				GetAuditTrailFn: func(_ context.Context, _ ds.AuditTrailOpts) ([]ds.AuditEvent, error) {
					return nil, nil
				},
			}

			setupStackWithKAHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			}), mockDS)

			status, body := mcpCallTool(h, sessionID, "kubernaut_get_remediation_history", map[string]any{
				"namespace": "prod",
			}, testUser)

			Expect(status).To(Equal(http.StatusOK))
			text := extractTextContent(body)
			Expect(text).To(ContainSubstring("rem-001"))
			Expect(text).To(ContainSubstring("Succeeded"))
		})

		It("IT-BRIDGE-006: kubernaut_get_effectiveness dispatches to DS", func() {
			mockDS := &ds.MockClient{
				ListWorkflowsFn: func(_ context.Context, _ ds.ListWorkflowsOpts) ([]ds.Workflow, error) {
					return nil, nil
				},
				GetRemediationHistoryFn: func(_ context.Context, _ ds.HistoryOpts) ([]ds.HistoricalRemediation, error) {
					return nil, nil
				},
				GetEffectivenessFn: func(_ context.Context, _ ds.EffectivenessOpts) (*ds.EffectivenessReport, error) {
					return &ds.EffectivenessReport{WorkflowID: "wf-restart", SuccessRate: 0.92, SampleSize: 50}, nil
				},
				GetAuditTrailFn: func(_ context.Context, _ ds.AuditTrailOpts) ([]ds.AuditEvent, error) {
					return nil, nil
				},
			}

			setupStackWithKAHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			}), mockDS)

			status, body := mcpCallTool(h, sessionID, "kubernaut_get_effectiveness", map[string]any{
				"workflow_id": "wf-restart",
			}, testUser)

			Expect(status).To(Equal(http.StatusOK))
			text := extractTextContent(body)
			Expect(text).To(ContainSubstring("0.92"))
		})

		It("IT-BRIDGE-007: kubernaut_get_audit_trail dispatches to DS", func() {
			mockDS := &ds.MockClient{
				ListWorkflowsFn: func(_ context.Context, _ ds.ListWorkflowsOpts) ([]ds.Workflow, error) {
					return nil, nil
				},
				GetRemediationHistoryFn: func(_ context.Context, _ ds.HistoryOpts) ([]ds.HistoricalRemediation, error) {
					return nil, nil
				},
				GetEffectivenessFn: func(_ context.Context, _ ds.EffectivenessOpts) (*ds.EffectivenessReport, error) {
					return &ds.EffectivenessReport{}, nil
				},
				GetAuditTrailFn: func(_ context.Context, _ ds.AuditTrailOpts) ([]ds.AuditEvent, error) {
					return []ds.AuditEvent{
						{EventType: "remediation.approved", Actor: "admin", Timestamp: "2026-05-14T12:00:00Z"},
					}, nil
				},
			}

			setupStackWithKAHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			}), mockDS)

			status, body := mcpCallTool(h, sessionID, "kubernaut_get_audit_trail", map[string]any{
				"rr_id": "rem-001",
			}, testUser)

			Expect(status).To(Equal(http.StatusOK))
			text := extractTextContent(body)
			Expect(text).To(ContainSubstring("remediation.approved"))
		})
	})

	Describe("Present Decision Tool", func() {

		It("IT-BRIDGE-014: kubernaut_present_decision formats summary and options for user", func() {
			setupStackWithKAHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			}), newFakeDSClient())

			status, body := mcpCallTool(h, sessionID, "kubernaut_present_decision", map[string]any{
				"session_id": "sess-decision-it",
				"summary":    "Pod crash-looping due to OOMKilled",
				"options": []any{
					map[string]any{"workflow_id": "wf-restart", "name": "Restart Pod", "description": "Recreate pod", "risk": "low"},
					map[string]any{"workflow_id": "wf-scale", "name": "Scale Up", "description": "Add replicas", "risk": "medium"},
				},
			}, testUser)

			Expect(status).To(Equal(http.StatusOK))
			text := extractTextContent(body)
			Expect(text).To(ContainSubstring("OOMKilled"))
			Expect(text).To(ContainSubstring("Restart Pod"))
			Expect(text).To(ContainSubstring("Scale Up"))
			Expect(text).To(ContainSubstring("presented"))
		})

		It("IT-BRIDGE-015: kubernaut_present_decision emits audit event", func() {
			setupStackWithKAHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			}), newFakeDSClient())

			auditor.Reset()

			mcpCallTool(h, sessionID, "kubernaut_present_decision", map[string]any{
				"session_id": "sess-audit-decision",
				"summary":    "test summary",
				"options":    []any{},
			}, testUser)

			events := auditor.Events()
			Expect(len(events)).To(BeNumerically(">=", 1),
				"present_decision should emit at least one audit event")
		})
	})

	Describe("Cross-Service Wiring", func() {

		It("IT-BRIDGE-008: KA and DS tools work in same MCP session with correct routing", func() {
			var kaHit atomic.Bool
			var dsHit atomic.Bool
			mockDS := &ds.MockClient{
				ListWorkflowsFn: func(_ context.Context, _ ds.ListWorkflowsOpts) ([]ds.Workflow, error) {
					dsHit.Store(true)
					return []ds.Workflow{{ID: "wf-1", Name: "restart"}}, nil
				},
				GetRemediationHistoryFn: func(_ context.Context, _ ds.HistoryOpts) ([]ds.HistoricalRemediation, error) {
					return nil, nil
				},
				GetEffectivenessFn: func(_ context.Context, _ ds.EffectivenessOpts) (*ds.EffectivenessReport, error) {
					return &ds.EffectivenessReport{}, nil
				},
				GetAuditTrailFn: func(_ context.Context, _ ds.AuditTrailOpts) ([]ds.AuditEvent, error) {
					return nil, nil
				},
			}

			setupStackWithKAHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				kaHit.Store(true)
				if r.Method == http.MethodPost && r.URL.Path == "/api/v1/incident/analyze" {
					w.WriteHeader(http.StatusAccepted)
					_ = json.NewEncoder(w).Encode(map[string]string{"session_id": "cross-sess"})
					return
				}
				if strings.HasSuffix(r.URL.Path, "/stream") {
					w.Header().Set("Content-Type", "text/event-stream")
					w.WriteHeader(http.StatusOK)
					_, _ = fmt.Fprintf(w, "event: complete\ndata: {\"type\":\"complete\",\"summary\":\"done\"}\n\n")
					return
				}
				w.WriteHeader(http.StatusNotFound)
			}), mockDS)

			// Call KA tool
			status1, _ := mcpCallTool(h, sessionID, "kubernaut_investigate", map[string]any{
				"namespace": "default", "name": "test", "kind": "Pod",
			}, testUser)
			Expect(status1).To(Equal(http.StatusOK))

			// Call DS tool in same session
			status2, _ := mcpCallTool(h, sessionID, "kubernaut_list_workflows", map[string]any{}, testUser)
			Expect(status2).To(Equal(http.StatusOK))

			Expect(kaHit.Load()).To(BeTrue(), "KA httptest should have been hit")
			Expect(dsHit.Load()).To(BeTrue(), "DS mock should have been called")
		})

		It("IT-BRIDGE-009: audit events emitted for both KA and DS tool calls", func() {
			mockDS := newFakeDSClient()

			setupStackWithKAHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost && r.URL.Path == "/api/v1/incident/analyze" {
					w.WriteHeader(http.StatusAccepted)
					_ = json.NewEncoder(w).Encode(map[string]string{"session_id": "audit-sess"})
					return
				}
				if strings.HasSuffix(r.URL.Path, "/stream") {
					w.Header().Set("Content-Type", "text/event-stream")
					w.WriteHeader(http.StatusOK)
					_, _ = fmt.Fprintf(w, "event: complete\ndata: {\"type\":\"complete\",\"summary\":\"done\"}\n\n")
					return
				}
				w.WriteHeader(http.StatusNotFound)
			}), mockDS)

			auditor.Reset()

			mcpCallTool(h, sessionID, "kubernaut_investigate", map[string]any{
				"namespace": "ns", "name": "pod", "kind": "Pod",
			}, testUser)
			mcpCallTool(h, sessionID, "kubernaut_list_workflows", map[string]any{}, testUser)

			events := auditor.Events()
			Expect(len(events)).To(BeNumerically(">=", 2), "should have audit events for both KA and DS tool calls")
		})
	})

	Describe("KA Failure Modes Through Bridge", func() {

		It("IT-BRIDGE-010: KA returning 500 produces tool error (not bridge crash)", func() {
			setupStackWithKAHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			}), newFakeDSClient())

			status, body := mcpCallTool(h, sessionID, "kubernaut_investigate", map[string]any{
				"namespace": "ns", "name": "pod", "kind": "Pod",
			}, testUser)

			Expect(status).To(Equal(http.StatusOK))
			text := extractTextContent(body)
			Expect(text).To(ContainSubstring("error"))
		})

		It("IT-BRIDGE-011: KA connection refused produces user-friendly error", func() {
			// Create a server and immediately close to get a "connection refused" port
			closedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			closedURL := closedServer.URL
			closedServer.Close()

			kaClient := ka.NewClient(ka.Config{
				BaseURL:            closedURL,
				Timeout:            500 * time.Millisecond,
				CBFailureThreshold: 10,
				RetryMax:           0,
			})

			auditor = &fakeAuditor{}
			testUser = &auth.UserIdentity{Username: "sre@kubernaut.ai", Groups: []string{"sre"}}
			fakeK8s := newFakeDynamicClient()

			cfg := handler.MCPConfig{
				ServerName:    "af-it",
				ServerVersion: "0.0.1-test",
				Enabled:       true,
				Bridge: &handler.MCPBridgeConfig{
					K8sClient: fakeK8s,
					KAClient:   kaClient,
					KAMCPClient: &ka.MockMCPClient{
						SelectWorkflowFn: func(_ context.Context, _ ka.SelectWorkflowArgs) (*ka.SelectWorkflowResult, error) {
							return nil, ka.ErrMCPUnavailable
						},
					},
					DSClient:           newFakeDSClient(),
					Authorizer:         &mapAuthorizer{roles: map[string][]string{"sre": {"*"}}},
					Logger:             logr.Discard(),
					Auditor:            auditor,
					Metrics:            newBridgeMetrics(),
					ToolTimeout:        5 * time.Second,
					MaxConcurrentTools: 10,
				},
			}

			var err error
			h, err = handler.NewMCPHandler(cfg)
			Expect(err).NotTo(HaveOccurred())
			sessionID = mcpInitialize(h, testUser)

			status, body := mcpCallTool(h, sessionID, "kubernaut_investigate", map[string]any{
				"namespace": "ns", "name": "pod", "kind": "Pod",
			}, testUser)

			Expect(status).To(Equal(http.StatusOK))
			text := extractTextContent(body)
			Expect(text).To(ContainSubstring("unavailable"))
		})

		It("IT-BRIDGE-012: nil DSClient returns clear error for DS tools", func() {
			setupStackWithKAHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}), nil)

			status, body := mcpCallTool(h, sessionID, "kubernaut_list_workflows", map[string]any{}, testUser)

			Expect(status).To(Equal(http.StatusOK))
			text := extractTextContent(body)
			Expect(text).NotTo(BeEmpty())
		})
	})

	Describe("Interactive Tool Wiring (G1)", func() {

		var invokedAction string

		setupInteractiveStack := func() {
			invokedAction = ""
			mockMCP := &ka.MockMCPClient{
				SelectWorkflowFn: func(_ context.Context, _ ka.SelectWorkflowArgs) (*ka.SelectWorkflowResult, error) {
					return &ka.SelectWorkflowResult{Status: "selected"}, nil
				},
				InvokeActionFn: func(_ context.Context, args ka.InvokeActionArgs) (*ka.InvokeActionResult, error) {
					invokedAction = args.Action
					return &ka.InvokeActionResult{SessionID: "interactive-sess", Status: "ok"}, nil
				},
			}
			kaStreamHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/stream") {
					w.Header().Set("Content-Type", "text/event-stream")
					w.WriteHeader(http.StatusOK)
					_, _ = fmt.Fprintf(w, "event: complete\ndata: {\"type\":\"complete\",\"summary\":\"done\"}\n\n")
					return
				}
				w.WriteHeader(http.StatusNotFound)
			})
			setupStackWithKAHandler(kaStreamHandler, newFakeDSClient())
			kaServer.Close()
			kaServer = httptest.NewServer(kaStreamHandler)

			kaClient := ka.NewClient(ka.Config{
				BaseURL:            kaServer.URL,
				Timeout:            5 * time.Second,
				CBFailureThreshold: 5,
				CBMaxRequests:      3,
				CBInterval:         10 * time.Second,
				CBTimeout:          100 * time.Millisecond,
				RetryMax:           1,
				RetryInitBackoff:   1 * time.Millisecond,
				RetryMaxBackoff:    5 * time.Millisecond,
				RetryableStatuses:  []int{503},
			})

			auditor = &fakeAuditor{}
			testUser = &auth.UserIdentity{Username: "sre@kubernaut.ai", Groups: []string{"sre"}}

			cfg := handler.MCPConfig{
				ServerName:    "af-it",
				ServerVersion: "0.0.1-test",
				Enabled:       true,
				Bridge: &handler.MCPBridgeConfig{
					K8sClient:          newFakeDynamicClient(),
					KAClient:           kaClient,
					KAMCPClient:        mockMCP,
					DSClient:           newFakeDSClient(),
					Authorizer:         &mapAuthorizer{roles: map[string][]string{"sre": {"*"}}},
					Logger:             logr.Discard(),
					Auditor:            auditor,
					Metrics:            newBridgeMetrics(),
					ToolTimeout:        5 * time.Second,
					MaxConcurrentTools: 10,
				},
			}

			var err error
			h, err = handler.NewMCPHandler(cfg)
			Expect(err).NotTo(HaveOccurred())
			sessionID = mcpInitialize(h, testUser)
		}

		It("IT-AF-1234-W01: kubernaut_takeover dispatches to KA MCP via InvokeAction", func() {
			setupInteractiveStack()

			status, body := mcpCallTool(h, sessionID, "kubernaut_takeover", map[string]any{
				"rr_id": "production/api-gw",
			}, testUser)

			Expect(status).To(Equal(http.StatusOK))
			text := extractTextContent(body)
			Expect(text).To(ContainSubstring("ok"))
			Expect(invokedAction).To(Equal("takeover"))
		})

		It("IT-AF-1293-BRIDGE-001: kubernaut_takeover propagates IS init failure to caller (FedRAMP AU-12)", func() {
			invokedAction = ""
			mockMCP := &ka.MockMCPClient{
				InvokeActionFn: func(_ context.Context, args ka.InvokeActionArgs) (*ka.InvokeActionResult, error) {
					invokedAction = args.Action
					return &ka.InvokeActionResult{SessionID: "sess-fail", Status: "ok"}, nil
				},
			}
			kaStreamHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			})
			setupStackWithKAHandler(kaStreamHandler, newFakeDSClient())
			kaServer.Close()
			kaServer = httptest.NewServer(kaStreamHandler)

			kaClient := ka.NewClient(ka.Config{
				BaseURL: kaServer.URL, Timeout: 5 * time.Second,
				CBFailureThreshold: 5, CBMaxRequests: 3, CBInterval: 10 * time.Second,
				CBTimeout: 100 * time.Millisecond, RetryMax: 1,
				RetryInitBackoff: 1 * time.Millisecond, RetryMaxBackoff: 5 * time.Millisecond,
				RetryableStatuses: []int{503},
			})

			auditor = &fakeAuditor{}
			testUser = &auth.UserIdentity{Username: "sre@kubernaut.ai", Groups: []string{"sre"}}

			failingInitializer := &failingSessionInitializer{err: fmt.Errorf("simulated K8s API error")}

			cfg := handler.MCPConfig{
				ServerName:    "af-it",
				ServerVersion: "0.0.1-test",
				Enabled:       true,
				Bridge: &handler.MCPBridgeConfig{
					K8sClient:          newFakeDynamicClient(),
					KAClient:           kaClient,
					KAMCPClient:        mockMCP,
					DSClient:           newFakeDSClient(),
					Authorizer:         &mapAuthorizer{roles: map[string][]string{"sre": {"*"}}},
					Logger:             logr.Discard(),
					Auditor:            auditor,
					Metrics:            newBridgeMetrics(),
					ToolTimeout:        5 * time.Second,
					MaxConcurrentTools: 10,
					SessionInitializer: failingInitializer,
				},
			}

			var err error
			h, err = handler.NewMCPHandler(cfg)
			Expect(err).NotTo(HaveOccurred())
			sessionID = mcpInitialize(h, testUser)

			status, body := mcpCallTool(h, sessionID, "kubernaut_takeover", map[string]any{
				"rr_id": "production/api-gw",
			}, testUser)

			Expect(status).To(Equal(http.StatusOK))
			Expect(invokedAction).To(Equal("takeover"))
			text := extractTextContent(body)
			Expect(text).To(ContainSubstring("IS CRD creation failed"),
				"error must propagate to caller so the MCP client can retry or alert the user")
		})

		It("IT-AF-1234-W02: kubernaut_message dispatches with message payload", func() {
			setupInteractiveStack()

			status, body := mcpCallTool(h, sessionID, "kubernaut_message", map[string]any{
				"rr_id":   "production/api-gw",
				"message": "increase memory to 512Mi",
			}, testUser)

			Expect(status).To(Equal(http.StatusOK))
			text := extractTextContent(body)
			Expect(text).To(ContainSubstring("ok"))
			Expect(invokedAction).To(Equal("message"))
		})

		It("IT-AF-1234-W03: kubernaut_complete dispatches complete action", func() {
			setupInteractiveStack()

			status, body := mcpCallTool(h, sessionID, "kubernaut_complete", map[string]any{
				"rr_id": "production/api-gw",
			}, testUser)

			Expect(status).To(Equal(http.StatusOK))
			text := extractTextContent(body)
			Expect(text).To(ContainSubstring("ok"))
			Expect(invokedAction).To(Equal("complete"))
		})

		It("IT-AF-1234-W04: kubernaut_cancel dispatches cancel action", func() {
			setupInteractiveStack()

			status, body := mcpCallTool(h, sessionID, "kubernaut_cancel", map[string]any{
				"rr_id": "production/api-gw",
			}, testUser)

			Expect(status).To(Equal(http.StatusOK))
			text := extractTextContent(body)
			Expect(text).To(ContainSubstring("ok"))
			Expect(invokedAction).To(Equal("cancel"))
		})

		It("IT-AF-1234-W05: kubernaut_status dispatches status action", func() {
			setupInteractiveStack()

			status, body := mcpCallTool(h, sessionID, "kubernaut_status", map[string]any{
				"rr_id": "production/api-gw",
			}, testUser)

			Expect(status).To(Equal(http.StatusOK))
			text := extractTextContent(body)
			Expect(text).To(ContainSubstring("ok"))
			Expect(invokedAction).To(Equal("status"))
		})

		It("IT-AF-1234-W06: kubernaut_reconnect dispatches reconnect action", func() {
			setupInteractiveStack()

			status, body := mcpCallTool(h, sessionID, "kubernaut_reconnect", map[string]any{
				"rr_id": "production/api-gw",
			}, testUser)

			Expect(status).To(Equal(http.StatusOK))
			text := extractTextContent(body)
			Expect(text).To(ContainSubstring("ok"))
			Expect(invokedAction).To(Equal("reconnect"))
		})

		It("IT-AF-1234-W07: kubernaut_investigate dispatches to KA SSE stream (namespace/name)", func() {
			kaStreamHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost && r.URL.Path == "/api/v1/incident/analyze" {
					w.WriteHeader(http.StatusAccepted)
					_ = json.NewEncoder(w).Encode(map[string]string{"session_id": "sess-stream-001"})
					return
				}
				if strings.HasSuffix(r.URL.Path, "/stream") {
					w.Header().Set("Content-Type", "text/event-stream")
					w.WriteHeader(http.StatusOK)
					_, _ = fmt.Fprintf(w, "event: complete\ndata: {\"type\":\"complete\",\"summary\":\"done\"}\n\n")
					return
				}
				w.WriteHeader(http.StatusNotFound)
			})
			setupStackWithKAHandler(kaStreamHandler, newFakeDSClient())

			status, body := mcpCallTool(h, sessionID, "kubernaut_investigate", map[string]any{
				"namespace": "production",
				"name":      "api-gw",
				"kind":      "Deployment",
			}, testUser)

			Expect(status).To(Equal(http.StatusOK))
			text := extractTextContent(body)
			Expect(text).To(SatisfyAny(
				ContainSubstring("completed"),
				ContainSubstring("done"),
				ContainSubstring("sess-stream-001"),
			))
		})
	})

	Describe("KA Circuit Breaker Through Bridge", func() {

		It("IT-BRIDGE-013: CB trips after N failures, subsequent tool calls fail fast with friendly error", func() {
			var callCount atomic.Int32
			setupStackWithKAHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				callCount.Add(1)
				w.WriteHeader(http.StatusBadGateway)
			}), newFakeDSClient())

			// Fire enough requests to trip the CB (threshold=5)
			for i := 0; i < 6; i++ {
				mcpCallTool(h, sessionID, "kubernaut_investigate", map[string]any{
					"namespace": "ns", "name": "pod", "kind": "Pod",
				}, testUser)
			}

			// Next call should fail fast (CB open, no HTTP call to server)
			beforeCount := callCount.Load()
			_, body := mcpCallTool(h, sessionID, "kubernaut_investigate", map[string]any{
				"namespace": "ns", "name": "pod", "kind": "Pod",
			}, testUser)

			text := extractTextContent(body)
			Expect(text).To(ContainSubstring("unavailable"))
			Expect(callCount.Load()).To(Equal(beforeCount),
				"should not have made additional HTTP call when CB is open")
		})
	})

	Describe("Observability Enrichment (G11/G12/G18)", func() {

		It("IT-AF-1234-W30: per-tool timeout overrides global for investigate tool", func() {
			setupStackWithKAHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			}), newFakeDSClient())

			cfg := handler.MCPBridgeConfig{
				ToolTimeout: 5 * time.Second,
				ToolTimeouts: map[string]time.Duration{
					"kubernaut_investigate": 120 * time.Second,
				},
			}

			streamTimeout := cfg.GetToolTimeoutFor("kubernaut_investigate")
			defaultTimeout := cfg.GetToolTimeoutFor("kubernaut_list_remediations")

			Expect(streamTimeout).To(Equal(120 * time.Second))
			Expect(defaultTimeout).To(Equal(5 * time.Second))
		})

		It("IT-AF-1234-W31: audit emits session_id and rr_id from interactive tool args", func() {
			setupStackWithKAHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			}), newFakeDSClient())
			auditor.Reset()

			mcpCallTool(h, sessionID, "kubernaut_takeover", map[string]any{
				"rr_id": "production/api-gw",
			}, testUser)

			events := auditor.Events()
			var hasRRID bool
			for _, e := range events {
				if e.Detail["rr_id"] == "production/api-gw" {
					hasRRID = true
				}
			}
			Expect(hasRRID).To(BeTrue(), "audit event should include rr_id from tool args")
		})

		It("IT-AF-1234-W32: audit emits execution_duration_ms on success", func() {
			setupStackWithKAHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			}), newFakeDSClient())
			auditor.Reset()

			mcpCallTool(h, sessionID, "kubernaut_present_decision", map[string]any{
				"session_id": "sess-obs-001",
				"summary":    "test summary",
				"options":    []any{},
			}, testUser)

			events := auditor.Events()
			var hasDuration bool
			for _, e := range events {
				if e.Detail["execution_duration_ms"] != "" {
					hasDuration = true
				}
			}
			Expect(hasDuration).To(BeTrue(), "audit event should include execution_duration_ms")
		})
	})
})

type failingSessionInitializer struct {
	err error
}

func (f *failingSessionInitializer) InitializeSessionByRR(_ context.Context, _, _, _, _ string, _ []string) error {
	return f.err
}
