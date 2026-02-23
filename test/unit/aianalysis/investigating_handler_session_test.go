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

package aianalysis

import (
	"context"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/internal/controller/aianalysis"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/metrics"
	hgptclient "github.com/jordigilh/kubernaut/pkg/holmesgpt/client"
	"github.com/jordigilh/kubernaut/test/shared/mocks"
)

// ========================================
// BR-AA-HAPI-064: Session-Based Pull Design Unit Tests
//
// Test Plan: docs/testing/BR-AA-HAPI-064/session_based_pull_test_plan_v1.0.md
//
// These tests validate the async submit/poll/result flow between
// AA controller and HolmesGPT-API using session IDs.
//
// TDD Phase: RED - all tests expected to fail until GREEN phase implements
// the handler logic for async session management.
// ========================================

// sessionAuditSpy tracks session-related audit events for validation.
// BR-AUDIT-005: Audit as side-effect validation in unit tests.
type sessionAuditSpy struct {
	mu                sync.Mutex
	submitEvents      []sessionSubmitEvent
	resultEvents      []sessionResultEvent
	sessionLostEvents []sessionLostEvent
	failedEvents      []failedAnalysisEvent
}

type sessionSubmitEvent struct {
	analysis  *aianalysisv1.AIAnalysis
	sessionID string
}

type sessionResultEvent struct {
	analysis          *aianalysisv1.AIAnalysis
	investigationTime int64
}

type sessionLostEvent struct {
	analysis   *aianalysisv1.AIAnalysis
	generation int32
}

func (s *sessionAuditSpy) RecordAIAgentCall(ctx context.Context, analysis *aianalysisv1.AIAnalysis, endpoint string, statusCode int, durationMs int) {
}
func (s *sessionAuditSpy) RecordPhaseTransition(ctx context.Context, analysis *aianalysisv1.AIAnalysis, from, to string) {
}
func (s *sessionAuditSpy) RecordAnalysisFailed(ctx context.Context, analysis *aianalysisv1.AIAnalysis, err error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failedEvents = append(s.failedEvents, failedAnalysisEvent{analysis: analysis, err: err})
	return nil
}
func (s *sessionAuditSpy) RecordAnalysisComplete(ctx context.Context, analysis *aianalysisv1.AIAnalysis) {
}
func (s *sessionAuditSpy) RecordAIAgentSubmit(ctx context.Context, analysis *aianalysisv1.AIAnalysis, sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.submitEvents = append(s.submitEvents, sessionSubmitEvent{analysis: analysis, sessionID: sessionID})
}
func (s *sessionAuditSpy) RecordAIAgentResult(ctx context.Context, analysis *aianalysisv1.AIAnalysis, investigationTimeMs int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resultEvents = append(s.resultEvents, sessionResultEvent{analysis: analysis, investigationTime: investigationTimeMs})
}
func (s *sessionAuditSpy) RecordAIAgentSessionLost(ctx context.Context, analysis *aianalysisv1.AIAnalysis, generation int32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessionLostEvents = append(s.sessionLostEvents, sessionLostEvent{analysis: analysis, generation: generation})
}

// ========================================
// Test Suite
// ========================================

