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
	"crypto/sha256"
	"encoding/hex"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// E2E-RO-106-001: Proactive Signal Mode Propagation
//
// Business Requirement: BR-SP-106, BR-AI-084
// Architecture: ADR-054 (Proactive Signal Mode Classification)
//
// Tests that RO correctly copies signal mode fields from SP.Status to AA.Spec:
// - SignalMode: "proactive" or "reactive"
// - SignalType: normalized type (e.g., "OOMKilled" from "PredictedOOMKill")
//
// Pattern: Manual SP status update (no SP controller deployed in RO E2E cluster)
// Same pattern as needs_human_review_e2e_test.go and lifecycle_e2e_test.go.

var _ = Describe("E2E-RO-106-001: Proactive Signal Mode Propagation", Label("e2e", "signalmode", "remediationorchestrator"), func() {
	var (
		testNS string
	)

	BeforeEach(func() {
		testNS = createTestNamespace("ro-signalmode-e2e")
	})

	AfterEach(func() {
		deleteTestNamespace(testNS)
	})

	It("should propagate signalMode=proactive from SP.Status to AA.Spec", func() {
		By("1. Creating RemediationRequest with proactive signal")
		rrName := "rr-proactive-" + uuid.New().String()[:13]
		now := metav1.Now()
		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      rrName,
				Namespace: controllerNamespace,
			},
			Spec: remediationv1.RemediationRequestSpec{
				SignalFingerprint: func() string {
					h := sha256.Sum256([]byte(uuid.New().String()))
					return hex.EncodeToString(h[:])
				}(),
				SignalName: "PredictedOOMKill",
				Severity:   "critical",
				SignalType: "PredictedOOMKill",
				TargetType: "kubernetes",
				TargetResource: remediationv1.ResourceIdentifier{
					Kind:      "Deployment",
					Name:      "api-server",
					Namespace: testNS,
				},
				FiringTime:   now,
				ReceivedTime: now,
				Deduplication: sharedtypes.DeduplicationInfo{
					FirstOccurrence: now,
					LastOccurrence:  now,
					OccurrenceCount: 1,
				},
			},
		}
		Expect(k8sClient.Create(ctx, rr)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, rr) })

		By("2. Waiting for RO to create SignalProcessing CRD")
		var sp *signalprocessingv1.SignalProcessing
		Eventually(func() bool {
			spList := &signalprocessingv1.SignalProcessingList{}
			_ = k8sClient.List(ctx, spList, client.InNamespace(controllerNamespace))
			for i := range spList.Items {
				if len(spList.Items[i].OwnerReferences) > 0 &&
					spList.Items[i].OwnerReferences[0].Kind == "RemediationRequest" &&
					spList.Items[i].OwnerReferences[0].Name == rrName {
					sp = &spList.Items[i]
					return true
				}
			}
			return false
		}, timeout, interval).Should(BeTrue(), "SignalProcessing should be created by RO")

		By("3. Manually updating SP status with proactive signal mode (simulating SP controller)")
		sp.Status.Phase = signalprocessingv1.PhaseCompleted
		sp.Status.Severity = "critical"
		// BR-SP-106: Proactive signal mode fields
		sp.Status.SignalMode = "proactive"
		sp.Status.SignalName = "OOMKilled"                   // Normalized from PredictedOOMKill
		sp.Status.SourceSignalName = "PredictedOOMKill"    // Preserved for SOC2 audit trail
		sp.Status.EnvironmentClassification = &signalprocessingv1.EnvironmentClassification{
			Environment:  "production",
			Source:       "namespace-labels",
			ClassifiedAt: metav1.Now(),
		}
		sp.Status.PriorityAssignment = &signalprocessingv1.PriorityAssignment{
			Priority:   "P1",
			Source:     "rego-policy",
			AssignedAt: metav1.Now(),
		}
		Expect(k8sClient.Status().Update(ctx, sp)).To(Succeed())

		By("4. Waiting for RO to create AIAnalysis CRD")
		var analysis *aianalysisv1.AIAnalysis
		Eventually(func() bool {
			analysisList := &aianalysisv1.AIAnalysisList{}
			_ = k8sClient.List(ctx, analysisList, client.InNamespace(controllerNamespace))
			for i := range analysisList.Items {
				if len(analysisList.Items[i].OwnerReferences) > 0 &&
					analysisList.Items[i].OwnerReferences[0].Kind == "RemediationRequest" &&
					analysisList.Items[i].OwnerReferences[0].Name == rrName {
					analysis = &analysisList.Items[i]
					return true
				}
			}
			return false
		}, timeout, interval).Should(BeTrue(), "AIAnalysis should be created by RO")

		By("5. Verifying AIAnalysis has proactive signal mode from SP.Status")
		Expect(analysis.Spec.AnalysisRequest.SignalContext.SignalMode).To(Equal("proactive"),
			"BR-SP-106/ADR-054: AIAnalysis MUST propagate signalMode=proactive from SP.Status")

		By("6. Verifying AIAnalysis has NORMALIZED signal type")
		Expect(analysis.Spec.AnalysisRequest.SignalContext.SignalName).To(Equal("OOMKilled"),
			"BR-SP-106/ADR-054: AIAnalysis MUST use normalized SignalType from SP.Status (not PredictedOOMKill)")

		GinkgoWriter.Println("E2E-RO-106-001: Proactive signal mode propagation validated in Kind cluster")
	})

	It("should propagate signalMode=reactive from SP.Status to AA.Spec for standard signals", func() {
		By("1. Creating RemediationRequest with standard reactive signal")
		rrName := "rr-reactive-" + uuid.New().String()[:13]
		now := metav1.Now()
		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      rrName,
				Namespace: controllerNamespace,
			},
			Spec: remediationv1.RemediationRequestSpec{
				SignalFingerprint: func() string {
					h := sha256.Sum256([]byte(uuid.New().String()))
					return hex.EncodeToString(h[:])
				}(),
				SignalName: "OOMKilled",
				Severity:   "critical",
				SignalType: "OOMKilled",
				TargetType: "kubernetes",
				TargetResource: remediationv1.ResourceIdentifier{
					Kind:      "Deployment",
					Name:      "worker-service",
					Namespace: testNS,
				},
				FiringTime:   now,
				ReceivedTime: now,
				Deduplication: sharedtypes.DeduplicationInfo{
					FirstOccurrence: now,
					LastOccurrence:  now,
					OccurrenceCount: 1,
				},
			},
		}
		Expect(k8sClient.Create(ctx, rr)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, rr) })

		By("2. Waiting for RO to create SignalProcessing CRD")
		var sp *signalprocessingv1.SignalProcessing
		Eventually(func() bool {
			spList := &signalprocessingv1.SignalProcessingList{}
			_ = k8sClient.List(ctx, spList, client.InNamespace(controllerNamespace))
			for i := range spList.Items {
				if len(spList.Items[i].OwnerReferences) > 0 &&
					spList.Items[i].OwnerReferences[0].Kind == "RemediationRequest" &&
					spList.Items[i].OwnerReferences[0].Name == rrName {
					sp = &spList.Items[i]
					return true
				}
			}
			return false
		}, timeout, interval).Should(BeTrue())

		By("3. Manually updating SP status with reactive signal mode")
		sp.Status.Phase = signalprocessingv1.PhaseCompleted
		sp.Status.Severity = "critical"
		sp.Status.SignalMode = "reactive"
		sp.Status.SignalName = "OOMKilled" // Unchanged for reactive
		sp.Status.EnvironmentClassification = &signalprocessingv1.EnvironmentClassification{
			Environment:  "production",
			Source:       "namespace-labels",
			ClassifiedAt: metav1.Now(),
		}
		sp.Status.PriorityAssignment = &signalprocessingv1.PriorityAssignment{
			Priority:   "P1",
			Source:     "rego-policy",
			AssignedAt: metav1.Now(),
		}
		Expect(k8sClient.Status().Update(ctx, sp)).To(Succeed())

		By("4. Waiting for AIAnalysis and verifying reactive signal mode")
		var analysis *aianalysisv1.AIAnalysis
		Eventually(func() bool {
			analysisList := &aianalysisv1.AIAnalysisList{}
			_ = k8sClient.List(ctx, analysisList, client.InNamespace(controllerNamespace))
			for i := range analysisList.Items {
				if len(analysisList.Items[i].OwnerReferences) > 0 &&
					analysisList.Items[i].OwnerReferences[0].Kind == "RemediationRequest" &&
					analysisList.Items[i].OwnerReferences[0].Name == rrName {
					analysis = &analysisList.Items[i]
					return true
				}
			}
			return false
		}, timeout, interval).Should(BeTrue())

		Expect(analysis.Spec.AnalysisRequest.SignalContext.SignalMode).To(Equal("reactive"),
			"Standard signals should have signalMode=reactive in AIAnalysis")
		Expect(analysis.Spec.AnalysisRequest.SignalContext.SignalName).To(Equal("OOMKilled"),
			"Reactive signal type should pass through unchanged")

		GinkgoWriter.Println("E2E-RO-106-001: Reactive signal mode propagation validated in Kind cluster")
	})
})
