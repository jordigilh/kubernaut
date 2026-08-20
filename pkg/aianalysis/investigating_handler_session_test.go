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

package aianalysis_test

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

	agentsessionv1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/internal/controller/aianalysis"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/metrics"
	"github.com/jordigilh/kubernaut/test/shared/mocks"
)

// ========================================
// DD-AA-KA-001, BR-AA-KA-065: AgentSession CRD Channel Unit Tests
//
// Replaces the retired BR-AA-KA-064 session-based HTTP pull design tests.
// GetOrCreate is naturally idempotent (Get on every reconcile once the
// AgentSession exists), so the old submit-vs-poll distinction these tests
// originally targeted no longer exists as a separate code path -- Handle()
// always calls GetOrCreate once and branches on Status.Phase.
// ========================================

// sessionAuditSpy tracks session-related audit events for validation.
// BR-AUDIT-005: Audit as side-effect validation in unit tests. Shared by
// investigating_handler_identity_test.go, investigating_handler_is_phase_test.go,
// and investigation_timeout_test.go.
type sessionAuditSpy struct {
	mu           sync.Mutex
	submitEvents []sessionSubmitEvent
	resultEvents []sessionResultEvent
	failedEvents []failedAnalysisEvent
	// agentCallEvents tracks generic RecordAIAgentCall invocations.
	agentCallEvents []agentCallEvent
}

type agentCallEvent struct {
	analysis   *aianalysisv1.AIAnalysis
	endpoint   string
	statusCode int
	durationMs int
}

type sessionSubmitEvent struct {
	analysis  *aianalysisv1.AIAnalysis
	sessionID string
}

type sessionResultEvent struct {
	analysis          *aianalysisv1.AIAnalysis
	investigationTime int64
}