var _ = Describe("InvestigatingHandler Session-Based Pull (BR-AA-HAPI-064)", func() {
	var (
		handler    *handlers.InvestigatingHandler
		mockClient *mocks.MockHolmesGPTClient
		auditSpy   *sessionAuditSpy
		recorder   *record.FakeRecorder
		ctx        context.Context
	)

	// createSessionTestAnalysis creates a valid AIAnalysis for session tests
	createSessionTestAnalysis := func() *aianalysisv1.AIAnalysis {
		return &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-session-analysis",
				Namespace: "default",
			},
			Spec: aianalysisv1.AIAnalysisSpec{
				RemediationRequestRef: corev1.ObjectReference{
					Kind:      "RemediationRequest",
					Name:      "test-rr",
					Namespace: "default",
				},
				RemediationID: "test-remediation-session-001",
				AnalysisRequest: aianalysisv1.AnalysisRequest{
					SignalContext: aianalysisv1.SignalContextInput{
						Fingerprint:      "test-fingerprint",
						Severity:         "high",
						SignalName:       "OOMKilled",
						Environment:      "production",
						BusinessPriority: "P0",
						TargetResource: aianalysisv1.TargetResource{
							Kind:      "Pod",
							Name:      "test-pod",
							Namespace: "default",
						},
					},
					AnalysisTypes: []string{"investigation"},
				},
			},
			Status: aianalysisv1.AIAnalysisStatus{
				Phase: aianalysis.PhaseInvestigating,
			},
		}
	}

	BeforeEach(func() {
		ctx = context.Background()
		mockClient = mocks.NewMockHolmesGPTClient()
		auditSpy = &sessionAuditSpy{}
		recorder = record.NewFakeRecorder(20)
		testMetrics := metrics.NewMetrics()
		handler = handlers.NewInvestigatingHandler(mockClient, ctrl.Log.WithName("test-session"), testMetrics, auditSpy,
			handlers.WithSessionMode(), handlers.WithRecorder(recorder))
	})

	// ========================================
	// 1.1 Incident Submit Flow
	// ========================================
	Describe("Session Submit Flow", func() {
		// UT-AA-064-001: Submit investigation when InvestigationSession is nil
		Context("UT-AA-064-001: Submit investigation when InvestigationSession is nil", func() {
			It("should create a HAPI session and record it in CRD status", func() {
				analysis := createSessionTestAnalysis()
				// InvestigationSession is nil (first time)
				Expect(analysis.Status.InvestigationSession).To(BeNil())

				mockClient.WithSessionSubmitResponse("session-uuid-001")

				result, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())

				// CRD status: InvestigationSession populated
				Expect(analysis.Status.InvestigationSession).NotTo(BeNil(), "InvestigationSession should be populated after submit")
				Expect(analysis.Status.InvestigationSession.ID).To(Equal("session-uuid-001"), "Session ID should match HAPI response")
				Expect(analysis.Status.InvestigationSession.Generation).To(Equal(int32(0)), "Generation should be 0 for first session")
				Expect(analysis.Status.InvestigationSession.CreatedAt).NotTo(BeNil(), "CreatedAt should be set")

				// Condition: InvestigationSessionReady=True, Reason=SessionCreated
				cond := getCondition(analysis, "InvestigationSessionReady")
				Expect(cond).NotTo(BeNil(), "InvestigationSessionReady condition should be set")
				Expect(string(cond.Status)).To(Equal("True"))
				Expect(cond.Reason).To(Equal("SessionCreated"))

				// Result: RequeueAfter at configured session poll interval (non-blocking return for polling)
				Expect(result.RequeueAfter).To(Equal(handlers.DefaultSessionPollInterval), "Should requeue at session poll interval for first poll")

				// Audit side effect: exactly 1 aiagent.submit event
				Expect(auditSpy.submitEvents).To(HaveLen(1), "Should record exactly 1 submit audit event")
				Expect(auditSpy.submitEvents[0].sessionID).To(Equal("session-uuid-001"))

				// DD-EVENT-001: K8s Event -- SessionCreated (Normal)
				var evt string
				Eventually(recorder.Events).Should(Receive(&evt))
				Expect(evt).To(ContainSubstring("Normal"))
				Expect(evt).To(ContainSubstring("SessionCreated"))
				Expect(evt).To(ContainSubstring("session-uuid-001"))
			})
		})

		// UT-AA-064-002: Submit investigation after session regeneration
		Context("UT-AA-064-002: Submit after session regeneration (ID cleared, Generation preserved)", func() {
			It("should resubmit while preserving the regeneration count", func() {
				analysis := createSessionTestAnalysis()
				// Session was lost: ID cleared but Generation preserved
				analysis.Status.InvestigationSession = &aianalysisv1.InvestigationSession{
					ID:         "", // Cleared after session loss
					Generation: 2,
				}

				mockClient.WithSessionSubmitResponse("session-uuid-regen-001")

				result, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())

				// CRD status: ID populated with new UUID, Generation preserved at 2
				Expect(analysis.Status.InvestigationSession.ID).To(Equal("session-uuid-regen-001"))
				Expect(analysis.Status.InvestigationSession.Generation).To(Equal(int32(2)), "Generation should be preserved")
				Expect(analysis.Status.InvestigationSession.CreatedAt).NotTo(BeNil(), "CreatedAt should be updated")

				// Condition: InvestigationSessionReady=True, Reason=SessionRegenerated
				cond := getCondition(analysis, "InvestigationSessionReady")
				Expect(cond).NotTo(BeNil())
				Expect(string(cond.Status)).To(Equal("True"))
				Expect(cond.Reason).To(Equal("SessionRegenerated"))

				// Should requeue at session poll interval for first poll
				Expect(result.RequeueAfter).To(Equal(handlers.DefaultSessionPollInterval), "Regenerated session should requeue at standard poll interval")
			})
		})
	})

	// ========================================
	// 1.2 Incident Poll Flow
	// ========================================
	Describe("Session Poll Flow", func() {
		// UT-AA-064-003: Poll session -- status "pending"
		// BR-AA-HAPI-064.8: Constant poll interval (not backoff). Polling is normal
		// async behavior, not error recovery. Interval is configurable (default 15s).
		Context("UT-AA-064-003: Poll session -- status pending, controller requeues", func() {
			It("should update LastPolled and PollCount, requeue at constant interval", func() {
				analysis := createSessionTestAnalysis()
				analysis.Status.InvestigationSession = &aianalysisv1.InvestigationSession{
					ID:         "session-poll-001",
					Generation: 0,
					CreatedAt:  &metav1.Time{Time: time.Now().Add(-30 * time.Second)},
				}

				mockClient.WithSessionPollStatus("pending")

				result, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(analysis.Status.InvestigationSession.LastPolled).NotTo(BeNil(), "LastPolled should be updated")
				Expect(analysis.Status.InvestigationSession.PollCount).To(Equal(int32(1)), "PollCount should be incremented to 1")
				// BR-AA-HAPI-064.8: Constant interval, default 15s
				Expect(result.RequeueAfter).To(Equal(handlers.DefaultSessionPollInterval), "First poll should requeue at constant interval (15s)")
			})
		})

		// UT-AA-064-004: Poll session -- status "investigating", same constant interval
		// BR-AA-HAPI-064.8: Constant poll interval regardless of PollCount.
		Context("UT-AA-064-004: Poll session -- investigating, constant interval", func() {
			It("should use same constant interval for second consecutive poll", func() {
				analysis := createSessionTestAnalysis()
				lastPoll := metav1.Now()
				analysis.Status.InvestigationSession = &aianalysisv1.InvestigationSession{
					ID:         "session-poll-002",
					Generation: 0,
					CreatedAt:  &metav1.Time{Time: time.Now().Add(-60 * time.Second)},
					LastPolled: &lastPoll, // Already polled once
					PollCount:  1,         // Matches "already polled once"
				}

				mockClient.WithSessionPollStatus("investigating")

				result, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				// BR-AA-HAPI-064.8: Constant interval, same as first poll
				Expect(result.RequeueAfter).To(Equal(handlers.DefaultSessionPollInterval), "Second poll should use same constant interval (15s)")
			})
		})

		// UT-AA-064-005: Poll session -- status "completed", result fetched
		Context("UT-AA-064-005: Poll completed, result fetched and processed", func() {
			It("should retrieve result and advance to Analyzing phase", func() {
				analysis := createSessionTestAnalysis()
				analysis.Status.InvestigationSession = &aianalysisv1.InvestigationSession{
					ID:         "session-completed-001",
					Generation: 0,
					CreatedAt:  &metav1.Time{Time: time.Now().Add(-120 * time.Second)},
				}

				mockClient.WithSessionPollStatus("completed")
				// Configure GetSessionResult to return a full response with workflow
				mockClient.WithFullResponse(
					"Root cause identified: OOM",
					0.9,
					[]string{},
					"OOM caused by memory leak",
					"high",
					"wf-restart-pod",
					"kubernaut.io/workflows/restart:v1.0.0",
					0.9,
					"Selected for OOM recovery",
					false,
				)

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseAnalyzing), "Should advance to Analyzing phase")
				Expect(analysis.Status.SelectedWorkflow).NotTo(BeNil(), "SelectedWorkflow should be populated from result")
				Expect(analysis.Status.SelectedWorkflow.WorkflowID).To(Equal("wf-restart-pod"))

				// Audit side effect: exactly 1 aiagent.result event
				Expect(auditSpy.resultEvents).To(HaveLen(1), "Should record exactly 1 result audit event")
				Expect(auditSpy.resultEvents[0].investigationTime).To(BeNumerically(">", 0), "Investigation time should be positive")
			})
		})

		// UT-AA-064-006: Poll session -- status "failed"
		Context("UT-AA-064-006: Poll session -- failed, investigation terminates", func() {
			It("should surface HAPI-side failure to operators via CRD status", func() {
				analysis := createSessionTestAnalysis()
				analysis.Status.InvestigationSession = &aianalysisv1.InvestigationSession{
					ID:         "session-failed-001",
					Generation: 0,
					CreatedAt:  &metav1.Time{Time: time.Now().Add(-120 * time.Second)},
				}

				mockClient.PollSessionFunc = func(ctx context.Context, sessionID string) (*hgptclient.SessionStatus, error) {
					return &hgptclient.SessionStatus{
						Status: "failed",
						Error:  "LLM provider error: rate limit exceeded",
					}, nil
				}

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseFailed), "Should transition to Failed")
				Expect(analysis.Status.Message).To(ContainSubstring("LLM provider error"), "Error details should be in Message")
			})
		})

		// UT-AA-064-007: Polling interval is constant across all polls
		// BR-AA-HAPI-064.8: Constant interval -- polling is not error recovery.
		// Every poll uses the same configured interval regardless of PollCount.
		Context("UT-AA-064-007: Polling interval is constant across all polls", func() {
			It("should use the same constant interval for every poll", func() {
				analysis := createSessionTestAnalysis()
				analysis.Status.InvestigationSession = &aianalysisv1.InvestigationSession{
					ID:         "session-constant-001",
					Generation: 0,
					CreatedAt:  &metav1.Time{Time: time.Now().Add(-5 * time.Minute)},
				}

				var intervals []time.Duration
				mockClient.WithSessionPollStatus("investigating")

				// Poll 4 times and record intervals
				for i := 0; i < 4; i++ {
					result, err := handler.Handle(ctx, analysis)
					Expect(err).NotTo(HaveOccurred())
					intervals = append(intervals, result.RequeueAfter)
				}

				// BR-AA-HAPI-064.8: All polls use the same constant interval
				Expect(intervals).To(HaveLen(4))
				for i, interval := range intervals {
					Expect(interval).To(Equal(handlers.DefaultSessionPollInterval),
						"Poll %d should use constant interval (15s)", i+1)
				}
			})
		})
	})

	// ========================================
	// 1.3 Session Lost and Regeneration
	// ========================================
	Describe("Session Lost and Regeneration", func() {
		// UT-AA-064-008: Session lost (404) -- first regeneration
		Context("UT-AA-064-008: Session lost (404) -- first regeneration", func() {
			It("should increment Generation, clear ID, and requeue immediately", func() {
				analysis := createSessionTestAnalysis()
				createdAt := metav1.Now()
				analysis.Status.InvestigationSession = &aianalysisv1.InvestigationSession{
					ID:         "session-lost-001",
					Generation: 0,
					CreatedAt:  &createdAt,
				}

				// PollSession returns 404 (session lost)
				mockClient.WithSessionPollError(&hgptclient.APIError{StatusCode: 404, Message: "Session not found"})

				result, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())

				// CRD status: Generation=1, ID cleared, CreatedAt preserved
				Expect(analysis.Status.InvestigationSession.Generation).To(Equal(int32(1)), "Generation should increment to 1")
				Expect(analysis.Status.InvestigationSession.ID).To(BeEmpty(), "Session ID should be cleared")

				// Condition: InvestigationSessionReady=False, Reason=SessionLost
				cond := getCondition(analysis, "InvestigationSessionReady")
				Expect(cond).NotTo(BeNil())
				Expect(string(cond.Status)).To(Equal("False"))
				Expect(cond.Reason).To(Equal("SessionLost"))

				// Result: immediate resubmit (no delay)
				Expect(result.RequeueAfter).To(Equal(time.Duration(0)), "Should requeue immediately for resubmit")

				// Audit side effect: exactly 1 aiagent.session_lost event
				Expect(auditSpy.sessionLostEvents).To(HaveLen(1), "Should record exactly 1 session_lost audit event")
				Expect(auditSpy.sessionLostEvents[0].generation).To(Equal(int32(1)))

				// DD-EVENT-001: K8s Event -- SessionLost (Warning)
				var evt string
				Eventually(recorder.Events).Should(Receive(&evt))
				Expect(evt).To(ContainSubstring("Warning"))
				Expect(evt).To(ContainSubstring("SessionLost"))
				Expect(evt).To(ContainSubstring("generation 1"))
			})
		})

		// UT-AA-064-009: Session lost -- multiple regenerations under cap
		Context("UT-AA-064-009: Multiple regenerations under cap", func() {
			It("should continue self-healing up to the regeneration limit", func() {
				analysis := createSessionTestAnalysis()
				analysis.Status.InvestigationSession = &aianalysisv1.InvestigationSession{
					ID:         "session-regen-multi",
					Generation: 3,
					CreatedAt:  &metav1.Time{Time: time.Now().Add(-2 * time.Minute)},
				}

				mockClient.WithSessionPollError(&hgptclient.APIError{StatusCode: 404, Message: "Session not found"})

				result, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(analysis.Status.InvestigationSession.Generation).To(Equal(int32(4)), "Generation should increment to 4")
				Expect(analysis.Status.InvestigationSession.ID).To(BeEmpty(), "ID should be cleared")
				Expect(result.RequeueAfter).To(Equal(time.Duration(0)), "Should requeue immediately for resubmit")
				// Phase should NOT be Failed (still under cap)
				Expect(analysis.Status.Phase).NotTo(Equal(aianalysis.PhaseFailed), "Should NOT fail while under regeneration cap")
			})
		})

		// UT-AA-064-010: Regeneration cap exceeded -- investigation fails with escalation
		Context("UT-AA-064-010: Regeneration cap exceeded", func() {
			It("should fail with SessionRegenerationExceeded and emit K8s Warning Event", func() {
				analysis := createSessionTestAnalysis()
				analysis.Status.InvestigationSession = &aianalysisv1.InvestigationSession{
					ID:         "session-cap-exceeded",
					Generation: 4, // One more 404 will make it 5 = cap
					CreatedAt:  &metav1.Time{Time: time.Now().Add(-5 * time.Minute)},
				}

				mockClient.WithSessionPollError(&hgptclient.APIError{StatusCode: 404, Message: "Session not found"})

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())

				// CRD status: Phase=Failed, SubReason=SessionRegenerationExceeded
				Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseFailed), "Should transition to Failed")
				Expect(analysis.Status.SubReason).To(Equal("SessionRegenerationExceeded"), "SubReason should indicate cap exceeded")

				// Condition: InvestigationSessionReady=False, Reason=SessionRegenerationExceeded
				cond := getCondition(analysis, "InvestigationSessionReady")
				Expect(cond).NotTo(BeNil())
				Expect(string(cond.Status)).To(Equal("False"))
				Expect(cond.Reason).To(Equal("SessionRegenerationExceeded"))

				// DD-EVENT-001: K8s Events -- SessionLost (Warning) + SessionRegenerationExceeded (Warning)
				// The handler emits SessionLost first (before checking cap), then SessionRegenerationExceeded
				var evts []string
				for len(recorder.Events) > 0 {
					var e string
					Eventually(recorder.Events).Should(Receive(&e))
					evts = append(evts, e)
				}
				// SessionLost is emitted during handleSessionLost before the cap check
				Expect(evts).To(ContainElement(ContainSubstring("SessionLost")))
				// SessionRegenerationExceeded is emitted when cap is exceeded
				Expect(evts).To(ContainElement(ContainSubstring("SessionRegenerationExceeded")))
				Expect(evts).To(ContainElement(ContainSubstring("Warning")))
			})
		})
	})

	// ========================================
	// 1.4 Error Handling
	// ========================================
	Describe("Session Error Handling", func() {
		// UT-AA-064-011: Submit transient error (503)
		Context("UT-AA-064-011: Submit transient error (503)", func() {
			It("should stay Investigating and retry with backoff", func() {
				analysis := createSessionTestAnalysis()

				mockClient.WithSessionSubmitError(&hgptclient.APIError{StatusCode: 503, Message: "Service Unavailable"})

				result, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseInvestigating), "Phase should stay Investigating")
				Expect(analysis.Status.ConsecutiveFailures).To(BeNumerically(">", 0), "ConsecutiveFailures should be incremented")
				Expect(result.RequeueAfter).To(BeNumerically(">", 0), "Should requeue with exponential backoff")
			})
		})

		// UT-AA-064-012: Submit permanent error (401)
		Context("UT-AA-064-012: Submit permanent error (401)", func() {
			It("should fail immediately with PermanentError", func() {
				analysis := createSessionTestAnalysis()

				mockClient.WithSessionSubmitError(&hgptclient.APIError{StatusCode: 401, Message: "Unauthorized"})

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseFailed), "Should transition to Failed")
				Expect(analysis.Status.SubReason).To(Equal("PermanentError"), "SubReason should indicate permanent error")
			})
		})

		// UT-AA-064-013: GetSessionResult returns 409
		Context("UT-AA-064-013: GetSessionResult returns 409 Conflict", func() {
			It("should re-poll at standard session poll interval (treat as transient)", func() {
				analysis := createSessionTestAnalysis()
				analysis.Status.InvestigationSession = &aianalysisv1.InvestigationSession{
					ID:         "session-409",
					Generation: 0,
					CreatedAt:  &metav1.Time{Time: time.Now().Add(-60 * time.Second)},
				}

				// Poll says completed, but result returns 409
				mockClient.WithSessionPollStatus("completed")
				mockClient.WithSessionResultError(&hgptclient.APIError{StatusCode: 409, Message: "Conflict: result not ready"})

				result, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				// BR-AA-HAPI-064.8: Re-poll at standard constant interval (not backoff)
				Expect(result.RequeueAfter).To(Equal(handlers.DefaultSessionPollInterval), "409 should re-poll at standard session poll interval (15s)")
				// Phase should NOT be Failed
				Expect(analysis.Status.Phase).NotTo(Equal(aianalysis.PhaseFailed), "Should NOT fail on 409 (transient)")
				// PollCount should be incremented for observability
				Expect(analysis.Status.InvestigationSession.PollCount).To(Equal(int32(1)), "PollCount should be incremented after 409 re-poll")
			})
		})
	})

	// ========================================
	// 1.5 Client Configuration Correctness
	// ========================================
	Describe("Client Configuration", func() {
		// UT-AA-064-014: Async client constructor sets 30s timeout
		Context("UT-AA-064-014: Async client sets 30s timeout (not 10m workaround)", func() {
			It("should configure 30s HTTP timeout for short-lived async calls", func() {
				cfg := hgptclient.Config{
					BaseURL: "http://localhost:8080",
					Timeout: 30 * time.Second,
				}

				// BR-AA-HAPI-064.10: Timeout removal - verify config value
				Expect(cfg.Timeout).To(Equal(30 * time.Second), "Async client should use 30s timeout, not 10m workaround")
			})
		})
	})

	// ========================================
	// 1.6 Recovery Submit/Poll Flow (Dedicated)
	// ========================================
	Describe("Recovery Session Flow", func() {
		// UT-AA-064-015: Recovery submit routes to recovery endpoint
		Context("UT-AA-064-015: Recovery submit routes to recovery endpoint", func() {
			It("should call SubmitRecoveryInvestigation, not SubmitInvestigation", func() {
				analysis := createSessionTestAnalysis()
				analysis.Spec.IsRecoveryAttempt = true
				analysis.Spec.RecoveryAttemptNumber = 1
				analysis.Spec.PreviousExecutions = []aianalysisv1.PreviousExecution{
					{
						WorkflowExecutionRef: "we-001",
						OriginalRCA: aianalysisv1.OriginalRCA{
							Summary:    "OOM detected",
							SignalType: "OOMKilled",
							Severity:   "high",
						},
					},
				}

				mockClient.WithSessionSubmitResponse("recovery-session-001")

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())

				// SubmitRecoveryInvestigation should be called, NOT SubmitInvestigation
				Expect(mockClient.SubmitRecoveryCallCount).To(Equal(1), "SubmitRecoveryInvestigation should be called")
				Expect(mockClient.SubmitCallCount).To(Equal(0), "SubmitInvestigation should NOT be called")

				// CRD status: InvestigationSession populated
				Expect(analysis.Status.InvestigationSession).NotTo(BeNil())
				Expect(analysis.Status.InvestigationSession.ID).To(Equal("recovery-session-001"))
			})
		})

		// UT-AA-064-016: Recovery poll completed -- recovery result fetched
		Context("UT-AA-064-016: Recovery poll completed, result fetched and processed", func() {
			It("should process recovery result through the recovery response path", func() {
				analysis := createSessionTestAnalysis()
				analysis.Spec.IsRecoveryAttempt = true
				analysis.Spec.RecoveryAttemptNumber = 1
				analysis.Status.InvestigationSession = &aianalysisv1.InvestigationSession{
					ID:         "recovery-session-completed",
					Generation: 0,
					CreatedAt:  &metav1.Time{Time: time.Now().Add(-120 * time.Second)},
				}

				mockClient.WithSessionPollStatus("completed")
				// Recovery response
				mockClient.WithRecoverySuccessResponse(0.85, "scale-deployment-v1", "kubernaut.io/workflows/scale:v1", 0.85, true)

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				// Phase should advance (either Analyzing or Completed depending on recovery logic)
				Expect(analysis.Status.Phase).NotTo(Equal(aianalysis.PhaseInvestigating), "Should advance past Investigating")

				// Recovery-specific: GetRecoverySessionResult should be called
				Expect(mockClient.GetRecoveryResultCallCount).To(Equal(1), "GetRecoverySessionResult should be called")
				Expect(mockClient.GetResultCallCount).To(Equal(0), "GetSessionResult should NOT be called for recovery")
			})
		})

		// UT-AA-064-017: Recovery session lost -- same regeneration cap
		Context("UT-AA-064-017: Recovery session lost, same regeneration cap applies", func() {
			It("should apply the same regeneration flow as incident sessions", func() {
				analysis := createSessionTestAnalysis()
				analysis.Spec.IsRecoveryAttempt = true
				analysis.Status.InvestigationSession = &aianalysisv1.InvestigationSession{
					ID:         "recovery-session-lost",
					Generation: 0,
					CreatedAt:  &metav1.Time{Time: time.Now().Add(-60 * time.Second)},
				}

				mockClient.WithSessionPollError(&hgptclient.APIError{StatusCode: 404, Message: "Session not found"})

				result, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(analysis.Status.InvestigationSession.Generation).To(Equal(int32(1)), "Generation should increment")
				Expect(analysis.Status.InvestigationSession.ID).To(BeEmpty(), "ID should be cleared")
				Expect(result.RequeueAfter).To(Equal(time.Duration(0)), "Should requeue immediately")
			})
		})
	})
})

// ========================================
// Helper Functions
// ========================================

// getCondition returns the condition with the specified type from the AIAnalysis status
func getCondition(analysis *aianalysisv1.AIAnalysis, conditionType string) *metav1.Condition {
	for i := range analysis.Status.Conditions {
		if analysis.Status.Conditions[i].Type == conditionType {
			return &analysis.Status.Conditions[i]
		}
	}
	return nil
}
