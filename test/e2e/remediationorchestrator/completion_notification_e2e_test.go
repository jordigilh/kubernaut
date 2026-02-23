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
	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// E2E-RO-045-001: Completion Notification on Successful Remediation
//
// Business Requirement: BR-ORCH-045 (Completion Notification)
// Architecture: BR-ORCH-031 (Cascade Deletion), BR-ORCH-035 (Ref Tracking)
//
// Tests that RO creates a NotificationRequest CRD with type=completion
// when WorkflowExecution completes successfully and the RemediationRequest
// transitions to the Completed phase.
//
// Full lifecycle: RR → SP → AA → WE → Completed → NotificationRequest
// Pattern: Manual child CRD status updates (no child controllers deployed)

var _ = Describe("E2E-RO-045-001: Completion Notification", Label("e2e", "notification", "remediationorchestrator"), func() {
	var (
		testNS string
	)

	BeforeEach(func() {
		testNS = createTestNamespace("ro-completion-e2e")
	})

	AfterEach(func() {
		deleteTestNamespace(testNS)
	})

	It("should create NotificationRequest with type=completion when remediation succeeds", func() {
		By("1. Creating RemediationRequest")
		rrName := "rr-completion-" + uuid.New().String()[:13]
		now := metav1.Now()
		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      rrName,
				Namespace: testNS,
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
					Kind:      "Pod",
					Name:      "test-pod-completion",
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

		By("2. Waiting for RO to create SignalProcessing CRD")
		var sp *signalprocessingv1.SignalProcessing
		Eventually(func() bool {
			spList := &signalprocessingv1.SignalProcessingList{}
			_ = k8sClient.List(ctx, spList, client.InNamespace(testNS))
			if len(spList.Items) == 0 {
				return false
			}
			sp = &spList.Items[0]
			return true
		}, timeout, interval).Should(BeTrue(), "SignalProcessing should be created by RO")

		By("3. Manually updating SP status to Completed (simulating SP controller)")
		sp.Status.Phase = signalprocessingv1.PhaseCompleted
		sp.Status.Severity = "critical"
		sp.Status.SignalMode = "reactive"
		sp.Status.SignalName = "OOMKilled"
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
			_ = k8sClient.List(ctx, analysisList, client.InNamespace(testNS))
			if len(analysisList.Items) == 0 {
				return false
			}
			analysis = &analysisList.Items[0]
			return true
		}, timeout, interval).Should(BeTrue(), "AIAnalysis should be created by RO")

		By("5. Manually updating AIAnalysis status to Completed with SelectedWorkflow (simulating AA controller)")
		analysis.Status.Phase = aianalysisv1.PhaseCompleted
		analysis.Status.Reason = "AnalysisCompleted"
		analysis.Status.Message = "Workflow recommended: restart-pod-v1"
		analysis.Status.RootCause = "Memory exhaustion due to unbounded cache growth"
		analysis.Status.SelectedWorkflow = &aianalysisv1.SelectedWorkflow{
			WorkflowID:      "restart-pod-v1",
			Version:         "1.0.0",
			ExecutionBundle:  "quay.io/kubernaut/restart-pod:v1",
			ExecutionBundleDigest: "sha256:abc123def456",
			Confidence:      0.95,
			Rationale:       "High confidence match for pod restart scenario",
			ExecutionEngine: "tekton",
			Parameters: map[string]string{
				"TARGET_POD": "test-pod-completion",
			},
		}
		Expect(k8sClient.Status().Update(ctx, analysis)).To(Succeed())

		By("6. Waiting for RO to create WorkflowExecution CRD")
		var we *workflowexecutionv1.WorkflowExecution
		Eventually(func() bool {
			weList := &workflowexecutionv1.WorkflowExecutionList{}
			_ = k8sClient.List(ctx, weList, client.InNamespace(testNS))
			if len(weList.Items) == 0 {
				return false
			}
			we = &weList.Items[0]
			return true
		}, timeout, interval).Should(BeTrue(), "WorkflowExecution should be created by RO")

		By("7. Manually updating WorkflowExecution status to Completed (simulating WE controller)")
		we.Status.Phase = workflowexecutionv1.PhaseCompleted
		Expect(k8sClient.Status().Update(ctx, we)).To(Succeed())

		By("8. Waiting for RemediationRequest to transition to Completed")
		updatedRR := &remediationv1.RemediationRequest{}
		Eventually(func() remediationv1.RemediationPhase {
			_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), updatedRR)
			return updatedRR.Status.OverallPhase
		}, timeout, interval).Should(Equal(remediationv1.PhaseCompleted),
			"RemediationRequest should transition to Completed after WE completes")

		By("9. Waiting for RO to create completion NotificationRequest (BR-ORCH-045)")
		var notification *notificationv1.NotificationRequest
		Eventually(func() bool {
			notificationList := &notificationv1.NotificationRequestList{}
			_ = k8sClient.List(ctx, notificationList, client.InNamespace(testNS))
			if len(notificationList.Items) == 0 {
				return false
			}
			// Find the completion notification (may also have bulk duplicate)
			for i := range notificationList.Items {
				if notificationList.Items[i].Spec.Type == notificationv1.NotificationTypeCompletion {
					notification = &notificationList.Items[i]
					return true
				}
			}
			return false
		}, timeout, interval).Should(BeTrue(), "NotificationRequest with type=completion should be created by RO")

		By("10. Validating completion NotificationRequest content")
		Expect(notification.Spec.Type).To(Equal(notificationv1.NotificationTypeCompletion),
			"BR-ORCH-045: Notification type must be 'completion'")
		Expect(notification.Spec.Subject).To(ContainSubstring("Remediation Completed"),
			"Subject should indicate remediation completed")
		Expect(notification.Spec.Subject).To(ContainSubstring(rr.Spec.SignalName),
			"Subject should contain signal name")
		Expect(notification.Spec.Body).To(ContainSubstring("Memory exhaustion"),
			"Body should contain root cause analysis")
		Expect(notification.Spec.Body).To(ContainSubstring("restart-pod-v1"),
			"Body should contain workflow ID")
		Expect(notification.Spec.Priority).To(Equal(notificationv1.NotificationPriorityLow),
			"Completion notifications should be low priority (informational)")

		By("11. Validating NotificationRequest spec fields for routing (Issue #91)")
		Expect(notification.Spec.Type).To(Equal(notificationv1.NotificationTypeCompletion))
		Expect(notification.Spec.RemediationRequestRef).ToNot(BeNil())
		Expect(notification.Spec.RemediationRequestRef.Name).To(Equal(rr.Name))
		Expect(notification.Spec.Severity).ToNot(BeEmpty())

		By("12. Validating NotificationRequest metadata")
		Expect(notification.Spec.Metadata).To(HaveKeyWithValue("remediationRequest", rr.Name))
		Expect(notification.Spec.Metadata).To(HaveKeyWithValue("workflowId", "restart-pod-v1"))
		Expect(notification.Spec.Metadata).To(HaveKey("rootCause"))

		By("13. Validating NotificationRequest channels include file (for E2E verification)")
		Expect(notification.Spec.Channels).To(ContainElement(notificationv1.ChannelSlack))
		Expect(notification.Spec.Channels).To(ContainElement(notificationv1.ChannelFile))

		By("14. Validating owner reference for cascade deletion (BR-ORCH-031)")
		Expect(notification.OwnerReferences).To(HaveLen(1))
		Expect(notification.OwnerReferences[0].Name).To(Equal(rr.Name))
		Expect(notification.OwnerReferences[0].Kind).To(Equal("RemediationRequest"))

		GinkgoWriter.Println("E2E-RO-045-001: Completion notification validated in Kind cluster")
	})
})
