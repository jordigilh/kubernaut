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

package server_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/go-chi/chi/v5"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	kaserver "github.com/jordigilh/kubernaut/internal/kubernautagent/server"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	katypes "github.com/jordigilh/kubernaut/internal/kubernautagent/types"
	"github.com/jordigilh/kubernaut/pkg/agentclient"
	"github.com/jordigilh/kubernaut/pkg/shared/auth"
)

var _ = Describe("Wiring Integration Tests — #823", func() {

	// ---------------------------------------------------------------
	// IT-WIRE-01: SSE headers present on real HTTP response
	// ---------------------------------------------------------------
	Describe("IT-WIRE-01: SSE middleware headers on HTTP stream response", func() {
		It("returns Cache-Control, Connection, X-Accel-Buffering on /stream", func() {
			h := newTestHarness()
			defer h.Close()

			proceed := make(chan struct{})
			id := startInvestigationWithFunc(h, func(ctx context.Context) (interface{}, error) {
				<-proceed
				return &katypes.InvestigationResult{RCASummary: "done"}, nil
			})
			waitForStatus(h, id, session.StatusRunning)

			type result struct {
				resp *http.Response
				err  error
			}
			ch := make(chan result, 1)
			go func() {
				r, e := http.Get(h.Server.URL + "/api/v1/incident/session/" + id + "/stream")
				ch <- result{r, e}
			}()

			Eventually(func() int {
				return len(h.AuditStore.EventsOfType(audit.EventTypeSessionObserved))
			}, 5*time.Second).Should(BeNumerically(">=", 1),
				"subscriber must connect before investigation completes")
			close(proceed)

			res := <-ch
			Expect(res.err).NotTo(HaveOccurred())
			defer res.resp.Body.Close()

			Expect(res.resp.Header.Get("Cache-Control")).To(Equal("no-cache"),
				"SSEHeadersMiddleware must set Cache-Control")
			Expect(res.resp.Header.Get("X-Accel-Buffering")).To(Equal("no"),
				"SSEHeadersMiddleware must set X-Accel-Buffering for reverse proxy compatibility")
		})
	})

	// ---------------------------------------------------------------
	// IT-WIRE-02: Rate limiter returns 429 when burst exceeded
	// ---------------------------------------------------------------
	Describe("IT-WIRE-02: Rate limiter rejects excess requests with 429", func() {
		It("returns 429 after burst is exhausted", func() {
			h := newTestHarness(withRateLimit(kaserver.RateLimitConfig{
				RequestsPerSecond: 1,
				Burst:             2,
				CleanupInterval:   time.Hour,
				MaxAge:            time.Hour,
			}))
			defer h.Close()

			codes := make([]int, 3)
			for i := 0; i < 3; i++ {
				resp, err := http.Get(h.Server.URL + "/api/v1/incident/session/nonexistent")
				Expect(err).NotTo(HaveOccurred())
				codes[i] = resp.StatusCode
				resp.Body.Close()
			}

			Expect(codes[2]).To(Equal(http.StatusTooManyRequests),
				"third request should be rate-limited (429)")
		})
	})

	// ---------------------------------------------------------------
	// IT-WIRE-04: Lazy sink activates mid-investigation via HTTP
	// ---------------------------------------------------------------
	Describe("IT-WIRE-04: Lazy sink activates when subscriber connects via HTTP", func() {
		It("SSE stream receives events after subscribe triggers lazy sink", func() {
			h := newTestHarness()
			defer h.Close()

			proceed := make(chan struct{})
			id := startInvestigationWithFunc(h, func(ctx context.Context) (interface{}, error) {
				<-proceed
				sink := session.EventSinkFromContext(ctx)
				if sink != nil {
					sink <- session.InvestigationEvent{
						Type:  session.EventTypeReasoningDelta,
						Turn:  0,
						Phase: "rca",
					}
				}
				return map[string]string{"rca_summary": "delivered"}, nil
			})
			waitForStatus(h, id, session.StatusRunning)

			type sseResult struct {
				body string
				err  error
			}
			ch := make(chan sseResult, 1)
			go func() {
				resp, err := http.Get(h.Server.URL + "/api/v1/incident/session/" + id + "/stream")
				if err != nil {
					ch <- sseResult{"", err}
					return
				}
				defer resp.Body.Close()
				data, readErr := io.ReadAll(resp.Body)
				ch <- sseResult{string(data), readErr}
			}()

			Eventually(func() int {
				return len(h.AuditStore.EventsOfType(audit.EventTypeSessionObserved))
			}, 5*time.Second).Should(BeNumerically(">=", 1),
				"subscriber must connect before investigation completes")
			close(proceed)

			var res sseResult
			Eventually(func() bool {
				select {
				case res = <-ch:
					return true
				default:
					return false
				}
			}, 10*time.Second).Should(BeTrue(), "SSE stream should complete")

			Expect(res.err).NotTo(HaveOccurred())
			Expect(res.body).To(ContainSubstring("event: reasoning_delta"),
				"lazy sink should activate on HTTP subscribe, delivering events")
		})
	})

	// ---------------------------------------------------------------
	// IT-WIRE-05: Error sanitization — 500 does not leak internals
	// ---------------------------------------------------------------
	Describe("IT-WIRE-05: 500 responses do not leak internal error details", func() {
		It("status endpoint returns generic error for unexpected failures", func() {
			h := newTestHarness()
			defer h.Close()

			resp, err := http.Get(h.Server.URL + "/api/v1/incident/session/nonexistent-id")
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)
			bodyStr := string(body)
			Expect(bodyStr).NotTo(ContainSubstring("session lookup:"),
				"error detail must not contain internal wrapping")
			Expect(bodyStr).NotTo(ContainSubstring("runtime error"),
				"error detail must not contain Go runtime errors")
		})
	})

	// ---------------------------------------------------------------
	// IT-WIRE-08: Cross-user authz returns 404 via real HTTP
	// ---------------------------------------------------------------
	Describe("IT-WIRE-08: Object-level authz returns 404 for non-owner via HTTP", func() {
		It("session endpoints return 404 when requested by wrong user", func() {
			h := newTestHarness(withAuthUser("user-a"))
			defer h.Close()

			id := startInvestigationAsUser(h, "user-a")
			waitForStatus(h, id, session.StatusRunning)

			h2 := newTestHarnessWithManager(h.Manager, h.AuditStore, "user-b")
			defer h2.Close()

			resp, err := http.Get(h2.Server.URL + "/api/v1/incident/session/" + id)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusNotFound),
				"non-owner should receive 404 for session status")

			h.Manager.CancelInvestigation(id)
		})
	})

	// ---------------------------------------------------------------
	// IT-WIRE-09: Access-denied audit event stored on unauthorized request
	// ---------------------------------------------------------------
	Describe("IT-WIRE-09: Access-denied audit event on unauthorized HTTP request", func() {
		It("emits aiagent.session.access_denied with correct attributes", func() {
			h := newTestHarness(withAuthUser("owner-user"))
			defer h.Close()

			id := startInvestigationAsUser(h, "owner-user")
			waitForStatus(h, id, session.StatusRunning)

			h2 := newTestHarnessWithManager(h.Manager, h.AuditStore, "attacker-user")
			defer h2.Close()

			resp, err := http.Get(h2.Server.URL + "/api/v1/incident/session/" + id)
			Expect(err).NotTo(HaveOccurred())
			resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusNotFound),
				"attacker-user should receive 404 from shared manager")

			Eventually(func() int {
				return len(h.AuditStore.EventsOfType(audit.EventTypeSessionAccessDenied))
			}, 5*time.Second).Should(BeNumerically(">=", 1),
				"access_denied event should be recorded for unauthorized access")

			denied := h.AuditStore.EventsOfType(audit.EventTypeSessionAccessDenied)
			Expect(denied[0].Data["requesting_user"]).To(Equal("attacker-user"))
			Expect(denied[0].Data["session_id"]).To(Equal(id))

			h.Manager.CancelInvestigation(id)
		})
	})

	// ---------------------------------------------------------------
	// IT-WIRE-10: Subscribe audit event with observer_user via HTTP
	// ---------------------------------------------------------------
	Describe("IT-WIRE-10: Subscribe audit event with observer_user", func() {
		It("emits aiagent.session.observed with observer identity on HTTP stream", func() {
			h := newTestHarness(withAuthUser("observer-user"))
			defer h.Close()

			id := startInvestigationAsUser(h, "observer-user")
			waitForStatus(h, id, session.StatusRunning)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			req, _ := http.NewRequestWithContext(ctx, "GET", h.Server.URL+"/api/v1/incident/session/"+id+"/stream", nil)

			go func() {
				resp, err := http.DefaultClient.Do(req)
				if err == nil {
					resp.Body.Close()
				}
			}()

			Eventually(func() int {
				return len(h.AuditStore.EventsOfType(audit.EventTypeSessionObserved))
			}, 5*time.Second).Should(BeNumerically(">=", 1),
				"session.observed event should be emitted when observer connects via HTTP")

			observed := h.AuditStore.EventsOfType(audit.EventTypeSessionObserved)
			Expect(observed[0].Data["observer_user"]).To(Equal("observer-user"))

			h.Manager.CancelInvestigation(id)
		})
	})

	// ---------------------------------------------------------------
	// IT-WIRE-11: EventTypeComplete flows through SSE to client
	// ---------------------------------------------------------------
	Describe("IT-WIRE-11: Complete event flows through SSE to HTTP client", func() {
		It("SSE body contains event: complete when investigation finishes", func() {
			h := newTestHarness()
			defer h.Close()

			proceed := make(chan struct{})
			id := startInvestigationWithFunc(h, func(ctx context.Context) (interface{}, error) {
				<-proceed
				return &katypes.InvestigationResult{RCASummary: "done"}, nil
			})
			waitForStatus(h, id, session.StatusRunning)

			type sseResult struct {
				body string
				err  error
			}
			ch := make(chan sseResult, 1)
			go func() {
				resp, err := http.Get(h.Server.URL + "/api/v1/incident/session/" + id + "/stream")
				if err != nil {
					ch <- sseResult{"", err}
					return
				}
				defer resp.Body.Close()
				data, readErr := io.ReadAll(resp.Body)
				ch <- sseResult{string(data), readErr}
			}()

			Eventually(func() int {
				return len(h.AuditStore.EventsOfType(audit.EventTypeSessionObserved))
			}, 5*time.Second).Should(BeNumerically(">=", 1),
				"subscriber must connect before investigation completes")
			close(proceed)

			var res sseResult
			Eventually(func() bool {
				select {
				case res = <-ch:
					return true
				default:
					return false
				}
			}, 10*time.Second).Should(BeTrue(), "SSE stream should complete")

			Expect(res.err).NotTo(HaveOccurred())
			Expect(res.body).To(ContainSubstring("event: complete"),
				"SSE stream must contain EventTypeComplete when investigation finishes")
		})
	})

	// ---------------------------------------------------------------
	// IT-WIRE-12: Snapshot endpoint returns enriched fields after completion
	// ---------------------------------------------------------------
	Describe("IT-WIRE-12: Snapshot returns enriched fields after investigation completes", func() {
		It("GET /snapshot returns session_id, status, rca_summary, and token counts", func() {
			h := newTestHarness()
			defer h.Close()

			id := startInvestigationWithFunc(h, func(ctx context.Context) (interface{}, error) {
				return &katypes.InvestigationResult{
					RCASummary: "pod OOM killed due to memory leak in worker container",
					Confidence: 0.92,
					TokenUsage: &katypes.TokenUsageSummary{
						PromptTokens:     800,
						CompletionTokens: 200,
					},
				}, nil
			})
			waitForStatus(h, id, session.StatusCompleted)

			resp, err := http.Get(h.Server.URL + "/api/v1/incident/session/" + id + "/snapshot")
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusOK),
				"snapshot of completed session should return 200")

			body, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())

			var snap map[string]interface{}
			Expect(json.Unmarshal(body, &snap)).To(Succeed())
			Expect(snap["session_id"]).To(Equal(id))
			Expect(snap["status"]).To(Equal("completed"))
			Expect(snap["rca_summary"]).To(Equal("pod OOM killed due to memory leak in worker container"))
			Expect(snap["total_prompt_tokens"]).To(BeNumerically("==", 800))
			Expect(snap["total_completion_tokens"]).To(BeNumerically("==", 200))
		})

		It("GET /snapshot returns 409 for in-progress session", func() {
			h := newTestHarness()
			defer h.Close()

			id := startInvestigation(h)
			waitForStatus(h, id, session.StatusRunning)

			resp, err := http.Get(h.Server.URL + "/api/v1/incident/session/" + id + "/snapshot")
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusConflict),
				"snapshot of running session should return 409 Conflict")

			h.Manager.CancelInvestigation(id)
		})
	})

	// ---------------------------------------------------------------
	// IT-WIRE-13: Token delta events flow through SSE to client
	// ---------------------------------------------------------------
	Describe("IT-WIRE-13: Token delta events flow through SSE to HTTP client", func() {
		It("SSE body contains event: token_delta when investigation streams tokens", func() {
			h := newTestHarness()
			defer h.Close()

			proceed := make(chan struct{})
			id := startInvestigationWithFunc(h, func(ctx context.Context) (interface{}, error) {
				<-proceed
				sink := session.EventSinkFromContext(ctx)
				if sink != nil {
					sink <- session.InvestigationEvent{
						Type:  session.EventTypeTokenDelta,
						Turn:  1,
						Phase: "rca",
						Data:  []byte(`{"delta":"The root cause"}`),
					}
					sink <- session.InvestigationEvent{
						Type:  session.EventTypeTokenDelta,
						Turn:  1,
						Phase: "rca",
						Data:  []byte(`{"delta":" is OOM"}`),
					}
				}
				return &katypes.InvestigationResult{RCASummary: "OOM killed"}, nil
			})
			waitForStatus(h, id, session.StatusRunning)

			type sseResult struct {
				body string
				err  error
			}
			ch := make(chan sseResult, 1)
			go func() {
				resp, err := http.Get(h.Server.URL + "/api/v1/incident/session/" + id + "/stream")
				if err != nil {
					ch <- sseResult{"", err}
					return
				}
				defer resp.Body.Close()
				data, readErr := io.ReadAll(resp.Body)
				ch <- sseResult{string(data), readErr}
			}()

			Eventually(func() int {
				return len(h.AuditStore.EventsOfType(audit.EventTypeSessionObserved))
			}, 5*time.Second).Should(BeNumerically(">=", 1),
				"subscriber must connect before investigation completes")
			close(proceed)

			var res sseResult
			Eventually(func() bool {
				select {
				case res = <-ch:
					return true
				default:
					return false
				}
			}, 10*time.Second).Should(BeTrue(), "SSE stream should complete")

			Expect(res.err).NotTo(HaveOccurred())
			Expect(res.body).To(ContainSubstring("event: token_delta"),
				"SSE stream must contain token_delta events when investigation streams tokens")
			Expect(res.body).To(ContainSubstring("The root cause"),
				"token_delta event data must flow through to the SSE body")
			Expect(res.body).To(ContainSubstring("event: complete"),
				"stream must terminate with complete event after token deltas")
		})
	})

	// ---------------------------------------------------------------
	// IT-WIRE-07: Retry/error events flow through SSE to client
	// ---------------------------------------------------------------
	Describe("IT-WIRE-07: Error events flow through SSE to HTTP client", func() {
		It("SSE body contains event: error when investigation emits an error event", func() {
			h := newTestHarness()
			defer h.Close()

			proceed := make(chan struct{})
			id := startInvestigationWithFunc(h, func(ctx context.Context) (interface{}, error) {
				<-proceed
				sink := session.EventSinkFromContext(ctx)
				if sink != nil {
					sink <- session.InvestigationEvent{
						Type:  session.EventTypeError,
						Turn:  1,
						Phase: "rca",
						Data:  []byte(`{"message":"LLM call failed, retrying"}`),
					}
				}
				return &katypes.InvestigationResult{RCASummary: "recovered"}, nil
			})
			waitForStatus(h, id, session.StatusRunning)

			type sseResult struct {
				body string
				err  error
			}
			ch := make(chan sseResult, 1)
			go func() {
				resp, err := http.Get(h.Server.URL + "/api/v1/incident/session/" + id + "/stream")
				if err != nil {
					ch <- sseResult{"", err}
					return
				}
				defer resp.Body.Close()
				data, readErr := io.ReadAll(resp.Body)
				ch <- sseResult{string(data), readErr}
			}()

			Eventually(func() int {
				return len(h.AuditStore.EventsOfType(audit.EventTypeSessionObserved))
			}, 5*time.Second).Should(BeNumerically(">=", 1),
				"subscriber must connect before investigation completes")
			close(proceed)

			var res sseResult
			Eventually(func() bool {
				select {
				case res = <-ch:
					return true
				default:
					return false
				}
			}, 10*time.Second).Should(BeTrue(), "SSE stream should complete")

			Expect(res.err).NotTo(HaveOccurred())
			Expect(res.body).To(ContainSubstring("event: error"),
				"SSE stream must contain error events emitted by the investigation")
			Expect(res.body).To(ContainSubstring("LLM call failed"),
				"error event data must flow through to the SSE body")
			Expect(res.body).To(ContainSubstring("event: complete"),
				"stream must still terminate with complete event after error")
		})
	})
})

