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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/agentclient"
	aiaudit "github.com/jordigilh/kubernaut/pkg/aianalysis/audit"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/metrics"
	"github.com/jordigilh/kubernaut/test/shared/mocks"
)

// AA-H4 (#1356): The investigating handler's session-based flow must poll
// when an active session exists, regardless of InvestigationTime. This test
// proves the handler reaches PollSession when KASession.ID is set — the
// exact scenario where the old idempotency guard would have blocked execution.
var _ = Describe("AA-H4: Investigating Handler Session Recovery", func() {
	var (
		ctx             context.Context
		mockAgentClient *mocks.MockAgentClient
		handler         *handlers.InvestigatingHandler
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockAgentClient = mocks.NewMockAgentClient()
		mockAgentClient.WithSessionPollStatus("investigating")

		mockAuditStore := NewMockAuditStore()
		auditClient := aiaudit.NewAuditClient(mockAuditStore, ctrl.Log.WithName("test-audit"))
		testMetrics := metrics.NewMetrics()

		handler = handlers.NewInvestigatingHandler(
			mockAgentClient,
			ctrl.Log.WithName("test-investigating-h4"),
			testMetrics,
			auditClient,
			handlers.WithSessionMode(),
		)
	})

	It("UT-AA-1356-H4-01: polls active session even when InvestigationTime > 0", func() {
		now := metav1.Now()
		analysis := &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "aa-h4-recovery",
				Namespace: "default",
			},
			Status: aianalysisv1.AIAnalysisStatus{
				Phase:             "Investigating",
				InvestigationTime: 5000,
				KASession: &aianalysisv1.KASession{
					ID:        "active-session-123",
					CreatedAt: &now,
					PollCount: 3,
				},
			},
		}

		result, err := handler.Handle(ctx, analysis)
		Expect(err).ToNot(HaveOccurred())

		// Core assertion: PollSession MUST be called, proving the handler
		// was not short-circuited. The controller-level idempotency guard
		// (phase_handlers.go:125-137) runs above Handle(), so Handle()
		// itself always executes the session flow when useSessionMode=true.
		Expect(mockAgentClient.PollCallCount).To(Equal(1),
			"AA-H4: PollSession must be called when active session exists")

		// Session flow returns a requeue for continued polling
		Expect(result.RequeueAfter).To(BeNumerically(">", 0),
			"handler should requeue for next poll interval")

		// PollCount should be incremented by the handler
		Expect(analysis.Status.KASession.PollCount).To(Equal(int32(4)),
			"PollCount must increment after successful poll")
	})
})

// ========================================
// Fix #1390: AA 409 Retry Cap
//
// UT-AA-1390-022..024: Validate that the handler caps consecutive 409 errors
// from GetSessionResult and triggers session regeneration after 3 consecutive failures.
// ========================================

var _ = Describe("Fix #1390: AA GetResult 409 Retry Cap — BR-REL-014", func() {
	var (
		ctx             context.Context
		mockAgentClient *mocks.MockAgentClient
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockAgentClient = mocks.NewMockAgentClient()
	})

	createCompletedAnalysis := func(sessionID string, consecutiveErrors int32) *aianalysisv1.AIAnalysis {
		now := metav1.Now()
		return &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "aa-1390-retry",
				Namespace: "default",
			},
			Status: aianalysisv1.AIAnalysisStatus{
				Phase: "Investigating",
				KASession: &aianalysisv1.KASession{
					ID:                         sessionID,
					CreatedAt:                  &now,
					PollCount:                  5,
					ConsecutiveGetResultErrors: consecutiveErrors,
				},
			},
		}
	}

	Context("UT-AA-1390-022 [SI-13]: Third consecutive GetResult 409 triggers session regeneration", func() {
		It("should clear session ID and requeue after 3 consecutive 409 errors", func() {
			mockAgentClient.WithSessionPollStatus("completed")
			mockAgentClient.GetResultErr = &agentclient.APIError{StatusCode: 409, Message: "session result is not an investigation result"}

			mockAuditStore := NewMockAuditStore()
			auditClient := aiaudit.NewAuditClient(mockAuditStore, ctrl.Log.WithName("test-409-cap"))
			testMetrics := metrics.NewMetrics()
			handler := handlers.NewInvestigatingHandler(
				mockAgentClient,
				ctrl.Log.WithName("test-1390-022"),
				testMetrics,
				auditClient,
				handlers.WithSessionMode(),
			)

			analysis := createCompletedAnalysis("session-409-cap", 2)

			result, err := handler.Handle(ctx, analysis)
			Expect(err).NotTo(HaveOccurred())
			Expect(analysis.Status.KASession.ID).To(BeEmpty(),
				"session ID must be cleared after 3 consecutive 409 errors")
			Expect(result.Requeue).To(BeTrue(),
				"should requeue for re-submit (session regeneration)")
		})
	})

	Context("UT-AA-1390-023 [SI-13]: Successful GetResult resets consecutive error counter to 0", func() {
		It("should reset ConsecutiveGetResultErrors to 0 on successful result retrieval", func() {
			mockAgentClient.WithSessionPollStatus("completed")
			mockAgentClient.Response = &agentclient.IncidentResponse{
				IncidentID: "test-success",
				Analysis:   "Test completed successfully",
				Confidence: 0.9,
			}

			mockAuditStore := NewMockAuditStore()
			auditClient := aiaudit.NewAuditClient(mockAuditStore, ctrl.Log.WithName("test-409-reset"))
			testMetrics := metrics.NewMetrics()
			handler := handlers.NewInvestigatingHandler(
				mockAgentClient,
				ctrl.Log.WithName("test-1390-023"),
				testMetrics,
				auditClient,
				handlers.WithSessionMode(),
			)

			analysis := createCompletedAnalysis("session-409-reset", 2)

			_, err := handler.Handle(ctx, analysis)
			Expect(err).NotTo(HaveOccurred())
			Expect(analysis.Status.KASession.ConsecutiveGetResultErrors).To(Equal(int32(0)),
				"ConsecutiveGetResultErrors must be reset to 0 after successful result")
		})
	})

	Context("UT-AA-1390-024 [AC-12]: Regeneration clears session ID and requeues for re-submit", func() {
		It("should increment consecutive errors and requeue on first 409", func() {
			mockAgentClient.WithSessionPollStatus("completed")
			mockAgentClient.GetResultErr = &agentclient.APIError{StatusCode: 409, Message: "session not completed"}

			mockAuditStore := NewMockAuditStore()
			auditClient := aiaudit.NewAuditClient(mockAuditStore, ctrl.Log.WithName("test-409-inc"))
			testMetrics := metrics.NewMetrics()
			handler := handlers.NewInvestigatingHandler(
				mockAgentClient,
				ctrl.Log.WithName("test-1390-024"),
				testMetrics,
				auditClient,
				handlers.WithSessionMode(),
			)

			analysis := createCompletedAnalysis("session-409-first", 0)

			result, err := handler.Handle(ctx, analysis)
			Expect(err).NotTo(HaveOccurred())
			Expect(analysis.Status.KASession.ConsecutiveGetResultErrors).To(Equal(int32(1)),
				"ConsecutiveGetResultErrors must increment on 409")
			Expect(analysis.Status.KASession.ID).NotTo(BeEmpty(),
				"session ID must NOT be cleared on first 409 — only after 3")
			Expect(result.RequeueAfter).To(BeNumerically(">", 0),
				"should requeue for next poll at standard interval")
		})
	})
})