func (s *sessionAuditSpy) RecordAIAgentCall(ctx context.Context, analysis *aianalysisv1.AIAnalysis, endpoint string, statusCode int, durationMs int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.agentCallEvents = append(s.agentCallEvents, agentCallEvent{
		analysis: analysis, endpoint: endpoint, statusCode: statusCode, durationMs: durationMs,
	})
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

// ========================================
// Test Suite
// ========================================

var _ = Describe("InvestigatingHandler AgentSession Channel (BR-AA-KA-065)", func() {
	var (
		handler    *handlers.InvestigatingHandler
		mockClient *mocks.MockAgentClient
		auditSpy   *sessionAuditSpy
		recorder   *record.FakeRecorder
		ctx        context.Context
	)

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
					AnalysisTypes: []aianalysisv1.AnalysisType{aianalysisv1.AnalysisTypeInvestigation},
				},
			},
			Status: aianalysisv1.AIAnalysisStatus{
				Phase: aianalysis.PhaseInvestigating,
			},
		}
	}

	BeforeEach(func() {
		ctx = context.Background()
		mockClient = mocks.NewMockAgentClient()
		auditSpy = &sessionAuditSpy{}
		recorder = record.NewFakeRecorder(20)
		testMetrics := metrics.NewMetrics()
		handler = handlers.NewInvestigatingHandler(mockClient, ctrl.Log.WithName("test-session"), testMetrics, auditSpy,
			handlers.WithRecorder(recorder))
	})

	// ========================================
	// AgentSession Creation (first observation)
	// ========================================
	Describe("AgentSession Creation", func() {
		Context("UT-AA-065-001: first reconcile creates the AgentSession", func() {
			It("should call GetOrCreate, record the SessionCreated condition, and requeue at the poll interval", func() {
				analysis := createSessionTestAnalysis()
				Expect(analysis.Status.KASession).To(BeNil())

				mockClient.WithPhase(agentsessionv1.AgentSessionPhaseInvestigating)

				result, err := handler.Handle(ctx, analysis)
				Expect(err).NotTo(HaveOccurred())

				Expect(analysis.Status.KASession).NotTo(BeNil(), "KASession status should be populated after GetOrCreate")
				Expect(analysis.Status.KASession.ID).To(Equal(mockClient.AgentSession.Name))
				Expect(analysis.Status.KASession.CreatedAt).NotTo(BeNil(), "CreatedAt should be set")

				cond := getCondition(analysis)
				Expect(cond).NotTo(BeNil(), "InvestigationSessionReady condition should be set")
				Expect(string(cond.Status)).To(Equal("True"))
				Expect(cond.Reason).To(Equal("SessionCreated"))

				// #2204: backstop requeue is now derived from the
				// investigation's own deadline (session.CreatedAt +
				// DefaultMaxInvestigationDuration, TimesOutAt unset here),
				// not a flat poll interval.
				Expect(result.RequeueAfter).To(BeNumerically("~", handlers.DefaultMaxInvestigationDuration, 5*time.Second),
					"should requeue at the investigation's own deadline (#2204)")

				Expect(auditSpy.submitEvents).To(HaveLen(1), "should record exactly 1 submit audit event")

				var evt string
				Eventually(recorder.Events).Should(Receive(&evt))
				Expect(evt).To(ContainSubstring("Normal"))
				Expect(evt).To(ContainSubstring("SessionCreated"))
			})
		})
	})

	// ========================================
	// AgentSession Phase Transitions
	// ========================================
	Describe("AgentSession Phase Transitions", func() {
		Context("UT-AA-065-002: Completed phase advances to Analyzing", func() {
			It("should process the curated result and advance to Analyzing phase", func() {
				analysis := createSessionTestAnalysis()
				analysis.Status.KASession = &aianalysisv1.KASession{
					ID:        "as-session-completed-001",
					CreatedAt: &metav1.Time{Time: time.Now().Add(-120 * time.Second)},
				}

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
				Expect(analysis.Status.GetRCAResult().SelectedWorkflow).NotTo(BeNil(), "SelectedWorkflow should be populated from result")
				Expect(analysis.Status.GetRCAResult().SelectedWorkflow.WorkflowID).To(Equal("wf-restart-pod"))
				Expect(analysis.Status.KASession.PollCount).To(Equal(int32(1)),
					"PollCount must be incremented even when poll returns completed")
				Expect(analysis.Status.KASession.LastPolled).NotTo(BeNil(),
					"LastPolled must be set on completed poll")

				Expect(auditSpy.resultEvents).To(HaveLen(1), "should record exactly 1 result audit event")
				Expect(auditSpy.resultEvents[0].investigationTime).To(BeNumerically(">", 0), "investigation time should be positive")
			})
		})

		Context("UT-AA-065-003: Failed phase surfaces KA's curated error", func() {
			It("should surface KA-side failure to operators via CRD status", func() {
				analysis := createSessionTestAnalysis()
				analysis.Status.KASession = &aianalysisv1.KASession{
					ID:        "as-session-failed-001",
					CreatedAt: &metav1.Time{Time: time.Now().Add(-120 * time.Second)},
				}

				mockClient.WithFailed("LLM provider error: rate limit exceeded")

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseFailed), "should transition to Failed")
				Expect(analysis.Status.Message).To(ContainSubstring("LLM provider error"), "error details should be in Message")
			})
		})

		Context("UT-AA-065-004: Cancelled phase is terminal (no takeover-resubmit)", func() {
			It("should transition to PhaseFailed with ReasonInteractiveCancelled", func() {
				analysis := createSessionTestAnalysis()
				now := metav1.Now()
				analysis.Status.KASession = &aianalysisv1.KASession{
					ID:        "as-session-cancelled-001",
					CreatedAt: &now,
					PollCount: 2,
				}

				mockClient.WithCancelled()

				result, err := handler.Handle(ctx, analysis)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.RequeueAfter).To(BeZero())

				Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseFailed))
				Expect(analysis.Status.Reason).To(Equal(aianalysisv1.ReasonInteractiveCancelled))
				Expect(analysis.Status.Message).To(ContainSubstring("cancelled"))
				Expect(analysis.Status.CompletedAt).NotTo(BeNil())
			})
		})

		// IT-AA-2088-020 (main port of #2086 Fix 4): Completed via KA's own
		// inactivity timeout, which synthesizes a nil result (Confidence=0,
		// no HumanReviewReason). Must not be misclassified identically to a
		// genuine "no matching workflows" conclusion.
		Context("IT-AA-2088-020: Completed via KA inactivity timeout (nil result)", func() {
			It("should classify as InvestigationInconclusive, not NoMatchingWorkflows (#2088 Fix 4)", func() {
				analysis := createSessionTestAnalysis()
				analysis.Status.KASession = &aianalysisv1.KASession{
					ID:        "as-session-timeout-001",
					CreatedAt: &metav1.Time{Time: time.Now().Add(-600 * time.Second)},
				}

				// #2088: mirrors KA's synthesizeNilResult default branch -- the
				// exact shape produced when a user-driving session's own
				// inactivity timeout fires with no InvestigationResult ever set.
				mockClient.WithResult(&agentsessionv1.AgentSessionResult{
					IncidentID:       "as-session-timeout-001",
					Analysis:         "Investigation completed without result",
					NeedsHumanReview: false,
					Confidence:       0,
					Timestamp:        "2026-04-03T00:00:00Z",
					Warnings:         []string{},
				})

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseFailed))
				Expect(analysis.Status.SubReason).NotTo(Equal("NoMatchingWorkflows"),
					"#2088: a session that timed out with no result must not be reported as a genuine no-match conclusion")
				Expect(analysis.Status.SubReason).To(Equal("InvestigationInconclusive"),
					"#2088: distinguishable classification for a session that completed without producing any result")
				Expect(analysis.Status.Message).NotTo(Equal("No workflow selected for remediation"),
					"#2088: message must reflect the true timeout/no-result cause, not the generic no-match message")
			})
		})
	})

	// ========================================
	// AgentSession Capacity-Exceeded Retry (BR-AI-009, DD-AA-KA-001 amendment)
	//
	// KA tags a dispatch-time capacity rejection (session.ErrMaxInvestigationsReached)
	// with Status.Reason=CapacityExceeded -- a transient, self-resolving
	// backpressure condition, not a genuine investigation failure. AA reuses
	// its existing ErrorClassifier retry machinery (synthetic HTTP-429) with
	// KASession.Generation as the retry-attempt counter.
	// ========================================
	Describe("AgentSession Capacity-Exceeded Retry", func() {
		Context("UT-AA-065-007: within retry budget", func() {
			It("should delete the stale AgentSession, increment Generation, and requeue with backoff without permanently failing", func() {
				analysis := createSessionTestAnalysis()
				analysis.Status.KASession = &aianalysisv1.KASession{
					ID:         "as-session-capacity-001",
					Generation: 0,
					CreatedAt:  &metav1.Time{Time: time.Now().Add(-5 * time.Second)},
				}

				mockClient.WithFailedCapacityExceeded("KA dispatch capacity exceeded")

				result, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(analysis.Status.Phase).NotTo(Equal(aianalysis.PhaseFailed),
					"a within-budget capacity-exceeded retry must not permanently fail the AIAnalysis")
				Expect(mockClient.DeleteForRetryCallCount).To(Equal(1),
					"the stale AgentSession must be deleted so the next reconcile's GetOrCreate falls through to Create")
				Expect(analysis.Status.KASession).NotTo(BeNil())
				Expect(analysis.Status.KASession.Generation).To(Equal(int32(1)),
					"the retry-attempt counter (repurposed KASession.Generation) must be incremented")
				Expect(result.RequeueAfter).To(BeNumerically(">", 0),
					"must requeue with backoff, not busy-loop against KA")
			})
		})

		Context("UT-AA-065-008: retry budget exhausted", func() {
			It("should fall through to today's permanent-fail path, unchanged", func() {
				analysis := createSessionTestAnalysis()
				analysis.Status.KASession = &aianalysisv1.KASession{
					ID:         "as-session-capacity-002",
					Generation: int32(handlers.MaxRetries),
					CreatedAt:  &metav1.Time{Time: time.Now().Add(-5 * time.Second)},
				}

				mockClient.WithFailedCapacityExceeded("KA dispatch capacity exceeded")

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseFailed),
					"once the retry budget is exhausted, a capacity-exceeded failure must permanently fail like any other failure")
				Expect(mockClient.DeleteForRetryCallCount).To(Equal(0),
					"a budget-exhausted failure must not attempt another retry deletion")
				Expect(analysis.Status.Message).To(ContainSubstring("KA dispatch capacity exceeded"),
					"the curated KA error must still be surfaced to operators once the retry budget is exhausted")
			})
		})
	})

	// ========================================
	// GetOrCreate Error Handling
	// ========================================
	Describe("GetOrCreate Error Handling", func() {
		Context("UT-AA-065-005: transient K8s API error retries with backoff", func() {
			It("should stay Investigating and retry with backoff", func() {
				analysis := createSessionTestAnalysis()

				mockClient.WithError(k8sStatusError(503, "etcd request timed out"))

				result, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseInvestigating), "Phase should stay Investigating")
				Expect(analysis.Status.GetInvestigationMetadata().ConsecutiveFailures).To(BeNumerically(">", 0), "ConsecutiveFailures should be incremented")
				Expect(result.RequeueAfter).To(BeNumerically(">", 0), "Should requeue with exponential backoff")
			})
		})

		Context("UT-AA-065-006: permanent K8s API error fails immediately", func() {
			It("should fail immediately with PermanentError", func() {
				analysis := createSessionTestAnalysis()

				mockClient.WithError(k8sStatusError(401, "Unauthorized"))

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseFailed), "should transition to Failed")
				Expect(analysis.Status.SubReason).To(Equal("PermanentError"), "SubReason should indicate permanent error")
			})
		})
	})

	// ========================================
	// Interactive Session -- User Driving (DD-INTERACTIVE-002)
	// ========================================
	Describe("Interactive Session -- User Driving", func() {
		Context("UT-AA-703-001: Interactive AgentSession requeues at the poll interval", func() {
			It("should requeue at the configured session poll interval", func() {
				analysis := createSessionTestAnalysis()
				analysis.Status.KASession = &aianalysisv1.KASession{
					ID:        "as-session-interactive-001",
					CreatedAt: &metav1.Time{Time: time.Now().Add(-60 * time.Second)},
				}

				mockClient.WithPhase(agentsessionv1.AgentSessionPhaseInvestigating).
					WithInteractive("oncall@example.com", nil)

				result, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				// #2204: session created 60s ago against the default 25m
				// cap -- ~24m remains until the deadline.
				Expect(result.RequeueAfter).To(BeNumerically("~", handlers.DefaultMaxInvestigationDuration-60*time.Second, 5*time.Second),
					"should requeue at the investigation's own deadline while a user is driving (#2204)")
			})
		})

		Context("UT-AA-703-002: Interactive AgentSession increments poll tracking", func() {
			It("should increment PollCount and set LastPolled", func() {
				analysis := createSessionTestAnalysis()
				analysis.Status.KASession = &aianalysisv1.KASession{
					ID:        "as-session-interactive-002",
					PollCount: 3,
					CreatedAt: &metav1.Time{Time: time.Now().Add(-120 * time.Second)},
				}

				mockClient.WithPhase(agentsessionv1.AgentSessionPhaseInvestigating).
					WithInteractive("oncall@example.com", nil)

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())
				Expect(analysis.Status.KASession.PollCount).To(Equal(int32(4)), "PollCount should be incremented from 3 to 4")
				Expect(analysis.Status.KASession.LastPolled).NotTo(BeNil(), "LastPolled should be set")
			})
		})

		Context("UT-AA-703-003: Interactive AgentSession emits K8s UserDriving event", func() {
			It("should emit a Normal event with reason UserDriving", func() {
				analysis := createSessionTestAnalysis()
				analysis.Status.KASession = &aianalysisv1.KASession{
					ID:        "as-session-interactive-003",
					CreatedAt: &metav1.Time{Time: time.Now().Add(-30 * time.Second)},
				}

				mockClient.WithPhase(agentsessionv1.AgentSessionPhaseInvestigating).
					WithInteractive("oncall@example.com", nil)

				_, err := handler.Handle(ctx, analysis)

				Expect(err).NotTo(HaveOccurred())

				var foundEvent bool
				close(recorder.Events)
				for event := range recorder.Events {
					if event != "" {
						Expect(event).To(ContainSubstring("UserDriving"))
						foundEvent = true
						break
					}
				}
				Expect(foundEvent).To(BeTrue(), "should emit a UserDriving K8s event")
			})
		})

		Context("UT-AA-703-004: Interactive AgentSession then Completed", func() {
			It("should transition to Analyzing once the AgentSession completes", func() {
				analysis := createSessionTestAnalysis()
				analysis.Status.KASession = &aianalysisv1.KASession{
					ID:        "as-session-interactive-004",
					CreatedAt: &metav1.Time{Time: time.Now().Add(-180 * time.Second)},
				}

				mockClient.WithPhase(agentsessionv1.AgentSessionPhaseInvestigating).
					WithInteractive("oncall@example.com", nil)

				result1, err := handler.Handle(ctx, analysis)
				Expect(err).NotTo(HaveOccurred())
				// #2204: session created 180s ago against the default 25m cap.
				Expect(result1.RequeueAfter).To(BeNumerically("~", handlers.DefaultMaxInvestigationDuration-180*time.Second, 5*time.Second))

				mockClient.WithFullResponse(
					"Root cause: config drift",
					0.85,
					[]string{},
					"Config drift detected",
					"medium",
					"wf-rollback",
					"kubernaut.io/workflows/rollback:v1.0.0",
					0.85,
					"Selected for config drift",
					false,
				)

				_, err = handler.Handle(ctx, analysis)
				Expect(err).NotTo(HaveOccurred())
				Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseAnalyzing),
					"should advance to Analyzing after interactive -> completed")
			})
		})
	})
})

// ========================================
// Helper Functions
// ========================================

// getCondition returns the InvestigationSessionReady condition from the AIAnalysis status.
// Shared by investigating_handler_is_phase_test.go and investigation_timeout_test.go.
func getCondition(analysis *aianalysisv1.AIAnalysis) *metav1.Condition {
	for i := range analysis.Status.Conditions {
		if analysis.Status.Conditions[i].Type == "InvestigationSessionReady" {
			return &analysis.Status.Conditions[i]
		}
	}
	return nil
}
