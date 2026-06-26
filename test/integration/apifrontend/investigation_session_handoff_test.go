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

package apifrontend_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"

	"github.com/go-logr/logr"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	aiav1alpha1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

// =============================================================================
// IT-AF-1332: Investigation Session Handoff (BR-INTERACTIVE-010, #1332)
//
// Wiring Manifest rows covered:
//   - KASessionPool.Inject          (IT-AF-1332-010)
//   - Session on StartInvestigationResult (IT-AF-1332-010)
//   - Post-investigate session reuse (IT-AF-1332-020)
//   - Fallback to cleanup           (IT-AF-1332-030)
//
// Pyramid Invariant:
//   UT proves logic (session_pool_test.go, ka_investigate_mcp_test.go).
//   IT (this file) proves wiring through production dispatch paths.
// =============================================================================

// handoffSession is a trackable PoolSession used by IT-AF-1332 and IT-AF-1387 tests.
type handoffSession struct {
	id       string
	callFn   func(ctx context.Context, params *mcp.CallToolParams) (*mcp.CallToolResult, error)
	pingFn   func(ctx context.Context, params *mcp.PingParams) error
	closed   int32
}

func (s *handoffSession) CallTool(ctx context.Context, params *mcp.CallToolParams) (*mcp.CallToolResult, error) {
	if s.callFn != nil {
		return s.callFn(ctx, params)
	}
	return &mcp.CallToolResult{}, nil
}

func (s *handoffSession) Ping(ctx context.Context, params *mcp.PingParams) error {
	if s.pingFn != nil {
		return s.pingFn(ctx, params)
	}
	return nil
}

func (s *handoffSession) Close() error {
	atomic.AddInt32(&s.closed, 1)
	return nil
}

func (s *handoffSession) isClosed() bool {
	return atomic.LoadInt32(&s.closed) > 0
}

