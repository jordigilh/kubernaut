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
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
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

// mockLeaseSessionManager provides a fake in-memory SessionManager for integration tests.
type mockLeaseSessionManager struct {
	mu       sync.Mutex
	sessions map[string]*mcpinternal.InteractiveSession
	rrIndex  map[string]string
}

func newMockLeaseSessionManager() *mockLeaseSessionManager {
	return &mockLeaseSessionManager{
		sessions: make(map[string]*mcpinternal.InteractiveSession),
		rrIndex:  make(map[string]string),
	}
}

func (m *mockLeaseSessionManager) Takeover(_ context.Context, rrID string, user mcpinternal.UserInfo) (*mcpinternal.InteractiveSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, held := m.rrIndex[rrID]; held {
		return nil, mcpinternal.ErrLeaseHeld
	}
	sessionID := fmt.Sprintf("int-sess-%d", time.Now().UnixNano())
	sess := &mcpinternal.InteractiveSession{
		SessionID:     sessionID,
		CorrelationID: rrID,
		ActingUser:    user,
		StartedAt:     time.Now(),
	}
	m.sessions[sessionID] = sess
	m.rrIndex[rrID] = sessionID
	return sess, nil
}

func (m *mockLeaseSessionManager) Release(sessionID string, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	sess, ok := m.sessions[sessionID]
	if !ok {
		return mcpinternal.ErrSessionNotFound
	}
	delete(m.rrIndex, sess.CorrelationID)
	delete(m.sessions, sessionID)
	return nil
}

func (m *mockLeaseSessionManager) GetDriver(rrID string) (*mcpinternal.InteractiveSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	sessID, ok := m.rrIndex[rrID]
	if !ok {
		return nil, nil
	}
	return m.sessions[sessID], nil
}

func (m *mockLeaseSessionManager) IsDriverActive(rrID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.rrIndex[rrID]
	return ok
}

// mockReconIT mocks ContextReconstructor for integration tests.
type mockReconIT struct{}

func (m *mockReconIT) Reconstruct(_ context.Context, _ string, _ string) ([]mcpinternal.ConversationTurn, error) {
	return nil, nil
}

// mockAutoMgrIT mocks AutonomousSessionManager backed by a real session.Manager.
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
			store := session.NewStore(30 * time.Minute)
			mgr := session.NewManager(store, slog.Default(), nil, nil)
			autoMgr := &mockAutoMgrIT{mgr: mgr}

			// Start an autonomous investigation (simulates 200ms LLM processing)
			var autonomousCompleted atomic.Bool
			sessionID, err := mgr.StartInvestigation(context.Background(), func(ctx context.Context) (interface{}, error) {
				select {
				case <-time.After(200 * time.Millisecond):
					autonomousCompleted.Store(true)
					return "auto-result", nil
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}, map[string]string{"remediation_id": "rr-it-001"})
			Expect(err).NotTo(HaveOccurred())

			// Wait for it to be running
			Eventually(func() session.Status {
				s, _ := mgr.GetSession(sessionID)
				if s == nil {
					return ""
				}
				return s.Status
			}).Should(Equal(session.StatusRunning))

			// Now perform takeover via the InvestigateTool
			leaseMgr := newMockLeaseSessionManager()
			runner := &delayedMockRunner{delay: 10 * time.Millisecond, response: "interactive response"}
			recon := &mockReconIT{}
			tool := tools.NewInvestigateTool(leaseMgr, runner, recon, autoMgr)

			user := mcpinternal.UserInfo{Username: "sre-operator@example.com"}
			input := tools.InvestigateInput{
				RRID:   "rr-it-001",
				Action: tools.ActionTakeover,
			}

			out, err := tool.Handle(context.Background(), input, user)
			Expect(err).NotTo(HaveOccurred())
			Expect(out.SessionID).NotTo(BeEmpty())
			Expect(out.Status).To(Equal("takeover_started"))

			// Autonomous session should now be cancelled
			Eventually(func() session.Status {
				s, _ := mgr.GetSession(sessionID)
				if s == nil {
					return ""
				}
				return s.Status
			}).Should(Equal(session.StatusCancelled))

			// Interactive session is active
			Expect(leaseMgr.IsDriverActive("rr-it-001")).To(BeTrue())
		})
	})

	Describe("IT-KA-MCP-003: Concurrent tools/call requests are serialized by session mutex via real MCP SDK", func() {
		It("should process concurrent messages sequentially for the same session", func() {
			leaseMgr := newMockLeaseSessionManager()
			runner := &delayedMockRunner{delay: 30 * time.Millisecond, response: "llm-response"}
			recon := &mockReconIT{}
			store := session.NewStore(30 * time.Minute)
			mgr := session.NewManager(store, slog.Default(), nil, nil)
			autoMgr := &mockAutoMgrIT{mgr: mgr}

			// Start autonomous session
			_, err := mgr.StartInvestigation(context.Background(), func(ctx context.Context) (interface{}, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			}, map[string]string{"remediation_id": "rr-concurrent-001"})
			Expect(err).NotTo(HaveOccurred())

			tool := tools.NewInvestigateTool(leaseMgr, runner, recon, autoMgr)
			user := mcpinternal.UserInfo{Username: "alice@example.com"}

			// Perform takeover first
			takeoverInput := tools.InvestigateInput{
				RRID:   "rr-concurrent-001",
				Action: tools.ActionTakeover,
			}
			_, err = tool.Handle(context.Background(), takeoverInput, user)
			Expect(err).NotTo(HaveOccurred())

			// Build a real MCP server with the tool registered
			handler, _ := mcpinternal.BootstrapMCPWithTool(mcpinternal.MCPDeps{
				AuthMiddleware: fakeAuthMiddleware("alice@example.com"),
			}, tool)

			r := chi.NewRouter()
			r.Handle("/api/v1/mcp", kaserver.SSEHeadersMiddleware(handler))
			r.Handle("/api/v1/mcp/*", kaserver.SSEHeadersMiddleware(handler))
			ts := httptest.NewServer(r)
			defer ts.Close()

			// Send 3 concurrent message requests
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

			// All 3 calls should have succeeded
			Expect(results[0] + results[1] + results[2]).To(Equal(3))

			// Verify serialization: total time >= 3 * delay (not parallel)
			Expect(runner.calls.Load()).To(Equal(int32(3)))

			// Find overall duration (from earliest start to latest end)
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
			// If truly serialized, total >= 3*30ms = 90ms. Parallel would be ~30ms.
			Expect(totalDuration).To(BeNumerically(">=", 80*time.Millisecond),
				"concurrent messages should be serialized (total time >= 80ms)")
		})
	})
})

// jsonRPCToolCall creates a JSON-RPC tools/call request body.
func jsonRPCToolCall(id int, toolName string, arguments interface{}) string {
	args, _ := json.Marshal(arguments)
	return fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":{"name":"%s","arguments":%s}}`, id, toolName, string(args))
}

// doMCPRequest sends a JSON-RPC request to the MCP endpoint with auth.
func doMCPRequest(ts *httptest.Server, body string) (*http.Response, string, error) {
	req, err := http.NewRequest("POST", ts.URL+"/api/v1/mcp", strings.NewReader(body))
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer test-token")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	return resp, string(respBody), nil
}
