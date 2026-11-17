package creator_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/creator"
	rometrics "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

var _ = Describe("RemediationRequest child identity during creator reuse", func() {
	It("UT-ORCH-2588-015: refuses to reuse a stale SignalProcessing with the same deterministic name", func() {
		scheme := runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(signalprocessingv1.AddToScheme(scheme)).To(Succeed())
		rr := creatorIdentityTestRequest("rr-sp-reuse", "rr-current-uid")
		stale := &signalprocessingv1.SignalProcessing{
			ObjectMeta: metav1.ObjectMeta{Name: "sp-" + rr.Name, Namespace: rr.Namespace},
			Spec: signalprocessingv1.SignalProcessingSpec{
				RemediationRequestRef: signalprocessingv1.ObjectReference{
					Name: rr.Name, Namespace: rr.Namespace, UID: "rr-old-uid",
				},
			},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(rr, stale).Build()
		spCreator := creator.NewSignalProcessingCreator(c, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

		_, err := spCreator.Create(context.Background(), rr)
		Expect(err).To(MatchError(ContainSubstring("identity mismatch")))
	})

	It("UT-ORCH-2588-016: refuses to reuse a stale AIAnalysis with the same deterministic name", func() {
		scheme := runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(aianalysisv1.AddToScheme(scheme)).To(Succeed())
		rr := creatorIdentityTestRequest("rr-ai-reuse", "rr-current-uid")
		stale := &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{Name: "ai-" + rr.Name, Namespace: rr.Namespace},
			Spec: aianalysisv1.AIAnalysisSpec{
				RemediationRequestRef: corev1.ObjectReference{
					Name: rr.Name, Namespace: rr.Namespace, UID: types.UID("rr-old-uid"),
				},
			},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(rr, stale).Build()
		aiCreator := creator.NewAIAnalysisCreator(c, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

		_, err := aiCreator.Create(context.Background(), rr, &signalprocessingv1.SignalProcessing{})
		Expect(err).To(MatchError(ContainSubstring("identity mismatch")))
	})

	It("UT-ORCH-2588-017: refuses to reuse a stale WorkflowExecution with the same deterministic name", func() {
		scheme := runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(aianalysisv1.AddToScheme(scheme)).To(Succeed())
		Expect(workflowexecutionv1.AddToScheme(scheme)).To(Succeed())
		rr := creatorIdentityTestRequest("rr-wfe-reuse", "rr-current-uid")
		stale := &workflowexecutionv1.WorkflowExecution{
			ObjectMeta: metav1.ObjectMeta{Name: "we-" + rr.Name, Namespace: rr.Namespace},
			Spec: workflowexecutionv1.WorkflowExecutionSpec{
				RemediationRequestRef: corev1.ObjectReference{
					Name: rr.Name, Namespace: rr.Namespace, UID: types.UID("rr-old-uid"),
				},
			},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(rr, stale).Build()
		wfeCreator := creator.NewWorkflowExecutionCreator(c, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

		_, _, err := wfeCreator.CreateWithResult(context.Background(), rr, creatorIdentityTestAIAnalysis())
		Expect(err).To(MatchError(ContainSubstring("identity mismatch")))
	})
})

func creatorIdentityTestRequest(name, uid string) *remediationv1.RemediationRequest {
	return &remediationv1.RemediationRequest{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "system", UID: types.UID(uid)},
	}
}

func creatorIdentityTestAIAnalysis() *aianalysisv1.AIAnalysis {
	return &aianalysisv1.AIAnalysis{
		Status: aianalysisv1.AIAnalysisStatus{
			RCAResult: &aianalysisv1.RCAResult{
				SelectedWorkflow: &aianalysisv1.SelectedWorkflow{
					WorkflowSnapshot: sharedtypes.WorkflowSnapshot{
						WorkflowID:      "workflow-id",
						WorkflowName:    "workflow-name",
						ActionType:      "restart",
						Version:         "1.0.0",
						ExecutionBundle: "oci://registry/workflow:v1",
						ExecutionEngine: "kubernetes-job",
					},
				},
			},
		},
	}
}