var _ = Describe("Investigation Session Handoff (#1332)", Label("integration", "session-handoff"), func() {

	Describe("IT-AF-1332-010: blocking investigate hands off session to pool", func() {
		It("should inject session into pool and deregister from MonitorRegistry", func() {
			injectedSession := &handoffSession{id: "injected-010"}
			eventCh := make(chan ka.InvestigationEvent, 10)

			mockMCP := &ka.MockMCPClient{
				StartInvestigationFn: func(_ context.Context, _ ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
					go func() {
						eventCh <- ka.InvestigationEvent{
							Type: ka.EventTypeReasoningDelta,
							Data: json.RawMessage(`{"text":"RCA: OOM detected."}`),
						}
						eventCh <- ka.InvestigationEvent{
							Type: ka.EventTypeComplete,
							Data: json.RawMessage(`{}`),
						}
						close(eventCh)
					}()
					return &ka.StartInvestigationResult{
						SessionID: "sess-it-1332-010",
						Status:    "autonomous_started",
						Events:    eventCh,
						Closer:    func() {},
						Session:   injectedSession,
					}, nil
				},
			}

			var factoryCalled int32
			pool := ka.NewKASessionPool(ka.PoolConfig{
				Factory: func(_ context.Context) (ka.PoolSession, error) {
					atomic.AddInt32(&factoryCalled, 1)
					return &handoffSession{id: "factory-created"}, nil
				},
				MaxEntries: 10,
				Logger:     logr.Discard(),
			})
			registry := tools.NewMonitorRegistry()

			result, err := tools.HandleInvestigationMCPWithRegistry(
				context.Background(), &tools.InvestigateConfig{
					MCPClient: mockMCP,
					Registry:  registry,
					Pool:      pool,
				}, tools.InvestigateMCPArgs{RRID: "rr-it-1332-010"}, true, "alice",
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Status).To(Equal("completed"))
			Expect(result.Summary).To(Equal("RCA: OOM detected."))

			By("verifying session was injected into pool")
			acquired, err := pool.Acquire(context.Background(), "rr-it-1332-010", "alice")
			Expect(err).NotTo(HaveOccurred())
			Expect(acquired).To(BeIdenticalTo(injectedSession),
				"pool.Acquire must return the injected investigation session, not a factory-created one")

			By("verifying factory was never called")
			Expect(atomic.LoadInt32(&factoryCalled)).To(Equal(int32(0)),
				"factory must not be called when Inject placed a session for this key")

			By("verifying session deregistered from MonitorRegistry")
			Expect(registry.Active("sess-it-1332-010")).To(BeFalse(),
				"session must be deregistered from MonitorRegistry after handoff")

			By("verifying session was NOT closed")
			Expect(injectedSession.isClosed()).To(BeFalse(),
				"injected session must remain open for reuse by discover_workflows")
		})
	})

	Describe("IT-AF-1332-020: discover_workflows reuses injected session from pool", func() {
		It("should acquire injected session and call tool via the same connection", func() {
			var callToolInvoked int32
			injectedSession := &handoffSession{
				id: "injected-020",
				callFn: func(_ context.Context, params *mcp.CallToolParams) (*mcp.CallToolResult, error) {
					atomic.AddInt32(&callToolInvoked, 1)
					resp := map[string]any{
						"workflows": []map[string]any{
							{
								"workflow_id": "wf-oomkill-v1",
								"name":        "OOMKill Recovery",
								"description": "Increase memory limit",
								"kind":        "remediation",
								"confidence":  0.95,
							},
						},
					}
					raw, _ := json.Marshal(resp)
					return &mcp.CallToolResult{
						Content: []mcp.Content{&mcp.TextContent{Text: string(raw)}},
					}, nil
				},
			}

			eventCh := make(chan ka.InvestigationEvent, 10)
			mockMCP := &ka.MockMCPClient{
				StartInvestigationFn: func(_ context.Context, _ ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
					go func() {
						eventCh <- ka.InvestigationEvent{
							Type: ka.EventTypeComplete,
							Data: json.RawMessage(`{}`),
						}
						close(eventCh)
					}()
					return &ka.StartInvestigationResult{
						SessionID: "sess-it-1332-020",
						Status:    "autonomous_started",
						Events:    eventCh,
						Closer:    func() {},
						Session:   injectedSession,
					}, nil
				},
			}

			var factoryCalled int32
			pool := ka.NewKASessionPool(ka.PoolConfig{
				Factory: func(_ context.Context) (ka.PoolSession, error) {
					atomic.AddInt32(&factoryCalled, 1)
					return &handoffSession{id: "factory-sentinel"}, nil
				},
				MaxEntries: 10,
				Logger:     logr.Discard(),
			})
			registry := tools.NewMonitorRegistry()

			By("completing the investigation (blocking path with handoff)")
			_, err := tools.HandleInvestigationMCPWithRegistry(
				context.Background(), &tools.InvestigateConfig{
					MCPClient: mockMCP,
					Registry:  registry,
					Pool:      pool,
				}, tools.InvestigateMCPArgs{RRID: "rr-it-1332-020"}, true, "bob",
			)
			Expect(err).NotTo(HaveOccurred())

			By("calling DiscoverWorkflows via PooledMCPClient backed by the same pool")
			pooledClient := ka.NewPooledMCPClient(pool, logr.Discard())
			ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
				Username: "bob",
				Groups:   []string{"system:authenticated"},
			})
			result, err := pooledClient.DiscoverWorkflows(ctx, ka.DiscoverWorkflowsArgs{
				RRID: "rr-it-1332-020",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Workflows).To(HaveLen(1))
			Expect(result.Workflows[0].WorkflowID).To(Equal("wf-oomkill-v1"))

			By("verifying the injected session's CallTool was invoked (not factory)")
			Expect(atomic.LoadInt32(&callToolInvoked)).To(Equal(int32(1)),
				"PooledMCPClient must call the injected session, proving session reuse")
			Expect(atomic.LoadInt32(&factoryCalled)).To(Equal(int32(0)),
				"factory must not be called when injected session exists for (rr_id, username)")
		})
	})

	Describe("IT-AF-1332-030: blocking mode falls back to cleanup when pool is nil", func() {
		It("should call cleanup (closer) instead of injecting when pool is nil", func() {
			var closerCalled int32
			eventCh := make(chan ka.InvestigationEvent, 10)

			mockMCP := &ka.MockMCPClient{
				StartInvestigationFn: func(_ context.Context, _ ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
					go func() {
						eventCh <- ka.InvestigationEvent{
							Type: ka.EventTypeComplete,
							Data: json.RawMessage(`{}`),
						}
						close(eventCh)
					}()
					return &ka.StartInvestigationResult{
						SessionID: "sess-it-1332-030",
						Status:    "autonomous_started",
						Events:    eventCh,
						Closer:    func() { atomic.AddInt32(&closerCalled, 1) },
					}, nil
				},
			}

			result, err := tools.HandleInvestigationMCPWithRegistry(
				context.Background(), &tools.InvestigateConfig{
					MCPClient: mockMCP,
				}, tools.InvestigateMCPArgs{RRID: "rr-it-1332-030"}, true, "",
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Status).To(Equal("completed"))

			By("verifying cleanup was called exactly once")
			Expect(atomic.LoadInt32(&closerCalled)).To(Equal(int32(1)),
				"cleanup (closer) must be called when pool is nil — MCP bridge path behavior")
		})
	})
})

