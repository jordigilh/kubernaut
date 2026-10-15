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

package controller_test

import (
	"context"
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	prodcontroller "github.com/jordigilh/kubernaut/internal/controller/remediationorchestrator"
	roaudit "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/audit"
	rometrics "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
)

var _ = Describe("DD-KA-2496 RO history producer (BR-KA-016, BR-AUDIT-005, AU-3, CC7.2)", func() {
	It("UT-RO-2496-002: workflow creation emits the selected snapshot name and confidence", func() {
		ctx := log.IntoContext(context.Background(), zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))
		scheme := setupScheme()
		rr := newRemediationRequestWithChildRefs("rr-history-2496", defaultFixture, remediationv1.PhaseAnalyzing, "sp-history-2496", "ai-history-2496", "")
		// Standalone local scope; remote-cluster isolation is proved separately
		// through the fleet and authenticated history integration suites.
		rr.Spec.TargetResource.Name = "test-deployment"
		ai := newAIAnalysisCompleted("ai-history-2496", defaultFixture, rr.Name, 0.94, "5be40179-af71-4139-a630-7fcebe1891ad")
		ai.Status.GetRCAResult().SelectedWorkflow.WorkflowName = "Restore memory"
		ai.Status.GetRCAResult().RootCauseAnalysis.RemediationTarget.APIVersion = "apps/v1"
		sp := newSignalProcessingCompleted("sp-history-2496", rr.Name)
		target := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: defaultFixture, Labels: map[string]string{"kubernaut.ai/managed": "true"}}}
		k8s := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&remediationv1.RemediationRequest{}, &aianalysisv1.AIAnalysis{}, &workflowexecutionv1.WorkflowExecution{}).
			WithIndex(&remediationv1.RemediationRequest{}, "spec.signalFingerprint", func(obj client.Object) []string {
				return []string{obj.(*remediationv1.RemediationRequest).Spec.SignalFingerprint}
			}).
			WithIndex(&workflowexecutionv1.WorkflowExecution{}, "spec.targetResource", func(obj client.Object) []string {
				return []string{obj.(*workflowexecutionv1.WorkflowExecution).Spec.TargetResource}
			}).
			WithObjects(rr, ai, sp, target).Build()
		store := &MockAuditStore{}
		reconciler := prodcontroller.NewReconciler(prodcontroller.ReconcilerDeps{
			Client: k8s, APIReader: k8s, Scheme: scheme, AuditStore: store, Recorder: record.NewFakeRecorder(30),
			Metrics: rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
		})

		_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: rr.Name, Namespace: rr.Namespace}})
		Expect(err).NotTo(HaveOccurred())
		events := store.GetEventsByType(roaudit.EventTypeRemediationWorkflowCreated)
		Expect(events).To(HaveLen(1), "the real phase handler must call the real RO audit emitter")
		raw, err := json.Marshal(events[0].EventData)
		Expect(err).NotTo(HaveOccurred())
		var data map[string]interface{}
		Expect(json.Unmarshal(raw, &data)).To(Succeed())
		Expect(data).To(HaveKeyWithValue("workflow_id", ai.Status.GetRCAResult().SelectedWorkflow.WorkflowID))
		Expect(data).To(HaveKeyWithValue("workflow_name", "Restore memory"))
		Expect(data).To(HaveKeyWithValue("workflow_selection_confidence", 0.94))
		Expect(data).To(HaveKeyWithValue("target_resource", defaultFixture+"/Deployment/test-deployment"))
		Expect(events[0].CorrelationID).To(Equal(rr.Name))
		Expect(string(raw)).NotTo(ContainSubstring("parameters"))
	})
})
