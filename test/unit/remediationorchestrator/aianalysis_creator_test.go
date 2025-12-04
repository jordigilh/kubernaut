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
// BR-ORCH-025: AIAnalysis Child CRD Creation with Data Pass-Through
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

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/creator"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

var _ = Describe("BR-ORCH-025: AIAnalysis Child CRD Creation", func() {
	var (
		ctx        context.Context
		fakeClient client.Client
		scheme     *runtime.Scheme
		aiCreator  *creator.AIAnalysisCreator
		rr         *remediationv1.RemediationRequest
		sp         *signalprocessingv1.SignalProcessing
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()

		// Register schemes
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(signalprocessingv1.AddToScheme(scheme)).To(Succeed())
		Expect(aianalysisv1.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())

		fakeClient = fake.NewClientBuilder().
			WithScheme(scheme).
			Build()

		aiCreator = creator.NewAIAnalysisCreator(fakeClient, scheme)

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

		// Create SignalProcessing with enrichment results
		sp = &signalprocessingv1.SignalProcessing{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "sp-test-rr",
				Namespace: "default",
			},
			Spec: signalprocessingv1.SignalProcessingSpec{
				SignalFingerprint: rr.Spec.SignalFingerprint,
				SignalName:        rr.Spec.SignalName,
				Severity:          rr.Spec.Severity,
				Environment:       rr.Spec.Environment,
				Priority:          rr.Spec.Priority,
				SignalType:        rr.Spec.SignalType,
				TargetType:        rr.Spec.TargetType,
			},
		Status: signalprocessingv1.SignalProcessingStatus{
			Phase: signalprocessingv1.PhaseCompleted,
			EnrichmentResults: &sharedtypes.EnrichmentResults{
					KubernetesContext: &sharedtypes.KubernetesContext{
						Namespace: "default",
						PodDetails: &sharedtypes.PodDetails{
							Name:  "test-pod",
							Phase: "Running",
						},
					},
				},
			},
		}
		Expect(fakeClient.Create(ctx, sp)).To(Succeed())
	})

	Describe("Create", func() {
		// DescribeTable: Consolidates 7 individual tests into table-driven approach
		// Reference: SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md lines 1246-1306
		DescribeTable("should create AIAnalysis CRD with correct data pass-through",
			func(fieldName string, validateFunc func(*aianalysisv1.AIAnalysis)) {
				name, err := aiCreator.Create(ctx, rr, sp)
				Expect(err).NotTo(HaveOccurred())
				Expect(name).To(Equal("ai-test-rr"))

				ai := &aianalysisv1.AIAnalysis{}
				Expect(fakeClient.Get(ctx, client.ObjectKey{
					Name:      name,
					Namespace: rr.Namespace,
				}, ai)).To(Succeed())

				validateFunc(ai)
			},
			// RemediationRequestRef pass-through (BR-ORCH-025)
			Entry("RemediationRequestRef.Name pass-through",
				"RemediationRequestRef.Name",
				func(ai *aianalysisv1.AIAnalysis) {
					Expect(ai.Spec.RemediationRequestRef.Name).To(Equal("test-rr"))
				}),
			Entry("RemediationRequestRef.Namespace pass-through",
				"RemediationRequestRef.Namespace",
				func(ai *aianalysisv1.AIAnalysis) {
					Expect(ai.Spec.RemediationRequestRef.Namespace).To(Equal("default"))
				}),

			// Signal context from SignalProcessing (BR-ORCH-025)
			Entry("SignalContext.Fingerprint pass-through",
				"SignalContext.Fingerprint",
				func(ai *aianalysisv1.AIAnalysis) {
					Expect(ai.Spec.AnalysisRequest.SignalContext.Fingerprint).To(Equal("a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"))
				}),
			Entry("SignalContext.Severity pass-through",
				"SignalContext.Severity",
				func(ai *aianalysisv1.AIAnalysis) {
					Expect(ai.Spec.AnalysisRequest.SignalContext.Severity).To(Equal("warning"))
				}),
			Entry("SignalContext.Environment pass-through",
				"SignalContext.Environment",
				func(ai *aianalysisv1.AIAnalysis) {
					Expect(ai.Spec.AnalysisRequest.SignalContext.Environment).To(Equal("production"))
				}),
			Entry("SignalContext.BusinessPriority pass-through",
				"SignalContext.BusinessPriority",
				func(ai *aianalysisv1.AIAnalysis) {
					Expect(ai.Spec.AnalysisRequest.SignalContext.BusinessPriority).To(Equal("P1"))
				}),

			// Owner reference for cascade deletion (BR-ORCH-031)
			Entry("owner reference set for cascade deletion (BR-ORCH-031)",
				"OwnerReference",
				func(ai *aianalysisv1.AIAnalysis) {
					Expect(ai.OwnerReferences).To(HaveLen(1))
					Expect(ai.OwnerReferences[0].Name).To(Equal("test-rr"))
					Expect(ai.OwnerReferences[0].Kind).To(Equal("RemediationRequest"))
					Expect(*ai.OwnerReferences[0].Controller).To(BeTrue())
				}),

			// Labels for tracking
			Entry("remediation-request label set",
				"Label:remediation-request",
				func(ai *aianalysisv1.AIAnalysis) {
					Expect(ai.Labels).To(HaveKeyWithValue("kubernaut.ai/remediation-request", "test-rr"))
				}),
			Entry("component label set",
				"Label:component",
				func(ai *aianalysisv1.AIAnalysis) {
					Expect(ai.Labels).To(HaveKeyWithValue("kubernaut.ai/component", "ai-analysis"))
				}),

			// Analysis types (BR-ORCH-025)
			Entry("AnalysisTypes include investigation",
				"AnalysisTypes:investigation",
				func(ai *aianalysisv1.AIAnalysis) {
					Expect(ai.Spec.AnalysisRequest.AnalysisTypes).To(ContainElement("investigation"))
				}),
			Entry("AnalysisTypes include root-cause",
				"AnalysisTypes:root-cause",
				func(ai *aianalysisv1.AIAnalysis) {
					Expect(ai.Spec.AnalysisRequest.AnalysisTypes).To(ContainElement("root-cause"))
				}),
			Entry("AnalysisTypes include workflow-selection",
				"AnalysisTypes:workflow-selection",
				func(ai *aianalysisv1.AIAnalysis) {
					Expect(ai.Spec.AnalysisRequest.AnalysisTypes).To(ContainElement("workflow-selection"))
				}),
		)

		// Idempotency is a distinct business behavior - keep as separate test
		Context("idempotency (BR-ORCH-025)", func() {
			It("should return existing name if AIAnalysis already exists", func() {
				name1, err := aiCreator.Create(ctx, rr, sp)
				Expect(err).NotTo(HaveOccurred())

				name2, err := aiCreator.Create(ctx, rr, sp)
				Expect(err).NotTo(HaveOccurred())
				Expect(name2).To(Equal(name1))

				aiList := &aianalysisv1.AIAnalysisList{}
				Expect(fakeClient.List(ctx, aiList, client.InNamespace(rr.Namespace))).To(Succeed())
				Expect(aiList.Items).To(HaveLen(1))
			})
		})
	})
})
