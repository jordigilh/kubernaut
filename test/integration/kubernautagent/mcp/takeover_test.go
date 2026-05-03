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

	"github.com/go-logr/logr"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/tools"
	kaserver "github.com/jordigilh/kubernaut/internal/kubernautagent/server"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// delayedMockRunner simulates an LLM that takes time to respond.
type delayedMockRunner struct {
	delay    time.Duration
	response string
	calls    atomic.Int32
}

func (m *delayedMockRunner) RunInteractiveTurn(_ context.Context, _ []tools.LLMMessage, _ string) (string, error) {
	m.calls.Add(1)
	time.Sleep(m.delay)
	return m.response, nil
}

// mockReconIT mocks ContextReconstructor for integration tests.
type mockReconIT struct{}

func (m *mockReconIT) Reconstruct(_ context.Context, _ string, _ string) ([]mcpinternal.ConversationTurn, error) {
	return nil, nil
}

// mockAutoMgrIT wraps a real session.Manager for autonomous session management in tests.
type mockAutoMgrIT struct {
	mgr *session.Manager
}

func (m *mockAutoMgrIT) FindByRemediationID(rrID string) (string, bool) {
	return m.mgr.FindByRemediationID(rrID)
}

func (m *mockAutoMgrIT) CancelInvestigation(id string) error {
	return m.mgr.CancelInvestigation(id)
}

var _ = Describe("MCP Dynamic Takeover Integration — PR4 BR-INTERACTIVE-004", func() {

	Describe("IT-KA-TAKE-001: Takeover mid-LLM-turn — autonomous cancelled after turn completes", func() {
		It("should cancel the autonomous session and acquire the interactive lease", func() {
			nsName := uniqueNamespace("take01")
			createNamespace(context.Background(), sharedK8sClient, nsName)

			store := session.NewStore(30 * time.Minute)
			mgr := session.NewManager(store, logr.Discard(), nil, nil)
			autoMgr := &mockAutoMgrIT{mgr: mgr}

			var autonomousCompleted atomic.Bool
			sessionID, err := mgr.StartInvestigation(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
				select {
				case <-time.After(200 * time.Millisecond):
					autonomousCompleted.Store(true)
					return &katypes.InvestigationResult{RCASummary: "auto-result"}, nil
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}, map[string]string{"remediation_id": "rr-it-001"})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				s, _ := mgr.GetSession(sessionID)
				if s == nil {
					return ""
				}
				return s.Status
			}).Should(Equal(session.StatusRunning))

			logger := logr.Discard()
			leaseMgr := mcpinternal.NewLeaseSessionManagerConcrete(sharedK8sClient, nsName, logger)
			runner := &delayedMockRunner{delay: 10 * time.Millisecond, response: "interactive response"}
			recon := &mockReconIT{}
			tool := tools.NewInvestigateTool(leaseMgr, runner, recon, tools.WithAutonomousManager(autoMgr))

			user := mcpinternal.UserInfo{Username: "sre-operator@example.com"}
			input := tools.InvestigateInput{
				RRID:   "rr-it-001",
				Action: tools.ActionTakeover,
			}

			out, err := tool.Handle(context.Background(), input, user)
			Expect(err).NotTo(HaveOccurred())
			Expect(out.SessionID).NotTo(BeEmpty())
			Expect(out.Status).To(Equal("takeover_started"))

			Eventually(func() session.Status {
				s, _ := mgr.GetSession(sessionID)
				if s == nil {
					return ""
				}
				return s.Status
			}).Should(Equal(session.StatusCancelled))

			Expect(leaseMgr.IsDriverActive("rr-it-001")).To(BeTrue())
		})
	})

	Describe("IT-KA-MCP-003: Concurrent tools/call requests are serialized by session mutex via real MCP SDK", func() {
		It("should process concurrent messages sequentially for the same session", func() {
			nsName := uniqueNamespace("conc")
			createNamespace(context.Background(), sharedK8sClient, nsName)

			logger := logr.Discard()
			leaseMgr := mcpinternal.NewLeaseSessionManagerConcrete(sharedK8sClient, nsName, logger)
			runner := &delayedMockRunner{delay: 30 * time.Millisecond, response: "llm-response"}
			recon := &mockReconIT{}
			store := session.NewStore(30 * time.Minute)
			mgr := session.NewManager(store, logr.Discard(), nil, nil)
			autoMgr := &mockAutoMgrIT{mgr: mgr}

			_, err := mgr.StartInvestigation(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			}, map[string]string{"remediation_id": "rr-concurrent-001"})
			Expect(err).NotTo(HaveOccurred())

			tool := tools.NewInvestigateTool(leaseMgr, runner, recon, tools.WithAutonomousManager(autoMgr))
			user := mcpinternal.UserInfo{Username: "alice@example.com"}

			takeoverInput := tools.InvestigateInput{
				RRID:   "rr-concurrent-001",
				Action: tools.ActionTakeover,
			}
			_, err = tool.Handle(context.Background(), takeoverInput, user)
			Expect(err).NotTo(HaveOccurred())

			handler, _ := mcpinternal.BootstrapMCP(mcpinternal.MCPDeps{
				AuthMiddleware: fakeAuthMiddleware("alice@example.com"),
				Tools: mcpinternal.ToolDeps{
					Investigate: tools.InvestigateRegistration(tool, nil, nil),
				},
			})

			r := chi.NewRouter()
			r.Use(fakeAuthMiddleware("alice@example.com"))
			r.Handle("/api/v1/mcp", kaserver.SSEHeadersMiddleware(handler))
			r.Handle("/api/v1/mcp/*", kaserver.SSEHeadersMiddleware(handler))
			ts := httptest.NewServer(r)
			defer ts.Close()

			var wg sync.WaitGroup
			results := make([]int, 3)
			startTimes := make([]time.Time, 3)
			endTimes := make([]time.Time, 3)

			for i := 0; i < 3; i++ {
				wg.Add(1)
				go func(idx int) {
					defer wg.Done()
					startTimes[idx] = time.Now()

					msgInput := tools.InvestigateInput{
						RRID:    "rr-concurrent-001",
						Action:  tools.ActionMessage,
						Message: fmt.Sprintf("message %d", idx),
					}
					_, handleErr := tool.Handle(context.Background(), msgInput, user)
					endTimes[idx] = time.Now()
					if handleErr == nil {
						results[idx] = 1
					}
				}(i)
			}
			wg.Wait()

			Expect(results[0] + results[1] + results[2]).To(Equal(3))
			Expect(runner.calls.Load()).To(Equal(int32(3)))

			var earliest, latest time.Time
			for i := 0; i < 3; i++ {
				if earliest.IsZero() || startTimes[i].Before(earliest) {
					earliest = startTimes[i]
				}
				if endTimes[i].After(latest) {
					latest = endTimes[i]
				}
			}
			totalDuration := latest.Sub(earliest)
			Expect(totalDuration).To(BeNumerically(">=", 80*time.Millisecond),
				"concurrent messages should be serialized (total time >= 80ms)")
		})
	})
})
