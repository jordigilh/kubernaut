package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut-apifrontend/internal/auth"
	"github.com/jordigilh/kubernaut-apifrontend/internal/ds"
	"github.com/jordigilh/kubernaut-apifrontend/internal/handler"
	"github.com/jordigilh/kubernaut-apifrontend/internal/ka"
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
				DynFactory: auth.StaticDynamicFactory(fakeK8s),
				KAClient:   kaClient,
				KAMCPClient: &ka.MockMCPClient{
					SelectWorkflowFn: func(_ context.Context, _ ka.SelectWorkflowArgs) (*ka.SelectWorkflowResult, error) {
						return &ka.SelectWorkflowResult{Status: "selected", Message: "workflow selected"}, nil
					},
				},
				DSClient:           dsClient,
				RBACRoles:          map[string][]string{"sre": {"*"}},
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

		It("IT-BRIDGE-001: kubernaut_start_investigation dispatches POST to KA /analyze", func() {
			var capturedPath, capturedMethod string
			setupStackWithKAHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedPath = r.URL.Path
				capturedMethod = r.Method
				w.WriteHeader(http.StatusAccepted)
				_ = json.NewEncoder(w).Encode(map[string]string{"session_id": "it-sess-001"})
			}), newFakeDSClient())

			status, body := mcpCallTool(h, sessionID, "kubernaut_start_investigation", map[string]any{
				"namespace": "production",
				"name":      "api-gw",
				"kind":      "Deployment",
			}, testUser)

			Expect(status).To(Equal(http.StatusOK))
			Expect(capturedPath).To(Equal("/api/v1/incident/analyze"))
			Expect(capturedMethod).To(Equal(http.MethodPost))
			text := extractTextContent(body)
			Expect(text).To(ContainSubstring("it-sess-001"))
		})

		It("IT-BRIDGE-002: kubernaut_poll_investigation with completed status returns summary", func() {
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

			status, body := mcpCallTool(h, sessionID, "kubernaut_poll_investigation", map[string]any{
				"session_id": "sess-002",
			}, testUser)

			Expect(status).To(Equal(http.StatusOK))
			text := extractTextContent(body)
			Expect(text).To(ContainSubstring("completed"))
		})

		It("IT-BRIDGE-003: kubernaut_poll_investigation with in_progress is interrupted by tool timeout", func() {
			setupStackWithKAHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(ka.SessionStatus{SessionID: "sess-003", Status: "investigating"})
			}), newFakeDSClient())

			status, body := mcpCallTool(h, sessionID, "kubernaut_poll_investigation", map[string]any{
				"session_id": "sess-003",
			}, testUser)

			Expect(status).To(Equal(http.StatusOK))
			text := extractTextContent(body)
			// Tool timeout (5s) fires before 5x3s polling completes,
			// so we expect either a timeout error or in_progress
			Expect(text).To(SatisfyAny(
				ContainSubstring("in_progress"),
				ContainSubstring("deadline"),
				ContainSubstring("timeout"),
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

			setupStackWithKAHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				kaHit.Store(true)
				w.WriteHeader(http.StatusAccepted)
				_ = json.NewEncoder(w).Encode(map[string]string{"session_id": "cross-sess"})
			}), mockDS)

			// Call KA tool
			status1, _ := mcpCallTool(h, sessionID, "kubernaut_start_investigation", map[string]any{
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

			setupStackWithKAHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusAccepted)
				_ = json.NewEncoder(w).Encode(map[string]string{"session_id": "audit-sess"})
			}), mockDS)

			auditor.Reset()

			mcpCallTool(h, sessionID, "kubernaut_start_investigation", map[string]any{
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

			status, body := mcpCallTool(h, sessionID, "kubernaut_start_investigation", map[string]any{
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
					DynFactory: auth.StaticDynamicFactory(fakeK8s),
					KAClient:   kaClient,
					KAMCPClient: &ka.MockMCPClient{
						SelectWorkflowFn: func(_ context.Context, _ ka.SelectWorkflowArgs) (*ka.SelectWorkflowResult, error) {
							return nil, ka.ErrMCPUnavailable
						},
					},
					DSClient:           newFakeDSClient(),
					RBACRoles:          map[string][]string{"sre": {"*"}},
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

			status, body := mcpCallTool(h, sessionID, "kubernaut_start_investigation", map[string]any{
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

	Describe("KA Circuit Breaker Through Bridge", func() {

		It("IT-BRIDGE-013: CB trips after N failures, subsequent tool calls fail fast with friendly error", func() {
			var callCount atomic.Int32
			setupStackWithKAHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				callCount.Add(1)
				w.WriteHeader(http.StatusBadGateway)
			}), newFakeDSClient())

			// Fire enough requests to trip the CB (threshold=5)
			for i := 0; i < 6; i++ {
				mcpCallTool(h, sessionID, "kubernaut_start_investigation", map[string]any{
					"namespace": "ns", "name": "pod", "kind": "Pod",
				}, testUser)
			}

			// Next call should fail fast (CB open, no HTTP call to server)
			beforeCount := callCount.Load()
			_, body := mcpCallTool(h, sessionID, "kubernaut_start_investigation", map[string]any{
				"namespace": "ns", "name": "pod", "kind": "Pod",
			}, testUser)

			text := extractTextContent(body)
			Expect(text).To(ContainSubstring("unavailable"))
			Expect(callCount.Load()).To(Equal(beforeCount),
				"should not have made additional HTTP call when CB is open")
		})
	})
})
