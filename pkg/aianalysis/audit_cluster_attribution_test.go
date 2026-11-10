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
	"errors"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	aiaudit "github.com/jordigilh/kubernaut/pkg/aianalysis/audit"
	api "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type aiAuditStoreSpy struct {
	events []*api.AuditEventRequest
}

func (s *aiAuditStoreSpy) StoreAudit(_ context.Context, event *api.AuditEventRequest) error {
	s.events = append(s.events, event)
	return nil
}

func (*aiAuditStoreSpy) Flush(context.Context) error { return nil }
func (*aiAuditStoreSpy) Close() error                { return nil }

var _ = Describe("BR-AUDIT-005: AIAnalysis fleet audit attribution", func() {
	It("UT-AA-2586-004: policy and decision audit preserve distinct scores including zero (AU-3, CC7.2)", func() {
		store := &aiAuditStoreSpy{}
		client := aiaudit.NewAuditClient(store, logr.Discard())
		zero := 0.0
		analysis := &aianalysisv1.AIAnalysis{
			Spec:   aianalysisv1.AIAnalysisSpec{RemediationRequestRef: corev1.ObjectReference{Name: "rr-2586"}},
			Status: aianalysisv1.AIAnalysisStatus{RCAResult: &aianalysisv1.RCAResult{RCAConfidence: &zero, SelectedWorkflow: &aianalysisv1.SelectedWorkflow{Confidence: .95}}},
		}
		client.RecordRegoEvaluation(context.Background(), analysis, "requires_approval", false, 1, "policy", "hash")
		client.RecordApprovalDecision(context.Background(), analysis, "requires_approval", "policy")
		Expect(store.events).To(HaveLen(2))
		regoEvent := store.events[0].EventData.AIAnalysisRegoEvaluationPayload
		decisionEvent := store.events[1].EventData.AIAnalysisApprovalDecisionPayload
		Expect(regoEvent.RcaConfidence.Value).To(Equal(0.0))
		Expect(regoEvent.RcaConfidence.IsSet()).To(BeTrue())
		Expect(regoEvent.WorkflowConfidence.Value).To(Equal(.95))
		Expect(decisionEvent.RcaConfidence.IsSet()).To(BeTrue())
		Expect(decisionEvent.RcaConfidence.Value).To(Equal(0.0))
		Expect(decisionEvent.WorkflowConfidence.Value).To(Equal(.95))
		Expect(store.events[0].CorrelationID).To(Equal("rr-2586"))
		analysis.Status.RCAResult.RCAConfidence = nil
		client.RecordRegoEvaluation(context.Background(), analysis, "auto_approved", false, 1, "permissive policy", "hash")
		client.RecordApprovalDecision(context.Background(), analysis, "auto_approved", "permissive policy")
		Expect(store.events[2].EventData.AIAnalysisRegoEvaluationPayload.RcaConfidence.IsSet()).To(BeFalse())
		Expect(store.events[3].EventData.AIAnalysisApprovalDecisionPayload.RcaConfidence.IsSet()).To(BeFalse())
		Expect(store.events[3].EventData.AIAnalysisApprovalDecisionPayload.Reason).To(Equal("permissive policy"))
	})
	It("sets authoritative Spec.ClusterID on every target-associated audit event", func() {
		store := &aiAuditStoreSpy{}
		client := aiaudit.NewAuditClient(store, logr.Discard())
		analysis := &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{Name: "analysis-2426", Namespace: "kubernaut"},
			Spec: aianalysisv1.AIAnalysisSpec{
				ClusterID:             "remote-cluster-2426",
				RemediationID:         "rr-2426",
				RemediationRequestRef: corev1.ObjectReference{Name: "rr-2426"},
			},
		}

		client.RecordAnalysisComplete(context.Background(), analysis)
		client.RecordPhaseTransition(context.Background(), analysis, "Pending", "Investigating")
		client.RecordError(context.Background(), analysis, "Investigating", errors.New("analysis failed"))
		client.RecordAIAgentCall(context.Background(), analysis, "holmes", 200, 10)
		client.RecordApprovalDecision(context.Background(), analysis, "auto_approved", "policy")
		client.RecordRegoEvaluation(context.Background(), analysis, "allow", false, 5, "", "")
		client.RecordAIAgentSubmit(context.Background(), analysis, "session-2426")
		client.RecordAIAgentResult(context.Background(), analysis, 20)
		Expect(client.RecordAnalysisFailed(context.Background(), analysis, errors.New("failed"))).To(Succeed())

		Expect(store.events).To(HaveLen(10))
		for _, event := range store.events {
			Expect(event.ClusterID.IsSet()).To(BeTrue(), event.EventType)
			clusterID, ok := event.ClusterID.Get()
			Expect(ok).To(BeTrue(), event.EventType)
			Expect(clusterID).To(Equal("remote-cluster-2426"), event.EventType)
		}

		client.RecordConfigReloaded(context.Background(), "ca_cert", nil)
		Expect(store.events).To(HaveLen(11))
		Expect(store.events[10].ClusterID.IsSet()).To(BeFalse())
	})
})
