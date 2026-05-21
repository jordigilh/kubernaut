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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/internal/controller/aianalysis"
	"github.com/jordigilh/kubernaut/pkg/agentclient"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/metrics"
	"github.com/jordigilh/kubernaut/test/shared/mocks"
)

var _ = Describe("InvestigatingHandler Identity Propagation — #774, BR-INTERACTIVE-001", func() {

	var (
		handler    *handlers.InvestigatingHandler
		mockClient *mocks.MockAgentClient
		auditSpy   *sessionAuditSpy
		recorder   *record.FakeRecorder
		ctx        context.Context
	)

	createIdentityTestAnalysis := func() *aianalysisv1.AIAnalysis {
		return &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-identity-analysis",
				Namespace: "default",
			},
			Spec: aianalysisv1.AIAnalysisSpec{
				RemediationRequestRef: corev1.ObjectReference{
					Kind:      "RemediationRequest",
					Name:      "test-rr",
					Namespace: "default",
				},
				RemediationID: "test-remediation-774",
				AnalysisRequest: aianalysisv1.AnalysisRequest{
					SignalContext: aianalysisv1.SignalContextInput{
						Fingerprint:      "test-fingerprint-774",
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
					ID:        "session-774-001",
					PollCount: 3,
				},
			},
		}
	}

	BeforeEach(func() {
		ctx = context.Background()
		mockClient = mocks.NewMockAgentClient()
		auditSpy = &sessionAuditSpy{}
		recorder = record.NewFakeRecorder(20)
		testMetrics := metrics.NewMetrics()
		handler = handlers.NewInvestigatingHandler(mockClient, ctrl.Log.WithName("test-identity"), testMetrics, auditSpy,
			handlers.WithSessionMode(), handlers.WithRecorder(recorder))
	})

	Describe("UT-KA-774-007: handleSessionPollUserDriving copies identity from poll result to CR status", func() {
		It("should write ActingUser and ActingUserGroups to InteractiveSession on the CR", func() {
			analysis := createIdentityTestAnalysis()

			mockClient.PollSessionFunc = func(_ context.Context, _ string) (*agentclient.SessionStatusResult, error) {
				return &agentclient.SessionStatusResult{
					Status:           "user_driving",
					ActingUser:       "jane@example.com",
					ActingUserGroups: []string{"sre-team", "oncall"},
				}, nil
			}

			result, err := handler.Handle(ctx, analysis)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0),
				"user_driving should requeue for continued polling")

			Expect(analysis.Status.InteractiveSession).NotTo(BeNil(),
				"InteractiveSession should be populated by handleSessionPollUserDriving")
			Expect(analysis.Status.InteractiveSession.ActingUser).To(Equal("jane@example.com"),
				"ActingUser should be copied from poll result")
			Expect(analysis.Status.InteractiveSession.ActingUserGroups).To(Equal([]string{"sre-team", "oncall"}),
				"ActingUserGroups should be copied from poll result")
		})
	})

	Describe("UT-KA-774-008: handleSessionPollUserDriving with no identity in poll result", func() {
		It("should not create InteractiveSession when poll result has no identity", func() {
			analysis := createIdentityTestAnalysis()

			mockClient.PollSessionFunc = func(_ context.Context, _ string) (*agentclient.SessionStatusResult, error) {
				return &agentclient.SessionStatusResult{
					Status: "user_driving",
				}, nil
			}

			result, err := handler.Handle(ctx, analysis)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))

			Expect(analysis.Status.InteractiveSession).To(BeNil(),
				"InteractiveSession should remain nil when no identity provided")
		})
	})
})
