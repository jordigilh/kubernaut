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
	"sync/atomic"

	"github.com/go-logr/logr"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

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

// handoffSession is a trackable PoolSession used by IT-AF-1332 tests.
type handoffSession struct {
	id       string
	callFn   func(ctx context.Context, params *mcp.CallToolParams) (*mcp.CallToolResult, error)
	closed   int32
}

func (s *handoffSession) CallTool(ctx context.Context, params *mcp.CallToolParams) (*mcp.CallToolResult, error) {
	if s.callFn != nil {
		return s.callFn(ctx, params)
	}
	return &mcp.CallToolResult{}, nil
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
				context.Background(), mockMCP, nil, "",
				tools.InvestigateMCPArgs{RRID: "rr-it-1332-010"},
				nil, registry, nil, true, pool, "alice",
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
				context.Background(), mockMCP, nil, "",
				tools.InvestigateMCPArgs{RRID: "rr-it-1332-020"},
				nil, registry, nil, true, pool, "bob",
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
				context.Background(), mockMCP, nil, "",
				tools.InvestigateMCPArgs{RRID: "rr-it-1332-030"},
				nil, nil, nil, true, nil, "",
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Status).To(Equal("completed"))

			By("verifying cleanup was called exactly once")
			Expect(atomic.LoadInt32(&closerCalled)).To(Equal(int32(1)),
				"cleanup (closer) must be called when pool is nil — MCP bridge path behavior")
		})
	})
})
