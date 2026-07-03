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

package remediationorchestrator

import (
	"fmt"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
)

// completeSPWithClusterClassification transitions SP straight to Completed with
// ClusterClassification set in the SAME status update as the rest of the
// mandatory completion fields (BR-FLEET-003, #1511). Setting ClusterClassification
// in a separate, later Update() call (after updateSPStatus already completed SP)
// races with RO's AIAnalysisCreator: RO creates AIAnalysis as soon as it observes
// Phase=Completed and does not retroactively patch an already-created AIAnalysis,
// so the classification would be silently dropped.
func completeSPWithClusterClassification(namespace, name, cluster string) error {
	sp := &signalprocessingv1.SignalProcessing{}
	if err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, sp); err != nil {
		return err
	}

	now := metav1.Now()
	sp.Status.Phase = signalprocessingv1.PhaseCompleted
	sp.Status.CompletionTime = &now
	sp.Status.Severity = "critical"
	sp.Status.SignalMode = "reactive"
	sp.Status.SignalName = sp.Spec.Signal.Name
	sp.Status.EnvironmentClassification = &signalprocessingv1.EnvironmentClassification{
		Environment:  signalprocessingv1.EnvironmentProduction,
		Source:       "test",
		ClassifiedAt: now,
	}
	sp.Status.PriorityAssignment = &signalprocessingv1.PriorityAssignment{
		Priority:   signalprocessingv1.PriorityP1,
		Source:     "test",
		AssignedAt: now,
	}
	sp.Status.ClusterClassification = cluster

	return k8sClient.Status().Update(ctx, sp)
}

// IT-RO-1511-001: buildSignalContext() propagates ClusterClassification into
// AIAnalysis (BR-FLEET-003, #1511, AU-3). Exercises the real RO reconcile
// loop's AIAnalysisCreator, not a direct unit-level Create() call, so this
// proves the production wiring path (RR -> SP completion -> AIAnalysis
// creation) carries the new field end-to-end.
var _ = Describe("IT-RO-1511-001: RO propagates SP.Status.ClusterClassification into AIAnalysis (BR-FLEET-003)", Label("integration", "fleet"), func() {
	var (
		namespace string
		rrName    string
	)

	BeforeEach(func() {
		namespace = createTestNamespace("ro-cluster")
		rrName = fmt.Sprintf("cluster-%s", uuid.New().String()[:13])
	})

	AfterEach(func() {
		deleteTestNamespace(namespace)
	})

	It("IT-RO-1511-001a: AIAnalysis.Spec.AnalysisRequest.SignalContext.Cluster equals SP.Status.ClusterClassification", func() {
		By("Creating a RemediationRequest")
		createRemediationRequest(namespace, rrName)

		By("Waiting for SP to be created")
		spName := fmt.Sprintf("sp-%s", rrName)
		Eventually(func() error {
			sp := &signalprocessingv1.SignalProcessing{}
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
				Name: spName, Namespace: ROControllerNamespace,
			}, sp)
		}, timeout, interval).Should(Succeed())

		By("Completing SP with a cluster classification set")
		Expect(completeSPWithClusterClassification(ROControllerNamespace, spName, "production")).To(Succeed())

		By("Waiting for AIAnalysis to be created with the propagated cluster classification")
		aiName := fmt.Sprintf("ai-%s", rrName)
		Eventually(func() string {
			ai := &aianalysisv1.AIAnalysis{}
			if err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
				Name: aiName, Namespace: ROControllerNamespace,
			}, ai); err != nil {
				return ""
			}
			return ai.Spec.AnalysisRequest.SignalContext.Cluster
		}, timeout, interval).Should(Equal("production"))
	})

	It("IT-RO-1511-001b: empty SP.Status.ClusterClassification (non-fleet) leaves AIAnalysis.Spec...Cluster empty", func() {
		By("Creating a RemediationRequest")
		createRemediationRequest(namespace, rrName)

		By("Waiting for SP to be created")
		spName := fmt.Sprintf("sp-%s", rrName)
		Eventually(func() error {
			sp := &signalprocessingv1.SignalProcessing{}
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
				Name: spName, Namespace: ROControllerNamespace,
			}, sp)
		}, timeout, interval).Should(Succeed())

		By("Completing SP without any cluster classification (non-fleet deployment)")
		Expect(updateSPStatus(ROControllerNamespace, spName, signalprocessingv1.PhaseCompleted)).To(Succeed())

		By("Waiting for AIAnalysis to be created with an empty cluster classification")
		aiName := fmt.Sprintf("ai-%s", rrName)
		var ai *aianalysisv1.AIAnalysis
		Eventually(func() error {
			ai = &aianalysisv1.AIAnalysis{}
			return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
				Name: aiName, Namespace: ROControllerNamespace,
			}, ai)
		}, timeout, interval).Should(Succeed())
		Expect(ai.Spec.AnalysisRequest.SignalContext.Cluster).To(BeEmpty(),
			"non-fleet deployments must not populate Cluster (backward compatibility)")
	})
})