// =============================================================================
// IT-AF-1387: MCP Session Resilience — Ping-on-Acquire Wiring (#1387)
//
// Wiring Manifest rows covered:
//   - Ping-on-Acquire via PooledMCPClient.DiscoverWorkflows (IT-AF-1387-W01)
//   - Transparent reconnection via PooledMCPClient dispatch   (IT-AF-1387-W02)
//   - Healthy session reuse via PooledMCPClient dispatch      (IT-AF-1387-W03)
//
// Pyramid Invariant:
//   UT (session_pool_test.go) proves Ping-on-Acquire logic.
//   IT (this block) proves wiring through PooledMCPClient production dispatch.
// =============================================================================

var _ = Describe("MCP Session Resilience — Ping-on-Acquire Wiring (#1387)", Label("integration", "mcp-resilience"), func() {

	It("IT-AF-1387-W01 [SI-4, SC-24]: dead session evicted transparently when DiscoverWorkflows dispatches through pool", func() {
		var factoryCalled int32
		pool := ka.NewKASessionPool(ka.PoolConfig{
			Factory: func(_ context.Context) (ka.PoolSession, error) {
				atomic.AddInt32(&factoryCalled, 1)
				return &handoffSession{
					id: "factory-replacement",
					callFn: func(_ context.Context, _ *mcp.CallToolParams) (*mcp.CallToolResult, error) {
						resp := map[string]any{
							"workflows": []map[string]any{
								{"workflow_id": "wf-recovery-v1", "name": "Recovery", "description": "Auto recovery", "kind": "remediation", "confidence": 0.9},
							},
						}
						raw, _ := json.Marshal(resp)
						return &mcp.CallToolResult{
							Content: []mcp.Content{&mcp.TextContent{Text: string(raw)}},
						}, nil
					},
				}, nil
			},
			MaxEntries: 10,
			Logger:     logr.Discard(),
		})

		deadSession := &handoffSession{
			id: "dead-session",
			pingFn: func(_ context.Context, _ *mcp.PingParams) error {
				return fmt.Errorf("transport closed")
			},
		}
		pool.Inject("rr-it-1387-w01", "alice", deadSession)

		pooledClient := ka.NewPooledMCPClient(pool, logr.Discard())
		ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
			Username: "alice",
			Groups:   []string{"system:authenticated"},
		})

		result, err := pooledClient.DiscoverWorkflows(ctx, ka.DiscoverWorkflowsArgs{
			RRID: "rr-it-1387-w01",
		})
		Expect(err).NotTo(HaveOccurred(),
			"SI-4: PooledMCPClient.DiscoverWorkflows must succeed despite dead cached session")
		Expect(result).NotTo(BeNil())
		Expect(result.Workflows).To(HaveLen(1))
		Expect(result.Workflows[0].WorkflowID).To(Equal("wf-recovery-v1"))

		By("verifying dead session was evicted and closed")
		Expect(deadSession.isClosed()).To(BeTrue(),
			"SC-24: evicted session must be closed — fail in known state")

		By("verifying factory created replacement")
		Expect(atomic.LoadInt32(&factoryCalled)).To(Equal(int32(1)),
			"CP-10: factory must create exactly one replacement session")
	})

	It("IT-AF-1387-W02 [CP-10]: transparent reconnection via InvokeAction dispatch path", func() {
		var factoryCalled int32
		pool := ka.NewKASessionPool(ka.PoolConfig{
			Factory: func(_ context.Context) (ka.PoolSession, error) {
				atomic.AddInt32(&factoryCalled, 1)
				return &handoffSession{
					id: "factory-replacement-w02",
					callFn: func(_ context.Context, _ *mcp.CallToolParams) (*mcp.CallToolResult, error) {
						resp := map[string]any{"status": "active", "session_id": "sess-w02"}
						raw, _ := json.Marshal(resp)
						return &mcp.CallToolResult{
							Content: []mcp.Content{&mcp.TextContent{Text: string(raw)}},
						}, nil
					},
				}, nil
			},
			MaxEntries: 10,
			Logger:     logr.Discard(),
		})

		deadSession := &handoffSession{
			id: "dead-session-w02",
			pingFn: func(_ context.Context, _ *mcp.PingParams) error {
				return fmt.Errorf("connection reset by peer")
			},
		}
		pool.Inject("rr-it-1387-w02", "bob", deadSession)

		pooledClient := ka.NewPooledMCPClient(pool, logr.Discard())
		ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
			Username: "bob",
			Groups:   []string{"system:authenticated"},
		})

		result, err := pooledClient.InvokeAction(ctx, ka.InvokeActionArgs{
			RRID:   "rr-it-1387-w02",
			Action: "status",
		})
		Expect(err).NotTo(HaveOccurred(),
			"CP-10: InvokeAction must auto-reconstitute after dead session eviction")
		Expect(result).NotTo(BeNil())
		Expect(result.Status).To(Equal("active"))
	})

	It("IT-AF-1387-W03 [SI-4]: healthy session reused via PooledMCPClient without false-positive eviction", func() {
		var factoryCalled int32
		var callToolInvoked int32
		healthySession := &handoffSession{
			id: "healthy-session",
			callFn: func(_ context.Context, _ *mcp.CallToolParams) (*mcp.CallToolResult, error) {
				atomic.AddInt32(&callToolInvoked, 1)
				resp := map[string]any{
					"workflows": []map[string]any{
						{"workflow_id": "wf-healthy-v1", "name": "Healthy Path", "description": "test", "kind": "remediation", "confidence": 0.99},
					},
				}
				raw, _ := json.Marshal(resp)
				return &mcp.CallToolResult{
					Content: []mcp.Content{&mcp.TextContent{Text: string(raw)}},
				}, nil
			},
		}

		pool := ka.NewKASessionPool(ka.PoolConfig{
			Factory: func(_ context.Context) (ka.PoolSession, error) {
				atomic.AddInt32(&factoryCalled, 1)
				return &handoffSession{id: "factory-sentinel"}, nil
			},
			MaxEntries: 10,
			Logger:     logr.Discard(),
		})
		pool.Inject("rr-it-1387-w03", "carol", healthySession)

		pooledClient := ka.NewPooledMCPClient(pool, logr.Discard())
		ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
			Username: "carol",
			Groups:   []string{"system:authenticated"},
		})

		result, err := pooledClient.DiscoverWorkflows(ctx, ka.DiscoverWorkflowsArgs{
			RRID: "rr-it-1387-w03",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Workflows).To(HaveLen(1))
		Expect(result.Workflows[0].WorkflowID).To(Equal("wf-healthy-v1"))

		By("verifying healthy session was reused, not replaced")
		Expect(atomic.LoadInt32(&callToolInvoked)).To(Equal(int32(1)),
			"SI-4: healthy session must be reused — CallTool invoked exactly once on it")
		Expect(atomic.LoadInt32(&factoryCalled)).To(Equal(int32(0)),
			"SI-4: factory must not be called when cached session passes Ping")
		Expect(healthySession.isClosed()).To(BeFalse(),
			"SI-4: healthy session must NOT be closed")
	})
})