// --- Helpers ---

// startInvestigation creates a session directly via the Manager (bypassing ogen
// schema validation) and returns the session ID. The investigation blocks until
// context is cancelled.
func startInvestigation(h *testHarness) string {
	id, err := h.Manager.StartInvestigation(context.Background(), func(ctx context.Context) (interface{}, error) {
		<-ctx.Done()
		return &katypes.InvestigationResult{RCASummary: "cancelled"}, nil
	}, map[string]string{"remediation_id": "rr-wire-test"})
	Expect(err).NotTo(HaveOccurred())
	return id
}

// startInvestigationAsUser creates a session with a user identity on the context,
// which the manager stores as "created_by" for authz checks.
func startInvestigationAsUser(h *testHarness, user string) string {
	ctx := context.WithValue(context.Background(), auth.UserContextKey, user)
	id, err := h.Manager.StartInvestigation(ctx, func(ctx context.Context) (interface{}, error) {
		<-ctx.Done()
		return &katypes.InvestigationResult{RCASummary: "cancelled"}, nil
	}, map[string]string{"remediation_id": "rr-wire-test"})
	Expect(err).NotTo(HaveOccurred())
	return id
}

// startInvestigationWithFunc creates a session with a custom investigation function.
func startInvestigationWithFunc(h *testHarness, fn func(ctx context.Context) (interface{}, error)) string {
	id, err := h.Manager.StartInvestigation(context.Background(), fn, map[string]string{"remediation_id": "rr-wire-test"})
	Expect(err).NotTo(HaveOccurred())
	return id
}

