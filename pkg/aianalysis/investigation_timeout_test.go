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
	"fmt"
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

var _ = Describe("AA-Side Investigation Timeout — #1078", func() {
	var (
		handler    *handlers.InvestigatingHandler
		mockClient *mocks.MockAgentClient
		ctx        context.Context
		recorder   *record.FakeRecorder
	)

	createTimeoutTestAnalysis := func(createdAt time.Time) *aianalysisv1.AIAnalysis {
		cat := metav1.NewTime(createdAt)
		return &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-timeout",
				Namespace: "default",
			},
			Spec: aianalysisv1.AIAnalysisSpec{
				RemediationRequestRef: corev1.ObjectReference{
					Kind:      "RemediationRequest",
					Name:      "test-rr",
					Namespace: "default",
				},
				RemediationID: "test-remediation-timeout-001",
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
				KASession: &aianalysisv1.KASession{
					ID:        "as-session-timeout-001",
					CreatedAt: &cat,
					PollCount: 5,
				},
			},
		}
	}

	BeforeEach(func() {
		ctx = context.Background()
		mockClient = mocks.NewMockAgentClient()
		recorder = record.NewFakeRecorder(20)
		testMetrics := metrics.NewMetrics()
		handler = handlers.NewInvestigatingHandler(
			mockClient, ctrl.Log.WithName("test-timeout"), testMetrics, &noopAuditClient{},
			handlers.WithRecorder(recorder),
			handlers.WithMaxInvestigationDuration(25*time.Minute),
		)
	})

	Describe("UT-AA-1078-TOUT-001: Investigation timeout transitions to PhaseFailed", func() {
		It("should transition to PhaseFailed when elapsed time exceeds MaxInvestigationDuration", func() {
			analysis := createTimeoutTestAnalysis(time.Now().Add(-30 * time.Minute))
			mockClient.WithPhase(agentsessionv1.AgentSessionPhaseInvestigating)

			result, err := handler.Handle(ctx, analysis)
			Expect(err).NotTo(HaveOccurred())

			Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseFailed),
				"investigation exceeding MaxInvestigationDuration must transition to PhaseFailed")
			Expect(string(analysis.Status.Reason)).To(Equal("TransientError"))
			Expect(analysis.Status.SubReason).To(Equal("TransientError"))
			Expect(result.RequeueAfter).To(BeZero(), "failed analysis should not requeue for polling")
		})
	})

	Describe("UT-AA-1078-TOUT-002: Timeout message includes configured duration", func() {
		It("should include the configured max duration in the failure message", func() {
			analysis := createTimeoutTestAnalysis(time.Now().Add(-30 * time.Minute))
			mockClient.WithPhase(agentsessionv1.AgentSessionPhaseInvestigating)

			_, err := handler.Handle(ctx, analysis)
			Expect(err).NotTo(HaveOccurred())

			Expect(analysis.Status.Message).To(ContainSubstring("25m"),
				"timeout message must include the configured max duration")
		})
	})

	Describe("UT-AA-1078-TOUT-003: Investigation within duration limit continues polling", func() {
		It("should continue polling normally when within MaxInvestigationDuration", func() {
			analysis := createTimeoutTestAnalysis(time.Now().Add(-10 * time.Minute))
			mockClient.WithPhase(agentsessionv1.AgentSessionPhaseInvestigating)

			result, err := handler.Handle(ctx, analysis)
			Expect(err).NotTo(HaveOccurred())

			Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseInvestigating),
				"investigation within time limit must continue polling")
			Expect(result.RequeueAfter).To(BeNumerically(">", 0),
				"should requeue for next poll")
		})
	})

	Describe("UT-AA-1078-TOUT-004: Default handler uses DefaultMaxInvestigationDuration", func() {
		It("should use DefaultMaxInvestigationDuration when WithMaxInvestigationDuration is not called", func() {
			testMetrics := metrics.NewMetrics()
			defaultHandler := handlers.NewInvestigatingHandler(
				mockClient, ctrl.Log.WithName("test-default"), testMetrics, &noopAuditClient{},
				handlers.WithRecorder(recorder),
			)

			// Session created 30 minutes ago (exceeds default 25 minute limit)
			analysis := createTimeoutTestAnalysis(time.Now().Add(-30 * time.Minute))
			mockClient.WithPhase(agentsessionv1.AgentSessionPhaseInvestigating)

			_, err := defaultHandler.Handle(ctx, analysis)
			Expect(err).NotTo(HaveOccurred())

			Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseFailed),
				"default 25-minute timeout must apply when WithMaxInvestigationDuration is not set")
		})
	})

	Describe("UT-AA-1351-TOUT-006: successful poll resets ConsecutiveFailures (AA-CRIT-2)", func() {
		It("should reset ConsecutiveFailures to 0 on successful poll", func() {
			analysis := createTimeoutTestAnalysis(time.Now().Add(-5 * time.Minute))
			analysis.Status.EnsureInvestigationMetadata().ConsecutiveFailures = 3
			mockClient.WithPhase(agentsessionv1.AgentSessionPhaseInvestigating)

			_, err := handler.Handle(ctx, analysis)
			Expect(err).NotTo(HaveOccurred())

			Expect(analysis.Status.GetInvestigationMetadata().ConsecutiveFailures).To(Equal(int32(0)),
				"successful poll must reset ConsecutiveFailures to prevent transient error accumulation (AA-CRIT-2)")
		})
	})

	Describe("UT-AA-1351-TOUT-005: interactive session respects MaxInvestigationDuration (AA-CRIT-1)", func() {
		It("should transition to PhaseFailed when an interactive session exceeds MaxInvestigationDuration", func() {
			// Session created 30 minutes ago (exceeds 25 minute limit)
			analysis := createTimeoutTestAnalysis(time.Now().Add(-30 * time.Minute))
			mockClient.WithPhase(agentsessionv1.AgentSessionPhaseInvestigating).
				WithInteractive("oncall@example.com", nil)

			result, err := handler.Handle(ctx, analysis)
			Expect(err).NotTo(HaveOccurred())

			Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseFailed),
				"interactive sessions must NOT bypass MaxInvestigationDuration (AA-CRIT-1)")
			Expect(analysis.Status.Message).To(ContainSubstring("timed out"),
				"timeout message must explain the failure reason")
			Expect(result.RequeueAfter).To(BeZero(),
				"timed-out analysis should not requeue for polling")
		})

		It("should continue polling an interactive session within time limit", func() {
			// Session created 10 minutes ago (within 25 minute limit)
			analysis := createTimeoutTestAnalysis(time.Now().Add(-10 * time.Minute))
			mockClient.WithPhase(agentsessionv1.AgentSessionPhaseInvestigating).
				WithInteractive("oncall@example.com", nil)

			result, err := handler.Handle(ctx, analysis)
			Expect(err).NotTo(HaveOccurred())

			Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseInvestigating),
				"interactive session within time limit must continue polling")
			Expect(result.RequeueAfter).To(BeNumerically(">", 0),
				"should requeue for next poll")
		})
	})

	// DD-TIMEOUT-002 / Issue #2176: RO's authoritative Analyzing-phase timeout,
	// propagated as Spec.TimesOutAt, must take precedence over AA's own
	// hardcoded maxInvestigationDuration default, and the existing audit-gap
	// (checkInvestigationTimeout not calling RecordAnalysisFailed) must close.
	Describe("UT-AA-2176-001: Spec.TimesOutAt takes precedence over maxInvestigationDuration", func() {
		It("should fail the analysis when Spec.TimesOutAt has passed, even though session.CreatedAt is well within maxInvestigationDuration", func() {
			// Session created 5 minutes ago -- comfortably within the 25m default --
			// but RO's propagated absolute deadline has already passed.
			analysis := createTimeoutTestAnalysis(time.Now().Add(-5 * time.Minute))
			pastDeadline := metav1.NewTime(time.Now().Add(-1 * time.Minute))
			analysis.Spec.TimesOutAt = &pastDeadline
			mockClient.WithPhase(agentsessionv1.AgentSessionPhaseInvestigating)

			result, err := handler.Handle(ctx, analysis)
			Expect(err).NotTo(HaveOccurred())

			Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseFailed),
				"DD-TIMEOUT-002: an expired Spec.TimesOutAt must fail the analysis regardless of maxInvestigationDuration")
			Expect(result.RequeueAfter).To(BeZero(), "failed analysis should not requeue for polling")
		})

		It("should continue polling when Spec.TimesOutAt has not yet passed, even though session.CreatedAt exceeds maxInvestigationDuration", func() {
			// Session created 30 minutes ago -- exceeds the 25m default -- but
			// RO's propagated absolute deadline is still in the future, so the
			// authoritative RO-driven deadline must win over AA's own default.
			analysis := createTimeoutTestAnalysis(time.Now().Add(-30 * time.Minute))
			futureDeadline := metav1.NewTime(time.Now().Add(1 * time.Hour))
			analysis.Spec.TimesOutAt = &futureDeadline
			mockClient.WithPhase(agentsessionv1.AgentSessionPhaseInvestigating)

			result, err := handler.Handle(ctx, analysis)
			Expect(err).NotTo(HaveOccurred())

			Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseInvestigating),
				"DD-TIMEOUT-002: a future Spec.TimesOutAt must take precedence over the hardcoded maxInvestigationDuration default")
			Expect(result.RequeueAfter).To(BeNumerically(">", 0), "should requeue for next poll")
		})

		It("should fall back to session.CreatedAt+maxInvestigationDuration when Spec.TimesOutAt is nil (back-compat)", func() {
			analysis := createTimeoutTestAnalysis(time.Now().Add(-30 * time.Minute))
			Expect(analysis.Spec.TimesOutAt).To(BeNil())
			mockClient.WithPhase(agentsessionv1.AgentSessionPhaseInvestigating)

			result, err := handler.Handle(ctx, analysis)
			Expect(err).NotTo(HaveOccurred())

			Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseFailed),
				"nil Spec.TimesOutAt must fall back to the existing session.CreatedAt+maxInvestigationDuration behavior")
			Expect(result.RequeueAfter).To(BeZero())
		})
	})

	Describe("UT-AA-2176-002: checkInvestigationTimeout records a failure audit event", func() {
		It("should call RecordAnalysisFailed when the investigation times out (BR-AUDIT-005, AU-2/AU-3)", func() {
			spy := &auditClientSpy{}
			testMetrics := metrics.NewMetrics()
			auditedHandler := handlers.NewInvestigatingHandler(
				mockClient, ctrl.Log.WithName("test-timeout-audit"), testMetrics, spy,
				handlers.WithRecorder(recorder),
				handlers.WithMaxInvestigationDuration(25*time.Minute),
			)
			analysis := createTimeoutTestAnalysis(time.Now().Add(-30 * time.Minute))
			mockClient.WithPhase(agentsessionv1.AgentSessionPhaseInvestigating)

			_, err := auditedHandler.Handle(ctx, analysis)
			Expect(err).NotTo(HaveOccurred())

			Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseFailed))
			Expect(spy.failedAnalysisEvents).To(HaveLen(1),
				"BR-AUDIT-005 Gap #7 (Issue #2176): checkInvestigationTimeout must record a failure audit event, matching every other terminal-failure path in this handler")
			Expect(spy.failedAnalysisEvents[0].analysis.Name).To(Equal(analysis.Name))
			Expect(spy.failedAnalysisEvents[0].err).To(HaveOccurred())
		})

		It("should still fail the analysis (fail-open) when RecordAnalysisFailed itself errors", func() {
			spy := &auditClientSpy{recordAnalysisFailedErr: fmt.Errorf("audit store unavailable")}
			testMetrics := metrics.NewMetrics()
			auditedHandler := handlers.NewInvestigatingHandler(
				mockClient, ctrl.Log.WithName("test-timeout-audit-fail-open"), testMetrics, spy,
				handlers.WithRecorder(recorder),
				handlers.WithMaxInvestigationDuration(25*time.Minute),
			)
			analysis := createTimeoutTestAnalysis(time.Now().Add(-30 * time.Minute))
			mockClient.WithPhase(agentsessionv1.AgentSessionPhaseInvestigating)

			result, err := auditedHandler.Handle(ctx, analysis)

			Expect(err).NotTo(HaveOccurred(),
				"a failing audit write must not surface as a reconcile error (fail-open safety)")
			Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseFailed),
				"the timeout failure itself must still be applied even when its audit record fails to write")
			Expect(result.RequeueAfter).To(BeZero())
			Expect(spy.failedAnalysisEvents).To(HaveLen(1), "the audit attempt must still have been made")
		})
	})

	// #2204: the reconciler previously requeued a still-running investigation
	// at a flat, hardcoded interval (sessionPollInterval) regardless of how
	// far away the investigation's actual deadline was -- e.g. requeuing
	// every 15s (or 2s under some test suites' overrides) for the entire
	// lifetime of a 25-minute investigation, generating a per-process
	// reconcile/API-server-hit volume with no relationship to when a check
	// was actually needed. checkInvestigationTimeout already computes the
	// authoritative deadline (Spec.TimesOutAt, else
	// session.CreatedAt+maxInvestigationDuration) to decide *whether* to
	// fail -- these tests prove the backstop requeue now reuses that same
	// deadline to decide *when* to check next, collapsing the "still
	// running" path to a single precisely-scheduled reconcile at the
	// deadline instead of a periodic drumbeat.
	Describe("UT-AA-2204-001: backstop requeue derives from the investigation deadline, not a flat interval", func() {
		It("requeues close to Spec.TimesOutAt when it is sooner than any flat interval would have implied", func() {
			// Session created 1 minute ago (irrelevant to the deadline
			// computation once TimesOutAt is set) with TimesOutAt only 3s
			// away. A flat-interval requeue would blow past this deadline
			// by many seconds; the deadline-driven requeue must not.
			analysis := createTimeoutTestAnalysis(time.Now().Add(-1 * time.Minute))
			deadline := metav1.NewTime(time.Now().Add(3 * time.Second))
			analysis.Spec.TimesOutAt = &deadline
			mockClient.WithPhase(agentsessionv1.AgentSessionPhaseInvestigating)

			result, err := handler.Handle(ctx, analysis)
			Expect(err).NotTo(HaveOccurred())

			Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseInvestigating),
				"investigation still within its deadline must keep polling, not fail")
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))
			Expect(result.RequeueAfter).To(BeNumerically("<=", 4*time.Second),
				"#2204: requeue must track the imminent TimesOutAt deadline (~3s away), not a flat interval that would overshoot it")
		})

		It("requeues close to the maxInvestigationDuration-derived deadline when Spec.TimesOutAt is nil", func() {
			testMetrics := metrics.NewMetrics()
			shortMaxHandler := handlers.NewInvestigatingHandler(
				mockClient, ctrl.Log.WithName("test-short-max"), testMetrics, &noopAuditClient{},
				handlers.WithRecorder(recorder),
				handlers.WithMaxInvestigationDuration(10*time.Second),
			)
			// Session created 7s ago against a 10s max duration -- ~3s of
			// deadline remains, and Spec.TimesOutAt is nil (back-compat
			// fallback path).
			analysis := createTimeoutTestAnalysis(time.Now().Add(-7 * time.Second))
			Expect(analysis.Spec.TimesOutAt).To(BeNil())
			mockClient.WithPhase(agentsessionv1.AgentSessionPhaseInvestigating)

			result, err := shortMaxHandler.Handle(ctx, analysis)
			Expect(err).NotTo(HaveOccurred())

			Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseInvestigating))
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))
			Expect(result.RequeueAfter).To(BeNumerically("<=", 4*time.Second),
				"#2204: requeue must track the maxInvestigationDuration-derived deadline (~3s remaining), not a flat interval")
		})

		It("applies the same deadline-driven requeue while a user is actively driving an interactive session", func() {
			analysis := createTimeoutTestAnalysis(time.Now().Add(-1 * time.Minute))
			deadline := metav1.NewTime(time.Now().Add(3 * time.Second))
			analysis.Spec.TimesOutAt = &deadline
			mockClient.WithPhase(agentsessionv1.AgentSessionPhaseInvestigating).
				WithInteractive("oncall@example.com", nil)

			result, err := handler.Handle(ctx, analysis)
			Expect(err).NotTo(HaveOccurred())

			Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseInvestigating))
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))
			Expect(result.RequeueAfter).To(BeNumerically("<=", 4*time.Second),
				"AA-CRIT-1: interactive sessions share the same deadline-driven backstop, not a separate flat interval")
		})
	})

	Describe("UT-AA-1351-008: handleSessionPollFailed sets Reason and SubReason (AA-MED-1)", func() {
		It("should set structured failure fields when KA session poll returns failed", func() {
			analysis := createTimeoutTestAnalysis(time.Now().Add(-5 * time.Minute))
			mockClient.WithFailed("LLM provider error: rate limit exceeded")

			result, err := handler.Handle(ctx, analysis)
			Expect(err).NotTo(HaveOccurred())

			Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseFailed))
			Expect(analysis.Status.Reason).To(Equal(aianalysisv1.ReasonAPIError))
			Expect(analysis.Status.SubReason).To(Equal("InvestigationFailed"))
			Expect(analysis.Status.Message).To(ContainSubstring("rate limit"))
			Expect(analysis.Status.CompletedAt).NotTo(BeNil())
			Expect(result.RequeueAfter).To(BeZero(), "terminal failure should not requeue for polling")
		})
	})
})
