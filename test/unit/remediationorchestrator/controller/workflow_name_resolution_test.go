/*
Copyright 2025 Jordi Gil.

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

package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	remediationworkflowv1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	prodcontroller "github.com/jordigilh/kubernaut/internal/controller/remediationorchestrator"
	rometrics "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

// Issue #643 regression: resolveWorkflowName must resolve UUID → CRD name.
// The root cause was a missing remediationworkflowv1.AddToScheme(scheme) in
// cmd/remediationorchestrator/main.go, which caused client.List to silently
// return empty results and fall back to the UUID.
var _ = Describe("Issue #643: Workflow Name Resolution", func() {
	const (
		workflowUUID    = "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
		workflowCRDName = "crashloop-rollback-v1"
	)

	It("UT-RO-643-001: should resolve workflow UUID to CRD name when RemediationWorkflow exists", func() {
		ctx := context.Background()
		scheme := setupScheme()

		rw := &remediationworkflowv1.RemediationWorkflow{
			ObjectMeta: metav1.ObjectMeta{
				Name:      workflowCRDName,
				Namespace: "default",
			},
			Status: remediationworkflowv1.RemediationWorkflowStatus{
				WorkflowID: workflowUUID,
			},
		}

		rr := newRemediationRequestWithChildRefs(
			"test-rr-643", "default", remediationv1.PhaseAnalyzing,
			"sp-test-rr-643", "ai-test-rr-643", "")
		ai := newAIAnalysisCompleted("ai-test-rr-643", "default", "test-rr-643", 0.95, workflowUUID)
		sp := newSignalProcessingCompleted("sp-test-rr-643", "default", "test-rr-643")

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr, ai, sp, rw).
			WithStatusSubresource(&remediationv1.RemediationRequest{}).
			Build()

		recorder := record.NewFakeRecorder(20)
		reconciler := prodcontroller.NewReconciler(
			fakeClient, fakeClient, scheme,
			nil, recorder,
			rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
			prodcontroller.TimeoutConfig{}, &MockRoutingEngine{})

		_, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: "test-rr-643", Namespace: "default"},
		})
		Expect(err).NotTo(HaveOccurred())

		var updatedRR remediationv1.RemediationRequest
		err = fakeClient.Get(ctx, client.ObjectKey{Name: "test-rr-643", Namespace: "default"}, &updatedRR)
		Expect(err).NotTo(HaveOccurred())

		Expect(updatedRR.Status.WorkflowDisplayName).To(
			ContainSubstring(workflowCRDName),
			"WorkflowDisplayName should contain the CRD name, not the UUID")
		Expect(updatedRR.Status.WorkflowDisplayName).NotTo(
			ContainSubstring(workflowUUID),
			"WorkflowDisplayName should NOT contain the raw UUID")
	})

	It("UT-RO-643-002: should fall back to UUID when no matching RemediationWorkflow exists", func() {
		ctx := context.Background()
		scheme := setupScheme()

		rr := newRemediationRequestWithChildRefs(
			"test-rr-643b", "default", remediationv1.PhaseAnalyzing,
			"sp-test-rr-643b", "ai-test-rr-643b", "")
		ai := newAIAnalysisCompleted("ai-test-rr-643b", "default", "test-rr-643b", 0.95, workflowUUID)
		sp := newSignalProcessingCompleted("sp-test-rr-643b", "default", "test-rr-643b")

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr, ai, sp).
			WithStatusSubresource(&remediationv1.RemediationRequest{}).
			Build()

		recorder := record.NewFakeRecorder(20)
		reconciler := prodcontroller.NewReconciler(
			fakeClient, fakeClient, scheme,
			nil, recorder,
			rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
			prodcontroller.TimeoutConfig{}, &MockRoutingEngine{})

		_, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: "test-rr-643b", Namespace: "default"},
		})
		Expect(err).NotTo(HaveOccurred())

		var updatedRR remediationv1.RemediationRequest
		err = fakeClient.Get(ctx, client.ObjectKey{Name: "test-rr-643b", Namespace: "default"}, &updatedRR)
		Expect(err).NotTo(HaveOccurred())

		Expect(updatedRR.Status.WorkflowDisplayName).To(
			ContainSubstring(workflowUUID),
			"WorkflowDisplayName should fall back to UUID when no CRD match exists")
	})

	It("UT-RO-643-003: scheme must include RemediationWorkflow types", func() {
		scheme := setupScheme()
		gvk := remediationworkflowv1.GroupVersion.WithKind("RemediationWorkflow")
		recognized := scheme.Recognizes(gvk)
		Expect(recognized).To(BeTrue(),
			"Test scheme must recognize RemediationWorkflow GVK — mirrors cmd/remediationorchestrator/main.go init()")
	})
})
