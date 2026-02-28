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
	"fmt"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// ============================================================================
// SEVERITY NORMALIZATION INTEGRATION TESTS
// Authority: DD-SEVERITY-001 v1.1 (Severity Determination Refactoring)
// ============================================================================
//
// These tests verify RemediationOrchestrator creates AIAnalysis with
// NORMALIZED severity from SignalProcessing.Status.Severity (NOT external
// severity from RemediationRequest.Spec.Severity).
//
// Business Requirements:
// - BR-SP-105: Severity Determination via Rego Policy
// - BR-ORCH-025: Data pass-through to child CRDs
//
// Test Pattern (per TESTING_GUIDELINES.md):
// - Integration tier: Real K8s environment (envtest), no HTTP
// - Tests component coordination: RR → SP → RO → AA
// - Uses Eventually() for async operations (no time.Sleep)
// - Maps to business requirements (DD-SEVERITY-001)
//
// ============================================================================

var _ = Describe("DD-SEVERITY-001: Severity Normalization Integration", Label("integration", "severity"), func() {

	Context("Enterprise Severity Scheme (Sev1-4)", func() {
		var (
			namespace string
			rrName    string
		)

		BeforeEach(func() {
			namespace = createTestNamespace("ro-severity-sev")
			rrName = fmt.Sprintf("rr-sev1-%s", uuid.New().String()[:13])
		})

		AfterEach(func() {
			deleteTestNamespace(namespace)
		})

		It("[RO-INT-SEV-001] should create AIAnalysis with normalized severity from SP.Status (Sev1 → critical)", func() {
			By("1. Create RemediationRequest with external 'Sev1' severity")
			now := metav1.Now()
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      rrName,
					Namespace: ROControllerNamespace,
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: func() string { // UNIQUE per test to avoid routing deduplication (DD-RO-002)
						h := sha256.Sum256([]byte(uuid.New().String()))
						return hex.EncodeToString(h[:])
					}(),
					SignalName: "ProductionOutage",
					Severity:   "Sev1", // External (Enterprise severity scheme)
					SignalType: "alert",
					TargetType: "kubernetes",
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:      "Deployment",
						Name:      "api-server",
						Namespace: namespace,
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

			By("2. Wait for RO to create SignalProcessing")
			spName := fmt.Sprintf("sp-%s", rrName)
			sp := &signalprocessingv1.SignalProcessing{}

			Eventually(func() error {
				return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: spName, Namespace: ROControllerNamespace}, sp)
			}, timeout, interval).Should(Succeed(),
				"RO should create SignalProcessing when RR is created")

			By("3. Verify SP has external severity in Spec")
			Expect(sp.Spec.Signal.Severity).To(Equal("Sev1"),
				"SignalProcessing.Spec should copy external severity from RemediationRequest")

			By("4. Simulate SignalProcessing Rego normalization by updating Status")
			// In real environment, SignalProcessing controller runs Rego policy
			// For integration test, we use helper to consistently set normalized severity
			Expect(updateSPStatus(ROControllerNamespace, spName, signalprocessingv1.PhaseCompleted, "critical")).To(Succeed())

			By("5. Wait for RO to create AIAnalysis")
			var createdAA *aianalysisv1.AIAnalysis
			Eventually(func() bool {
				var aaList aianalysisv1.AIAnalysisList
				err := k8sManager.GetAPIReader().List(ctx, &aaList,
					client.InNamespace(ROControllerNamespace))
				if err != nil {
					return false
				}
				for i := range aaList.Items {
					if aaList.Items[i].Spec.RemediationRequestRef.Name == rrName {
						createdAA = &aaList.Items[i]
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue(),
				"RO should create AIAnalysis when SignalProcessing reaches Completed phase")

			By("6. Verify AIAnalysis has NORMALIZED severity (not external)")
			Expect(createdAA.Spec.AnalysisRequest.SignalContext.Severity).To(Equal("critical"),
				"DD-SEVERITY-001: AIAnalysis MUST use normalized severity from SP.Status.Severity (not external 'Sev1' from RR.Spec.Severity)")

			By("7. Verify RemediationRequest still has external severity")
			var updatedRR remediationv1.RemediationRequest
			Expect(k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: rrName, Namespace: ROControllerNamespace}, &updatedRR)).To(Succeed())
			Expect(updatedRR.Spec.Severity).To(Equal("Sev1"),
				"RemediationRequest should preserve external severity for operator-facing messages")

			GinkgoWriter.Printf("✅ Severity normalization validated: Sev1 (RR.Spec) → critical (SP.Status) → critical (AA.Spec)\n")
		})

		It("[RO-INT-SEV-002] should create AIAnalysis with normalized severity (Sev2 → high)", func() {
			By("1. Create RemediationRequest with external 'Sev2' severity")
			rrName := fmt.Sprintf("rr-sev2-%s", uuid.New().String()[:13])
			now := metav1.Now()
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      rrName,
					Namespace: ROControllerNamespace,
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: "b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3",
					SignalName:        "DegradedService",
					Severity:          "Sev2", // External (Enterprise severity scheme)
					SignalType:        "alert",
					TargetType:        "kubernetes",
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:      "Deployment",
						Name:      "worker-service",
						Namespace: namespace,
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

			By("2. Wait for SignalProcessing and simulate normalization")
			spName := fmt.Sprintf("sp-%s", rrName)
			sp := &signalprocessingv1.SignalProcessing{}
			Eventually(func() error {
				return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: spName, Namespace: ROControllerNamespace}, sp)
			}, timeout, interval).Should(Succeed())

			// Simulate SignalProcessing Rego normalization using helper (DD-SEVERITY-001)
			Expect(updateSPStatus(ROControllerNamespace, spName, signalprocessingv1.PhaseCompleted, "high")).To(Succeed())

			By("3. Wait for AIAnalysis and verify normalized severity")
			var createdAA *aianalysisv1.AIAnalysis
			Eventually(func() bool {
				var aaList aianalysisv1.AIAnalysisList
				err := k8sManager.GetAPIReader().List(ctx, &aaList,
					client.InNamespace(ROControllerNamespace))
				if err != nil {
					return false
				}
				for i := range aaList.Items {
					if aaList.Items[i].Spec.RemediationRequestRef.Name == rrName {
						createdAA = &aaList.Items[i]
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())

			Expect(createdAA.Spec.AnalysisRequest.SignalContext.Severity).To(Equal("high"),
				"DD-SEVERITY-001: AIAnalysis should use normalized 'high' (not external 'Sev2')")

			GinkgoWriter.Printf("✅ Severity normalization validated: Sev2 → high\n")
		})
	})

	Context("PagerDuty Severity Scheme (P0-P4)", func() {
		var (
			namespace string
			rrName    string
		)

		BeforeEach(func() {
			namespace = createTestNamespace("ro-severity-pd")
			rrName = fmt.Sprintf("rr-p0-%s", uuid.New().String()[:13])
		})

		AfterEach(func() {
			deleteTestNamespace(namespace)
		})

		It("[RO-INT-SEV-003] should create AIAnalysis with normalized severity (P0 → critical)", func() {
			By("1. Create RemediationRequest with external 'P0' severity")
			now := metav1.Now()
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      rrName,
					Namespace: ROControllerNamespace,
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: "c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4",
					SignalName:        "DatabaseOutage",
					Severity:          "P0", // External (PagerDuty severity scheme)
					SignalType:        "pagerduty",
					TargetType:        "kubernetes",
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:      "StatefulSet",
						Name:      "postgres",
						Namespace: namespace,
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

			By("2. Wait for SignalProcessing and simulate normalization")
			spName := fmt.Sprintf("sp-%s", rrName)
			sp := &signalprocessingv1.SignalProcessing{}
			Eventually(func() error {
				return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: spName, Namespace: ROControllerNamespace}, sp)
			}, timeout, interval).Should(Succeed())

			Expect(sp.Spec.Signal.Severity).To(Equal("P0"),
				"SignalProcessing should preserve external 'P0' in Spec")

			// Simulate SignalProcessing Rego normalization using helper (DD-SEVERITY-001)
			Expect(updateSPStatus(ROControllerNamespace, spName, signalprocessingv1.PhaseCompleted, "critical")).To(Succeed())

			By("3. Wait for AIAnalysis and verify normalized severity")
			var createdAA *aianalysisv1.AIAnalysis
			Eventually(func() bool {
				var aaList aianalysisv1.AIAnalysisList
				err := k8sManager.GetAPIReader().List(ctx, &aaList,
					client.InNamespace(ROControllerNamespace))
				if err != nil {
					return false
				}
				for i := range aaList.Items {
					if aaList.Items[i].Spec.RemediationRequestRef.Name == rrName {
						createdAA = &aaList.Items[i]
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())

			Expect(createdAA.Spec.AnalysisRequest.SignalContext.Severity).To(Equal("critical"),
				"DD-SEVERITY-001: AIAnalysis should use normalized 'critical' (not external 'P0')")

			GinkgoWriter.Printf("✅ Severity normalization validated: P0 → critical\n")
		})

		It("[RO-INT-SEV-004] should create AIAnalysis with normalized severity (P3 → medium)", func() {
			By("1. Create RemediationRequest with external 'P3' severity")
			rrName := fmt.Sprintf("rr-p3-%s", uuid.New().String()[:13])
			// Generate unique SignalFingerprint to prevent routing collision in parallel tests
			hash := sha256.Sum256([]byte(uuid.New().String()))
			now := metav1.Now()
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      rrName,
					Namespace: ROControllerNamespace,
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: hex.EncodeToString(hash[:]),
					SignalName:        "MinorIssue",
					Severity:          "P3", // External (PagerDuty severity scheme)
					SignalType:        "pagerduty",
					TargetType:        "kubernetes",
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:      "Deployment",
						Name:      "cache-service",
						Namespace: namespace,
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

			By("2. Wait for SignalProcessing and simulate normalization")
			spName := fmt.Sprintf("sp-%s", rrName)
			sp := &signalprocessingv1.SignalProcessing{}
			Eventually(func() error {
				return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: spName, Namespace: ROControllerNamespace}, sp)
			}, timeout, interval).Should(Succeed())

			// Simulate SignalProcessing Rego normalization using helper (DD-SEVERITY-001)
			Expect(updateSPStatus(ROControllerNamespace, spName, signalprocessingv1.PhaseCompleted, "medium")).To(Succeed())

			// RACE FIX: Ensure SignalProcessing status is fully propagated before expecting AIAnalysis
			// In CI's faster environment, the RO controller might not see the SP status update
			// immediately, causing it to delay AIAnalysis creation
			Eventually(func() signalprocessingv1.SignalProcessingPhase {
				err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: spName, Namespace: ROControllerNamespace}, sp)
				if err != nil {
					return ""
				}
				return sp.Status.Phase
			}, timeout, interval).Should(Equal(signalprocessingv1.PhaseCompleted),
				"SignalProcessing status must be Completed before RO creates AIAnalysis")

			By("3. Wait for AIAnalysis and verify normalized severity")
			var createdAA *aianalysisv1.AIAnalysis
			Eventually(func() bool {
				var aaList aianalysisv1.AIAnalysisList
				err := k8sManager.GetAPIReader().List(ctx, &aaList,
					client.InNamespace(ROControllerNamespace))
				if err != nil {
					return false
				}
				for i := range aaList.Items {
					if aaList.Items[i].Spec.RemediationRequestRef.Name == rrName {
						createdAA = &aaList.Items[i]
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())

			Expect(createdAA.Spec.AnalysisRequest.SignalContext.Severity).To(Equal("medium"),
				"DD-SEVERITY-001: AIAnalysis should use normalized 'medium' (not external 'P3')")

			GinkgoWriter.Printf("✅ Severity normalization validated: P3 → medium\n")
		})
	})

	// ========================================
	// TEST SUITE: Proactive Signal Mode Propagation (BR-SP-106, ADR-054)
	// Business Context: RO must copy signal mode and normalized type to AA
	// ========================================

	Context("IT-RO-084-001: Proactive Signal Mode Propagation", func() {
		var (
			namespace string
		)

		BeforeEach(func() {
			namespace = createTestNamespace("ro-signalmode")
		})

		AfterEach(func() {
			deleteTestNamespace(namespace)
		})

		It("[RO-INT-SM-001] should create AIAnalysis with signalMode=proactive and normalized signalType", func() {
			By("1. Create RemediationRequest with OOMKilled signal type")
			// Note: In production, RR has the original type (PredictedOOMKill) and SP normalizes it.
			// For this integration test, we simulate the SP normalization step manually.
			rrName := fmt.Sprintf("rr-proactive-%s", uuid.New().String()[:13])
			now := metav1.Now()
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      rrName,
					Namespace: ROControllerNamespace,
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
						Namespace: namespace,
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

			By("2. Wait for RO to create SignalProcessing")
			spName := fmt.Sprintf("sp-%s", rrName)
			sp := &signalprocessingv1.SignalProcessing{}
			Eventually(func() error {
				return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: spName, Namespace: ROControllerNamespace}, sp)
			}, timeout, interval).Should(Succeed(),
				"RO should create SignalProcessing when RR is created")

			By("3. Simulate SignalProcessing proactive mode classification (BR-SP-106)")
			// Simulate what the SignalModeClassifier would do:
			// PredictedOOMKill → signalMode=proactive, signalType=OOMKilled, originalSignalType=PredictedOOMKill
			Expect(updateSPStatusProactive(ROControllerNamespace, spName, "OOMKilled", "PredictedOOMKill", "critical")).To(Succeed())

			By("4. Wait for RO to create AIAnalysis")
			var createdAA *aianalysisv1.AIAnalysis
			Eventually(func() bool {
				var aaList aianalysisv1.AIAnalysisList
				err := k8sManager.GetAPIReader().List(ctx, &aaList,
					client.InNamespace(ROControllerNamespace))
				if err != nil {
					return false
				}
				for i := range aaList.Items {
					if aaList.Items[i].Spec.RemediationRequestRef.Name == rrName {
						createdAA = &aaList.Items[i]
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue(),
				"RO should create AIAnalysis when SignalProcessing reaches Completed phase")

			By("5. Verify AIAnalysis has proactive signal mode from SP.Status")
			Expect(createdAA.Spec.AnalysisRequest.SignalContext.SignalMode).To(Equal("proactive"),
				"BR-SP-106/ADR-054: AIAnalysis MUST propagate signalMode=proactive from SP.Status.SignalMode")

			By("6. Verify AIAnalysis has NORMALIZED signal type (not original)")
			Expect(createdAA.Spec.AnalysisRequest.SignalContext.SignalName).To(Equal("OOMKilled"),
				"BR-SP-106/ADR-054: AIAnalysis MUST use normalized SignalType from SP.Status.SignalType (not 'PredictedOOMKill')")

			GinkgoWriter.Printf("✅ Signal mode propagation validated: SP(proactive, OOMKilled) → AA(proactive, OOMKilled)\n")
		})

		It("[RO-INT-SM-002] should create AIAnalysis with signalMode=reactive for standard signals", func() {
			By("1. Create RemediationRequest with standard reactive signal")
			rrName := fmt.Sprintf("rr-reactive-%s", uuid.New().String()[:13])
			now := metav1.Now()
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      rrName,
					Namespace: ROControllerNamespace,
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
						Namespace: namespace,
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

			By("2. Wait for SignalProcessing and simulate reactive completion")
			spName := fmt.Sprintf("sp-%s", rrName)
			sp := &signalprocessingv1.SignalProcessing{}
			Eventually(func() error {
				return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: spName, Namespace: ROControllerNamespace}, sp)
			}, timeout, interval).Should(Succeed())

			// Use standard updateSPStatus which now sets reactive defaults
			Expect(updateSPStatus(ROControllerNamespace, spName, signalprocessingv1.PhaseCompleted, "critical")).To(Succeed())

			By("3. Wait for AIAnalysis and verify reactive signal mode")
			var createdAA *aianalysisv1.AIAnalysis
			Eventually(func() bool {
				var aaList aianalysisv1.AIAnalysisList
				err := k8sManager.GetAPIReader().List(ctx, &aaList,
					client.InNamespace(ROControllerNamespace))
				if err != nil {
					return false
				}
				for i := range aaList.Items {
					if aaList.Items[i].Spec.RemediationRequestRef.Name == rrName {
						createdAA = &aaList.Items[i]
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())

			Expect(createdAA.Spec.AnalysisRequest.SignalContext.SignalMode).To(Equal("reactive"),
				"Standard signals should have signalMode=reactive in AIAnalysis")
			Expect(createdAA.Spec.AnalysisRequest.SignalContext.SignalName).To(Equal("OOMKilled"),
				"Reactive signal type should pass through unchanged")

			GinkgoWriter.Printf("✅ Reactive signal mode propagation validated: SP(reactive, OOMKilled) → AA(reactive, OOMKilled)\n")
		})
	})

	Context("Standard Severity Values (Backward Compatibility)", func() {
		var (
			namespace string
			rrName    string
		)

		BeforeEach(func() {
			namespace = createTestNamespace("ro-severity-std")
			rrName = fmt.Sprintf("rr-standard-%s", uuid.New().String()[:13])
		})

		AfterEach(func() {
			deleteTestNamespace(namespace)
		})

		It("[RO-INT-SEV-005] should handle standard 'critical' severity (1:1 mapping)", func() {
			By("1. Create RemediationRequest with standard 'critical' severity")
			now := metav1.Now()
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      rrName,
					Namespace: ROControllerNamespace,
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: "e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6",
					SignalName:        "StandardCritical",
					Severity:          "critical", // Standard severity value
					SignalType:        "alert",
					TargetType:        "kubernetes",
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:      "Deployment",
						Name:      "app",
						Namespace: namespace,
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

			By("2. Wait for SignalProcessing and simulate 1:1 mapping")
			spName := fmt.Sprintf("sp-%s", rrName)
			sp := &signalprocessingv1.SignalProcessing{}
			Eventually(func() error {
				return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: spName, Namespace: ROControllerNamespace}, sp)
			}, timeout, interval).Should(Succeed())

			// Simulate SignalProcessing Rego normalization using helper (DD-SEVERITY-001)
			Expect(updateSPStatus(ROControllerNamespace, spName, signalprocessingv1.PhaseCompleted, "critical")).To(Succeed())

			By("3. Wait for AIAnalysis and verify severity")
			var createdAA *aianalysisv1.AIAnalysis
			Eventually(func() bool {
				var aaList aianalysisv1.AIAnalysisList
				err := k8sManager.GetAPIReader().List(ctx, &aaList,
					client.InNamespace(ROControllerNamespace))
				if err != nil {
					return false
				}
				for i := range aaList.Items {
					if aaList.Items[i].Spec.RemediationRequestRef.Name == rrName {
						createdAA = &aaList.Items[i]
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())

			Expect(createdAA.Spec.AnalysisRequest.SignalContext.Severity).To(Equal("critical"),
				"DD-SEVERITY-001: AIAnalysis should preserve standard 'critical' severity")

			GinkgoWriter.Printf("✅ Standard severity validated: critical → critical (1:1 mapping)\n")
		})
	})
})
