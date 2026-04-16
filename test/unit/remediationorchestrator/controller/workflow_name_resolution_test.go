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
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	prodcontroller "github.com/jordigilh/kubernaut/internal/controller/remediationorchestrator"
	rometrics "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/routing"
	"github.com/prometheus/client_golang/prometheus"
)

// Issue #643 v2: WorkflowDisplayName must be resolved from DataStorage,
// not from RemediationWorkflow CRD status. DS is the authoritative source
// and remains available even when the CRD is deleted.
var _ = Describe("Issue #643 v2: Workflow Name Resolution via DataStorage", func() {
	const (
		workflowUUID = "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
		workflowName = "crashloop-rollback-v1"
		actionType   = "RestartPod"
	)

	It("UT-RO-643-001: should resolve workflow UUID to ActionType:WorkflowName from DS", func() {
		ctx := context.Background()
		scheme := setupScheme()

		rr := newRemediationRequestWithChildRefs(
			"test-rr-643", "default", remediationv1.PhaseAnalyzing,
			"sp-test-rr-643", "ai-test-rr-643", "")
		ai := newAIAnalysisCompleted("ai-test-rr-643", "default", "test-rr-643", 0.95, workflowUUID)
		sp := newSignalProcessingCompleted("sp-test-rr-643", "default", "test-rr-643")

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

		reconciler.SetWorkflowResolver(&MockWorkflowResolver{
			Responses: map[string]*routing.WorkflowDisplayInfo{
				workflowUUID: {
					WorkflowName: workflowName,
					ActionType:   actionType,
				},
			},
		})

		_, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: "test-rr-643", Namespace: "default"},
		})
		Expect(err).NotTo(HaveOccurred())

		var updatedRR remediationv1.RemediationRequest
		err = fakeClient.Get(ctx, client.ObjectKey{Name: "test-rr-643", Namespace: "default"}, &updatedRR)
		Expect(err).NotTo(HaveOccurred())

		Expect(updatedRR.Status.WorkflowDisplayName).To(
			Equal(actionType+":"+workflowName),
			"WorkflowDisplayName should be ActionType:WorkflowName from DS")
		Expect(updatedRR.Status.WorkflowDisplayName).NotTo(
			ContainSubstring(workflowUUID),
			"WorkflowDisplayName should NOT contain the raw UUID")
	})

	It("UT-RO-643-002: should fall back to UUID when DS returns no match", func() {
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

		reconciler.SetWorkflowResolver(&MockWorkflowResolver{
			Responses: map[string]*routing.WorkflowDisplayInfo{},
		})

		_, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: "test-rr-643b", Namespace: "default"},
		})
		Expect(err).NotTo(HaveOccurred())

		var updatedRR remediationv1.RemediationRequest
		err = fakeClient.Get(ctx, client.ObjectKey{Name: "test-rr-643b", Namespace: "default"}, &updatedRR)
		Expect(err).NotTo(HaveOccurred())

		Expect(updatedRR.Status.WorkflowDisplayName).To(
			ContainSubstring(workflowUUID),
			"WorkflowDisplayName should fall back to UUID when DS has no match")
	})

	It("UT-RO-643-003: should fall back to UUID when resolver is nil (graceful degradation)", func() {
		ctx := context.Background()
		scheme := setupScheme()

		rr := newRemediationRequestWithChildRefs(
			"test-rr-643c", "default", remediationv1.PhaseAnalyzing,
			"sp-test-rr-643c", "ai-test-rr-643c", "")
		ai := newAIAnalysisCompleted("ai-test-rr-643c", "default", "test-rr-643c", 0.95, workflowUUID)
		sp := newSignalProcessingCompleted("sp-test-rr-643c", "default", "test-rr-643c")

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
		// Do NOT call SetWorkflowResolver — resolver stays nil

		_, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: "test-rr-643c", Namespace: "default"},
		})
		Expect(err).NotTo(HaveOccurred())

		var updatedRR remediationv1.RemediationRequest
		err = fakeClient.Get(ctx, client.ObjectKey{Name: "test-rr-643c", Namespace: "default"}, &updatedRR)
		Expect(err).NotTo(HaveOccurred())

		Expect(updatedRR.Status.WorkflowDisplayName).To(
			ContainSubstring(workflowUUID),
			"WorkflowDisplayName should fall back to UUID when resolver is nil")
	})
})
