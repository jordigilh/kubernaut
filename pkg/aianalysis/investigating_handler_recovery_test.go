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

	agentsessionv1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	aiaudit "github.com/jordigilh/kubernaut/pkg/aianalysis/audit"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/metrics"
	"github.com/jordigilh/kubernaut/test/shared/mocks"
)

// AA-H4 (#1356): The investigating handler's session-based flow must call
// GetOrCreate when an active session exists, regardless of InvestigationTime.
// This test proves Handle() is not short-circuited when KASession.ID is
// already set — the exact scenario where an old idempotency guard would
// have blocked execution. DD-AA-KA-001: GetOrCreate is naturally idempotent
// (Get on every reconcile once the AgentSession exists), replacing the
// retired submit-vs-poll distinction this test originally targeted.
var _ = Describe("AA-H4: Investigating Handler Session Recovery", func() {
	var (
		ctx             context.Context
		mockAgentClient *mocks.MockAgentClient
		handler         *handlers.InvestigatingHandler
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockAgentClient = mocks.NewMockAgentClient().WithPhase(agentsessionv1.AgentSessionPhaseInvestigating)

		mockAuditStore := NewMockAuditStore()
		auditClient := aiaudit.NewAuditClient(mockAuditStore, ctrl.Log.WithName("test-audit"))
		testMetrics := metrics.NewMetrics()

		handler = handlers.NewInvestigatingHandler(
			mockAgentClient,
			ctrl.Log.WithName("test-investigating-h4"),
			testMetrics,
			auditClient,
		)
	})

	It("UT-AA-1356-H4-01: calls GetOrCreate for an active session even when InvestigationTime > 0", func() {
		now := metav1.Now()
		analysis := &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "aa-h4-recovery",
				Namespace: "default",
			},
			Status: aianalysisv1.AIAnalysisStatus{
				Phase: "Investigating",
				InvestigationMetadata: &aianalysisv1.InvestigationMetadata{
					InvestigationTime: 5000,
				},
				KASession: &aianalysisv1.KASession{
					ID:        "active-session-123",
					CreatedAt: &now,
					PollCount: 3,
				},
			},
		}

		result, err := handler.Handle(ctx, analysis)
		Expect(err).ToNot(HaveOccurred())

		// Core assertion: GetOrCreate MUST be called, proving the handler
		// was not short-circuited. The controller-level idempotency guard
		// (phase_handlers.go) runs above Handle(), so Handle() itself always
		// executes the session flow.
		Expect(mockAgentClient.GetCallCount()).To(Equal(1),
			"AA-H4: GetOrCreate must be called when an active session exists")

		// Session flow returns a requeue for continued polling
		Expect(result.RequeueAfter).To(BeNumerically(">", 0),
			"handler should requeue for next poll interval")

		// PollCount should be incremented by the handler
		Expect(analysis.Status.KASession.PollCount).To(Equal(int32(4)),
			"PollCount must increment after successful poll")
	})
})
