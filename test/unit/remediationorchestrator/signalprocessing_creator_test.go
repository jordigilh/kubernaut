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

// Package remediationorchestrator contains unit tests for the Remediation Orchestrator controller.
// BR-ORCH-025: SignalProcessing Child CRD Creation
// BR-ORCH-031: Cascade Deletion via Owner References
package remediationorchestrator

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/creator"
)

var _ = Describe("BR-ORCH-025: SignalProcessing Child CRD Creation", func() {
	var (
		ctx        context.Context
		fakeClient client.Client
		scheme     *runtime.Scheme
		spCreator  *creator.SignalProcessingCreator
		rr         *remediationv1.RemediationRequest
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()

		// Register schemes
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(signalprocessingv1.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())

		fakeClient = fake.NewClientBuilder().
			WithScheme(scheme).
			Build()

		spCreator = creator.NewSignalProcessingCreator(fakeClient, scheme)

		// Create test RemediationRequest
		rr = &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-rr",
				Namespace: "default",
				UID:       "test-uid-123",
			},
			Spec: remediationv1.RemediationRequestSpec{
				SignalFingerprint: "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
				SignalName:        "HighMemoryUsage",
				Severity:          "warning",
				Environment:       "production",
				Priority:          "P1",
				SignalType:        "prometheus",
				TargetType:        "kubernetes",
				TargetResource: remediationv1.ResourceIdentifier{
					Kind:      "Pod",
					Name:      "test-pod",
					Namespace: "default",
				},
			},
		}
		Expect(fakeClient.Create(ctx, rr)).To(Succeed())
	})

	Describe("Create", func() {
		// DescribeTable: Consolidates 6 individual tests into table-driven approach
		// Reference: SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md lines 1246-1306
		DescribeTable("should create SignalProcessing CRD with correct data pass-through",
			func(fieldName string, validateFunc func(*signalprocessingv1.SignalProcessing)) {
				name, err := spCreator.Create(ctx, rr)
				Expect(err).NotTo(HaveOccurred())
				Expect(name).To(Equal("sp-test-rr"))

				sp := &signalprocessingv1.SignalProcessing{}
				Expect(fakeClient.Get(ctx, client.ObjectKey{
					Name:      name,
					Namespace: rr.Namespace,
				}, sp)).To(Succeed())

				validateFunc(sp)
			},
			// Signal context pass-through (BR-ORCH-025)
			Entry("SignalFingerprint pass-through",
				"SignalFingerprint",
				func(sp *signalprocessingv1.SignalProcessing) {
					Expect(sp.Spec.SignalFingerprint).To(Equal("a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"))
				}),
			Entry("SignalName pass-through",
				"SignalName",
				func(sp *signalprocessingv1.SignalProcessing) {
					Expect(sp.Spec.SignalName).To(Equal("HighMemoryUsage"))
				}),
			Entry("Severity pass-through",
				"Severity",
				func(sp *signalprocessingv1.SignalProcessing) {
					Expect(sp.Spec.Severity).To(Equal("warning"))
				}),
			Entry("Environment pass-through",
				"Environment",
				func(sp *signalprocessingv1.SignalProcessing) {
					Expect(sp.Spec.Environment).To(Equal("production"))
				}),
			Entry("Priority pass-through",
				"Priority",
				func(sp *signalprocessingv1.SignalProcessing) {
					Expect(sp.Spec.Priority).To(Equal("P1"))
				}),
			Entry("SignalType pass-through",
				"SignalType",
				func(sp *signalprocessingv1.SignalProcessing) {
					Expect(sp.Spec.SignalType).To(Equal("prometheus"))
				}),
			Entry("TargetType pass-through",
				"TargetType",
				func(sp *signalprocessingv1.SignalProcessing) {
					Expect(sp.Spec.TargetType).To(Equal("kubernetes"))
				}),

			// TargetResource pass-through (BR-ORCH-025)
			Entry("TargetResource.Kind pass-through",
				"TargetResource.Kind",
				func(sp *signalprocessingv1.SignalProcessing) {
					Expect(sp.Spec.TargetResource.Kind).To(Equal("Pod"))
				}),
			Entry("TargetResource.Name pass-through",
				"TargetResource.Name",
				func(sp *signalprocessingv1.SignalProcessing) {
					Expect(sp.Spec.TargetResource.Name).To(Equal("test-pod"))
				}),
			Entry("TargetResource.Namespace pass-through",
				"TargetResource.Namespace",
				func(sp *signalprocessingv1.SignalProcessing) {
					Expect(sp.Spec.TargetResource.Namespace).To(Equal("default"))
				}),

			// Owner reference for cascade deletion (BR-ORCH-031)
			Entry("owner reference set for cascade deletion (BR-ORCH-031)",
				"OwnerReference",
				func(sp *signalprocessingv1.SignalProcessing) {
					Expect(sp.OwnerReferences).To(HaveLen(1))
					Expect(sp.OwnerReferences[0].Name).To(Equal("test-rr"))
					Expect(sp.OwnerReferences[0].Kind).To(Equal("RemediationRequest"))
					Expect(*sp.OwnerReferences[0].Controller).To(BeTrue())
				}),

			// Labels for tracking
			Entry("remediation-request label set",
				"Label:remediation-request",
				func(sp *signalprocessingv1.SignalProcessing) {
					Expect(sp.Labels).To(HaveKeyWithValue("kubernaut.ai/remediation-request", "test-rr"))
				}),
			Entry("component label set",
				"Label:component",
				func(sp *signalprocessingv1.SignalProcessing) {
					Expect(sp.Labels).To(HaveKeyWithValue("kubernaut.ai/component", "signal-processing"))
				}),

			// RemediationRequestRef for audit trail
			Entry("RemediationRequestRef.Name set",
				"RemediationRequestRef.Name",
				func(sp *signalprocessingv1.SignalProcessing) {
					Expect(sp.Spec.RemediationRequestRef.Name).To(Equal("test-rr"))
				}),
			Entry("RemediationRequestRef.Namespace set",
				"RemediationRequestRef.Namespace",
				func(sp *signalprocessingv1.SignalProcessing) {
					Expect(sp.Spec.RemediationRequestRef.Namespace).To(Equal("default"))
				}),
		)

		// Idempotency is a distinct business behavior - keep as separate test
		Context("idempotency (BR-ORCH-025)", func() {
			It("should return existing name if SignalProcessing already exists", func() {
				// Create first time
				name1, err := spCreator.Create(ctx, rr)
				Expect(err).NotTo(HaveOccurred())

				// Create second time (should be idempotent)
				name2, err := spCreator.Create(ctx, rr)
				Expect(err).NotTo(HaveOccurred())
				Expect(name2).To(Equal(name1))

				// Verify only one SignalProcessing exists
				spList := &signalprocessingv1.SignalProcessingList{}
				Expect(fakeClient.List(ctx, spList, client.InNamespace(rr.Namespace))).To(Succeed())
				Expect(spList.Items).To(HaveLen(1))
			})
		})
	})
})
