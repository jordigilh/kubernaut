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

// Business Requirement: ADR-EM-001, DD-EM-003, DD-EM-004, BR-ORCH-031
// Purpose: Characterization tests for EffectivenessAssessmentCreator (CHAR-RO-1532) --
// establishes coverage before complexity-lint decomposition (Wave B, #1532).
package creator_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/prometheus/client_golang/prometheus"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/creator"
	rometrics "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
)

// Note: RunSpecs is invoked once per package by TestClusterIDPropagation in
// clusterid_propagation_test.go; Ginkgo does not support calling RunSpecs
// more than once within the same test binary.
var _ = Describe("EffectivenessAssessmentCreator (CHAR-RO-1532)", func() {
	var (
		ctx       context.Context
		k8sClient client.Client
		scheme    *runtime.Scheme
		m         *rometrics.Metrics
		recorder  *record.FakeRecorder
		rr        *remediationv1.RemediationRequest
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(eav1.AddToScheme(scheme)).To(Succeed())
		reg := prometheus.NewRegistry()
		m = rometrics.NewMetricsWithRegistry(reg)
		recorder = record.NewFakeRecorder(10)

		rr = &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "rr-ea-test",
				Namespace: "kubernaut-system",
				UID:       "rr-ea-test-uid",
			},
			Spec: remediationv1.RemediationRequestSpec{
				SignalFingerprint: "aabbccdd1234567890abcdef1234567890abcdef1234567890abcdef12345678",
				SignalName:        "HighCPU",
				Severity:          "warning",
				SignalType:        "alert",
				TargetType:        "kubernetes",
				TargetResource: remediationv1.ResourceIdentifier{
					Kind:      "Deployment",
					Name:      "api-server",
					Namespace: "prod",
				},
				ClusterID: "prod-east-1",
			},
			Status: remediationv1.RemediationRequestStatus{
				OverallPhase: "Completed",
			},
		}

		k8sClient = fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr).
			WithStatusSubresource(rr).
			Build()
	})

	Describe("NewEffectivenessAssessmentCreator", func() {
		It("panics when metrics is nil (DD-METRICS-001 mandate)", func() {
			Expect(func() {
				creator.NewEffectivenessAssessmentCreator(k8sClient, scheme, nil, recorder, 5*time.Minute)
			}).To(PanicWith(ContainSubstring("DD-METRICS-001")))
		})

		It("constructs successfully with non-nil metrics", func() {
			c := creator.NewEffectivenessAssessmentCreator(k8sClient, scheme, m, recorder, 5*time.Minute)
			Expect(c).NotTo(BeNil())
		})
	})

	Describe("StabilizationWindow", func() {
		It("returns the configured stabilization window duration", func() {
			c := creator.NewEffectivenessAssessmentCreator(k8sClient, scheme, m, recorder, 7*time.Minute)
			Expect(c.StabilizationWindow()).To(Equal(7 * time.Minute))
		})
	})

	Describe("CreateEffectivenessAssessment", func() {
		It("creates an EA CRD with the deterministic name ea-<rr.Name>", func() {
			c := creator.NewEffectivenessAssessmentCreator(k8sClient, scheme, m, recorder, 5*time.Minute)

			name, err := c.CreateEffectivenessAssessment(ctx, rr, nil, nil, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(name).To(Equal("ea-rr-ea-test"))

			ea := &eav1.EffectivenessAssessment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: rr.Namespace}, ea)).To(Succeed())
			Expect(ea.Spec.CorrelationID).To(Equal(rr.Name))
			Expect(ea.Spec.RemediationRequestPhase).To(Equal("Completed"))
			Expect(ea.Spec.Config.StabilizationWindow.Duration).To(Equal(5 * time.Minute))
			Expect(ea.Spec.ClusterID).To(Equal("prod-east-1"))
		})

		It("falls back to RR.Spec.TargetResource for both signal and remediation targets when dualTarget is nil", func() {
			c := creator.NewEffectivenessAssessmentCreator(k8sClient, scheme, m, recorder, 5*time.Minute)

			name, err := c.CreateEffectivenessAssessment(ctx, rr, nil, nil, nil)
			Expect(err).NotTo(HaveOccurred())

			ea := &eav1.EffectivenessAssessment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: rr.Namespace}, ea)).To(Succeed())
			Expect(ea.Spec.SignalTarget.Kind).To(Equal("Deployment"))
			Expect(ea.Spec.SignalTarget.Name).To(Equal("api-server"))
			Expect(ea.Spec.RemediationTarget).To(Equal(ea.Spec.SignalTarget))
		})

		It("uses the DualTarget signal/remediation split when provided (DD-EM-003)", func() {
			c := creator.NewEffectivenessAssessmentCreator(k8sClient, scheme, m, recorder, 5*time.Minute)
			dt := &creator.DualTarget{
				Signal:      eav1.TargetResource{Kind: "Deployment", Name: "api-server", Namespace: "prod"},
				Remediation: eav1.TargetResource{Kind: "HorizontalPodAutoscaler", Name: "api-server-hpa", Namespace: "prod"},
			}

			name, err := c.CreateEffectivenessAssessment(ctx, rr, dt, nil, nil)
			Expect(err).NotTo(HaveOccurred())

			ea := &eav1.EffectivenessAssessment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: rr.Namespace}, ea)).To(Succeed())
			Expect(ea.Spec.SignalTarget.Kind).To(Equal("Deployment"))
			Expect(ea.Spec.RemediationTarget.Kind).To(Equal("HorizontalPodAutoscaler"))
			Expect(ea.Spec.RemediationTarget.Name).To(Equal("api-server-hpa"))
		})

		It("propagates optional HashComputeDelay and AlertCheckDelay (#277)", func() {
			c := creator.NewEffectivenessAssessmentCreator(k8sClient, scheme, m, recorder, 5*time.Minute)
			hashDelay := &metav1.Duration{Duration: 30 * time.Second}
			alertDelay := &metav1.Duration{Duration: 2 * time.Minute}

			name, err := c.CreateEffectivenessAssessment(ctx, rr, nil, hashDelay, alertDelay)
			Expect(err).NotTo(HaveOccurred())

			ea := &eav1.EffectivenessAssessment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: rr.Namespace}, ea)).To(Succeed())
			Expect(ea.Spec.Config.HashComputeDelay).To(Equal(hashDelay))
			Expect(ea.Spec.Config.AlertCheckDelay).To(Equal(alertDelay))
		})

		It("sets an owner reference to the RemediationRequest with blockOwnerDeletion=false (BR-ORCH-031)", func() {
			c := creator.NewEffectivenessAssessmentCreator(k8sClient, scheme, m, recorder, 5*time.Minute)

			name, err := c.CreateEffectivenessAssessment(ctx, rr, nil, nil, nil)
			Expect(err).NotTo(HaveOccurred())

			ea := &eav1.EffectivenessAssessment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: rr.Namespace}, ea)).To(Succeed())
			Expect(ea.OwnerReferences).To(HaveLen(1))
			Expect(ea.OwnerReferences[0].Name).To(Equal(rr.Name))
			Expect(ea.OwnerReferences[0].Controller).NotTo(BeNil())
			Expect(*ea.OwnerReferences[0].Controller).To(BeTrue())
			Expect(ea.OwnerReferences[0].BlockOwnerDeletion).NotTo(BeNil())
			Expect(*ea.OwnerReferences[0].BlockOwnerDeletion).To(BeFalse())
		})

		It("emits a K8s event on creation when a recorder is configured (DD-EVENT-001)", func() {
			c := creator.NewEffectivenessAssessmentCreator(k8sClient, scheme, m, recorder, 5*time.Minute)

			_, err := c.CreateEffectivenessAssessment(ctx, rr, nil, nil, nil)
			Expect(err).NotTo(HaveOccurred())

			Eventually(recorder.Events).Should(Receive(ContainSubstring("Created EffectivenessAssessment")))
		})

		It("does not panic and creates the EA when recorder is nil", func() {
			c := creator.NewEffectivenessAssessmentCreator(k8sClient, scheme, m, nil, 5*time.Minute)

			var name string
			var err error
			Expect(func() {
				name, err = c.CreateEffectivenessAssessment(ctx, rr, nil, nil, nil)
			}).NotTo(Panic())
			Expect(err).NotTo(HaveOccurred())
			Expect(name).NotTo(BeEmpty())
		})

		It("is idempotent: returns the existing EA name without error when already created", func() {
			c := creator.NewEffectivenessAssessmentCreator(k8sClient, scheme, m, recorder, 5*time.Minute)

			name1, err := c.CreateEffectivenessAssessment(ctx, rr, nil, nil, nil)
			Expect(err).NotTo(HaveOccurred())

			name2, err := c.CreateEffectivenessAssessment(ctx, rr, nil, nil, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(name2).To(Equal(name1))
		})

		It("returns an error when RemediationRequest has an empty UID (Gap 2.1 defensive programming)", func() {
			rrNoUID := rr.DeepCopy()
			rrNoUID.Name = "rr-no-uid"
			rrNoUID.UID = ""
			rrNoUID.ResourceVersion = ""

			c := creator.NewEffectivenessAssessmentCreator(k8sClient, scheme, m, recorder, 5*time.Minute)
			_, err := c.CreateEffectivenessAssessment(ctx, rrNoUID, nil, nil, nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("UID is required"))
		})
	})
})