func waitForStatus(h *testHarness, id string, status session.Status) {
	Eventually(func() session.Status {
		s, _ := h.Manager.GetSession(id)
		if s == nil {
			return session.StatusPending
		}
		return s.Status
	}, 5*time.Second).Should(Equal(status))
}

// channelInvestigator waits for proceed, calls emitEvents, then completes.
type channelInvestigator struct {
	proceed    chan struct{}
	emitEvents func(ctx context.Context)
}

func (c *channelInvestigator) Investigate(ctx context.Context, _ katypes.SignalContext) (*katypes.InvestigationResult, error) {
	<-c.proceed
	if c.emitEvents != nil {
		c.emitEvents(ctx)
	}
	return &katypes.InvestigationResult{RCASummary: "done"}, nil
}

// newTestHarnessWithManager builds a second harness sharing the same Manager
// and audit store but with a different auth user. Used for cross-user tests.
func newTestHarnessWithManager(mgr *session.Manager, auditStore *syncAuditRecorder, user string) *testHarness {
	inv := &blockingInvestigator{}
	handler := kaserver.NewHandler(mgr, inv, slog.Default())
	ogenSrv, _ := agentclient.NewServer(handler)

	r := chi.NewRouter()
	rl := kaserver.NewRateLimiter(kaserver.DefaultRateLimitConfig())
	r.Use(rl.Middleware)
	if user != "" {
		r.Use(fakeAuthMiddleware(user))
	}
	r.Mount("/", kaserver.SSEHeadersMiddleware(ogenSrv))

	ts := httptest.NewServer(r)
	return &testHarness{
		Server:      ts,
		Manager:     mgr,
		AuditStore:  auditStore,
		RateLimiter: rl,
	}
}