// =============================================================================
// IT-AF-1442: InjectVerified Wiring at Investigation Handoff (#1442)
//
// Pyramid Invariant:
//   UT (session_pool_test.go) proves InjectVerified logic (ping fail/pass).
//   IT (this block) proves InjectVerified is wired at the investigation
//   handoff path: HandleInvestigationMCPWithRegistry → pool.InjectVerified.
//   A dead session returned by StartInvestigation is rejected by InjectVerified
//   and never enters the pool — exercised through the production dispatch path.
// =============================================================================

var _ = Describe("InjectVerified Wiring at Investigation Handoff (#1442)", Label("integration", "session-handoff", "1442"), func() {

	It("IT-AF-1442-W01: dead session rejected by InjectVerified during investigation handoff", func() {
		deadSession := &handoffSession{
			id: "dead-handoff-session",
			pingFn: func(_ context.Context, _ *mcp.PingParams) error {
				return fmt.Errorf("transport closed")
			},
		}

		eventCh := make(chan ka.InvestigationEvent, 10)
		mockMCP := &ka.MockMCPClient{
			StartInvestigationFn: func(_ context.Context, _ ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
				go func() {
					eventCh <- ka.InvestigationEvent{
						Type: ka.EventTypeComplete,
						Data: json.RawMessage(`{}`),
					}
					close(eventCh)
				}()
				return &ka.StartInvestigationResult{
					SessionID: "sess-it-1442-w01",
					Status:    "autonomous_started",
					Events:    eventCh,
					Closer:    func() {},
					Session:   deadSession,
				}, nil
			},
		}

		pool := ka.NewKASessionPool(ka.PoolConfig{
			Factory: func(_ context.Context) (ka.PoolSession, error) {
				return &handoffSession{id: "factory-sentinel"}, nil
			},
			MaxEntries: 10,
			Logger:     logr.Discard(),
		})
		registry := tools.NewMonitorRegistry()

		result, err := tools.HandleInvestigationMCPWithRegistry(
			context.Background(), &tools.InvestigateConfig{
				MCPClient: mockMCP,
				Registry:  registry,
				Pool:      pool,
			}, tools.InvestigateMCPArgs{RRID: "rr-it-1442-w01"}, true, "alice",
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal("completed"))

		By("verifying dead session was NOT injected into pool")
		Expect(pool.Size()).To(Equal(0),
			"InjectVerified must reject dead session — pool must remain empty")

		By("verifying dead session was closed by InjectVerified")
		Expect(deadSession.isClosed()).To(BeTrue(),
			"InjectVerified must close the dead session after ping failure")
	})

	It("IT-AF-1442-W02: live session accepted by InjectVerified during investigation handoff", func() {
		liveSession := &handoffSession{id: "live-handoff-session"}

		eventCh := make(chan ka.InvestigationEvent, 10)
		mockMCP := &ka.MockMCPClient{
			StartInvestigationFn: func(_ context.Context, _ ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
				go func() {
					eventCh <- ka.InvestigationEvent{
						Type: ka.EventTypeComplete,
						Data: json.RawMessage(`{}`),
					}
					close(eventCh)
				}()
				return &ka.StartInvestigationResult{
					SessionID: "sess-it-1442-w02",
					Status:    "autonomous_started",
					Events:    eventCh,
					Closer:    func() {},
					Session:   liveSession,
				}, nil
			},
		}

		pool := ka.NewKASessionPool(ka.PoolConfig{
			Factory: func(_ context.Context) (ka.PoolSession, error) {
				return &handoffSession{id: "factory-sentinel"}, nil
			},
			MaxEntries: 10,
			Logger:     logr.Discard(),
		})
		registry := tools.NewMonitorRegistry()

		result, err := tools.HandleInvestigationMCPWithRegistry(
			context.Background(), &tools.InvestigateConfig{
				MCPClient: mockMCP,
				Registry:  registry,
				Pool:      pool,
			}, tools.InvestigateMCPArgs{RRID: "rr-it-1442-w02"}, true, "bob",
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal("completed"))

		By("verifying live session was injected into pool via InjectVerified")
		Expect(pool.Size()).To(Equal(1),
			"InjectVerified must accept live session — pool must contain it")
		Expect(liveSession.isClosed()).To(BeFalse(),
			"live session must remain open for reuse")
	})
})

