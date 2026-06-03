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
					ID:        "session-timeout-001",
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
			handlers.WithSessionMode(),
			handlers.WithRecorder(recorder),
			handlers.WithMaxInvestigationDuration(25*time.Minute),
		)
	})

	Describe("UT-AA-1078-TOUT-001: Investigation timeout transitions to PhaseFailed", func() {
		It("should transition to PhaseFailed when elapsed time exceeds MaxInvestigationDuration", func() {
			analysis := createTimeoutTestAnalysis(time.Now().Add(-30 * time.Minute))
			mockClient.WithSessionPollStatus("investigating")

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
			mockClient.WithSessionPollStatus("investigating")

			_, err := handler.Handle(ctx, analysis)
			Expect(err).NotTo(HaveOccurred())

			Expect(analysis.Status.Message).To(ContainSubstring("25m"),
				"timeout message must include the configured max duration")
		})
	})

	Describe("UT-AA-1078-TOUT-003: Investigation within duration limit continues polling", func() {
		It("should continue polling normally when within MaxInvestigationDuration", func() {
			analysis := createTimeoutTestAnalysis(time.Now().Add(-10 * time.Minute))
			mockClient.WithSessionPollStatus("investigating")

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
				handlers.WithSessionMode(),
				handlers.WithRecorder(recorder),
			)

			// Session created 30 minutes ago (exceeds default 25 minute limit)
			analysis := createTimeoutTestAnalysis(time.Now().Add(-30 * time.Minute))
			mockClient.WithSessionPollStatus("investigating")

			_, err := defaultHandler.Handle(ctx, analysis)
			Expect(err).NotTo(HaveOccurred())

			Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseFailed),
				"default 25-minute timeout must apply when WithMaxInvestigationDuration is not set")
		})
	})

	Describe("UT-AA-1351-TOUT-006: successful poll resets ConsecutiveFailures (AA-CRIT-2)", func() {
		It("should reset ConsecutiveFailures to 0 on successful poll", func() {
			analysis := createTimeoutTestAnalysis(time.Now().Add(-5 * time.Minute))
			analysis.Status.ConsecutiveFailures = 3
			mockClient.WithSessionPollStatus("investigating")

			_, err := handler.Handle(ctx, analysis)
			Expect(err).NotTo(HaveOccurred())

			Expect(analysis.Status.ConsecutiveFailures).To(Equal(int32(0)),
				"successful poll must reset ConsecutiveFailures to prevent transient error accumulation (AA-CRIT-2)")
		})
	})

	Describe("UT-AA-1351-TOUT-005: user_driving respects MaxInvestigationDuration (AA-CRIT-1)", func() {
		It("should transition to PhaseFailed when user_driving exceeds MaxInvestigationDuration", func() {
			// Session created 30 minutes ago (exceeds 25 minute limit)
			analysis := createTimeoutTestAnalysis(time.Now().Add(-30 * time.Minute))
			mockClient.WithSessionPollStatus("user_driving")

			result, err := handler.Handle(ctx, analysis)
			Expect(err).NotTo(HaveOccurred())

			Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseFailed),
				"user_driving sessions must NOT bypass MaxInvestigationDuration (AA-CRIT-1)")
			Expect(analysis.Status.Message).To(ContainSubstring("timed out"),
				"timeout message must explain the failure reason")
			Expect(result.RequeueAfter).To(BeZero(),
				"timed-out analysis should not requeue for polling")
		})

		It("should continue polling user_driving within time limit", func() {
			// Session created 10 minutes ago (within 25 minute limit)
			analysis := createTimeoutTestAnalysis(time.Now().Add(-10 * time.Minute))
			mockClient.WithSessionPollStatus("user_driving")

			result, err := handler.Handle(ctx, analysis)
			Expect(err).NotTo(HaveOccurred())

			Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseInvestigating),
				"user_driving within time limit must continue polling")
			Expect(result.RequeueAfter).To(BeNumerically(">", 0),
				"should requeue for next poll")
		})
	})
})
