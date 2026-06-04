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