// =============================================================================
// IT-AF-1452: Session ID Forwarding (BR-INTERACTIVE-010, #1452)
//
// Wiring Manifest rows covered:
//   - StartInvestigationArgs.SessionID  (IT-AF-1452-001)
//   - SDKMCPClient session_id argsMap   (IT-AF-1452-001)
//
// Pyramid Invariant:
//   UT proves logic (ka_investigate_mcp_test.go, start_investigation_test.go).
//   IT (this file) proves wiring through the AIA polling -> capture -> MCP forward path.
// =============================================================================

var _ = Describe("Session ID Forwarding (#1452)", Label("integration", "session-forwarding"), func() {

	Describe("IT-AF-1452-001 [SI-4+SC-8]: AIA CRD session ID flows through AF to MCP client", func() {
		It("should capture session ID from AIA CRD and forward it to StartInvestigation", func() {
			const aiaSessionID = "ka-sess-it-1452-001"
			var receivedArgs ka.StartInvestigationArgs
			eventCh := make(chan ka.InvestigationEvent, 10)

			mockMCP := &ka.MockMCPClient{
				StartInvestigationFn: func(_ context.Context, args ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
					receivedArgs = args
					return &ka.StartInvestigationResult{
						SessionID: aiaSessionID,
						Status:    "started",
						Events:    eventCh,
						Closer:    func() { close(eventCh) },
					}, nil
				},
			}

			s := runtime.NewScheme()
			_ = aiav1alpha1.AddToScheme(s)
			_ = corev1.AddToScheme(s)

			aiaObj := &aiav1alpha1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "kubernaut-system",
					Name:      "aia-rr-it-1452-001",
				},
				Spec: aiav1alpha1.AIAnalysisSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Name:      "rr-it-1452-001",
						Namespace: "kubernaut-system",
					},
					RemediationID: "rr-it-1452-001",
				},
				Status: aiav1alpha1.AIAnalysisStatus{
					KASession: &aiav1alpha1.KASession{
						ID: aiaSessionID,
					},
				},
			}
			tc := fake.NewClientBuilder().
				WithScheme(s).
				WithObjects(aiaObj).
				WithStatusSubresource(aiaObj).
				Build()

			recorder := &itAuditRecorder{}
			registry := tools.NewMonitorRegistry()

			ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
				Username: "it-user-1452",
				Groups:   []string{"sre"},
			})

			result, err := tools.HandleInvestigationMCPWithRegistry(
				ctx, &tools.InvestigateConfig{
					MCPClient: mockMCP,
					Client:    tc,
					Namespace: "kubernaut-system",
					Auditor:   recorder,
					Registry:  registry,
				}, tools.InvestigateMCPArgs{RRID: "rr-it-1452-001"}, false, "",
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.SessionID).To(Equal(aiaSessionID))

			By("verifying session ID was forwarded to MCP client (SC-8: transmission integrity)")
			Expect(receivedArgs.SessionID).To(Equal(aiaSessionID),
				"SC-8: AIA CRD session ID must be forwarded unmodified through the full AF dispatch path")
			Expect(receivedArgs.RRID).To(Equal("rr-it-1452-001"))

			By("verifying audit event references KA-confirmed session ID (AU-3: audit records)")
			Expect(recorder.events).To(HaveLen(1))
			Expect(recorder.events[0].Detail["ka_correlation_id"]).To(Equal(aiaSessionID),
				"AU-3: delegation audit event must reference the KA-confirmed session ID")
		})
	})
})

type itAuditRecorder struct {
	events []*audit.Event
}

func (r *itAuditRecorder) Emit(_ context.Context, e *audit.Event) {
	r.events = append(r.events, e)
}

var _ audit.Emitter = (*itAuditRecorder)(nil)
