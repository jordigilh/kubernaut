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

package workflowexecution

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	workflowexecution "github.com/jordigilh/kubernaut/internal/controller/workflowexecution"
	weconditions "github.com/jordigilh/kubernaut/pkg/workflowexecution"
)

// ========================================
// BR-WE-006: Kubernetes Conditions Integration Tests
// Per TESTING_GUIDELINES.md: These are INTEGRATION tests (real controller + K8s API)
// Per WE_BR_WE_006_TESTING_TRIAGE.md: Use Eventually() patterns, NO time.Sleep()
// ========================================

var _ = Describe("Conditions Integration", Label("integration", "conditions"), func() {
	Context("TektonPipelineCreated condition", func() {
		It("should be set after PipelineRun creation during reconciliation", func() {
			// Create WorkflowExecution
			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "wfe-condition-pipeline-created",
					Namespace:  DefaultNamespace,
					Generation: 1, // K8s increments on create/update
				},
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					RemediationRequestRef: corev1.ObjectReference{
						APIVersion: "remediation.kubernaut.ai/v1alpha1",
						Kind:       "RemediationRequest",
						Name:       "test-rr-condition-pipeline-created",
						Namespace:  DefaultNamespace,
					},
					WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
						WorkflowID:     "test-workflow",
						Version:        "v1.0.0",
						ContainerImage: "quay.io/kubernaut/workflows/test-hello-world:v1.0.0",
					},
					TargetResource: "default/deployment/condition-test-app",
					Parameters: map[string]string{
						"MESSAGE": "Testing TektonPipelineCreated condition",
					},
				},
			}
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			// ✅ REQUIRED: Use Eventually() to wait for condition (NO time.Sleep())
			// Per TESTING_GUIDELINES.md lines 443-487
			key := client.ObjectKeyFromObject(wfe)
			Eventually(func() []metav1.Condition {
				updated := &workflowexecutionv1alpha1.WorkflowExecution{}
				_ = k8sClient.Get(ctx, key, updated)
				return updated.Status.Conditions
			}, 30*time.Second, 1*time.Second).Should(ContainElement(
				And(
					HaveField("Type", weconditions.ConditionTektonPipelineCreated),
					HaveField("Status", metav1.ConditionTrue),
					HaveField("Reason", weconditions.ReasonPipelineCreated),
				),
			), "TektonPipelineCreated condition should be set after PipelineRun creation")

			// Verify PipelineRun was actually created
			var pr tektonv1.PipelineRun
			Eventually(func() error {
				prName := workflowexecution.PipelineRunName(wfe.Spec.TargetResource)
				return k8sClient.Get(ctx, client.ObjectKey{
					Name:      prName,
					Namespace: WorkflowExecutionNS,
				}, &pr)
			}, 10*time.Second, 1*time.Second).Should(Succeed(),
				"PipelineRun should be created in execution namespace")

			// Verify condition message includes PipelineRun name
			updated := &workflowexecutionv1alpha1.WorkflowExecution{}
			Expect(k8sClient.Get(ctx, key, updated)).To(Succeed())
			condition := weconditions.GetCondition(updated, weconditions.ConditionTektonPipelineCreated)
			Expect(condition.Message).To(ContainSubstring(pr.Name))
		})
	})

	// V1.0 NOTE: ResourceLocked condition test removed - routing moved to RO (DD-RO-002)
	// RO prevents creation of second WFE for same target, so WE never sees this scenario

	Context("TektonPipelineRunning condition", func() {
		It("should be set when PipelineRun starts executing", func() {
			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "wfe-condition-running",
					Namespace:  DefaultNamespace,
					Generation: 1, // K8s increments on create/update
				},
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					RemediationRequestRef: corev1.ObjectReference{
						APIVersion: "remediation.kubernaut.ai/v1alpha1",
						Kind:       "RemediationRequest",
						Name:       "test-rr-condition-running",
						Namespace:  DefaultNamespace,
					},
					WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
						WorkflowID:     "test-workflow",
						Version:        "v1.0.0",
						ContainerImage: "quay.io/kubernaut/workflows/test-hello-world:v1.0.0",
					},
					TargetResource: "default/deployment/running-test-app",
				},
			}
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			// Wait for WFE to reach Running phase
			key := client.ObjectKeyFromObject(wfe)
			Eventually(func() string {
				updated := &workflowexecutionv1alpha1.WorkflowExecution{}
				_ = k8sClient.Get(ctx, key, updated)
				return updated.Status.Phase
			}, 30*time.Second, 1*time.Second).Should(Equal(workflowexecutionv1alpha1.PhaseRunning))

			// Get the created PipelineRun
			var pr tektonv1.PipelineRun
			prName := workflowexecution.PipelineRunName(wfe.Spec.TargetResource)
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKey{
					Name:      prName,
					Namespace: WorkflowExecutionNS,
				}, &pr)
			}, 10*time.Second, 1*time.Second).Should(Succeed())

			// Simulate PipelineRun starting (set Succeeded condition to Unknown/Running)
			pr.Status.Conditions = duckv1.Conditions{
				{
					Type:   apis.ConditionSucceeded,
					Status: corev1.ConditionUnknown,
					Reason: "Running",
				},
			}
			Expect(k8sClient.Status().Update(ctx, &pr)).To(Succeed())

			// ✅ REQUIRED: Eventually() for condition check
			Eventually(func() bool {
				updated := &workflowexecutionv1alpha1.WorkflowExecution{}
				_ = k8sClient.Get(ctx, key, updated)
				return weconditions.IsConditionTrue(updated, weconditions.ConditionTektonPipelineRunning)
			}, 30*time.Second, 1*time.Second).Should(BeTrue(),
				"TektonPipelineRunning condition should be set when PipelineRun is running")
		})
	})

	Context("TektonPipelineComplete condition", func() {
		It("should be set to True when PipelineRun succeeds", func() {
			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "wfe-condition-complete-success",
					Namespace:  DefaultNamespace,
					Generation: 1, // K8s increments on create/update
				},
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					RemediationRequestRef: corev1.ObjectReference{
						APIVersion: "remediation.kubernaut.ai/v1alpha1",
						Kind:       "RemediationRequest",
						Name:       "test-rr-condition-complete-success",
						Namespace:  DefaultNamespace,
					},
					WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
						WorkflowID:     "test-workflow",
						Version:        "v1.0.0",
						ContainerImage: "quay.io/kubernaut/workflows/test-hello-world:v1.0.0",
					},
					TargetResource: "default/deployment/complete-success-app",
				},
			}
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			// Wait for Running phase
			key := client.ObjectKeyFromObject(wfe)
			Eventually(func() string {
				updated := &workflowexecutionv1alpha1.WorkflowExecution{}
				_ = k8sClient.Get(ctx, key, updated)
				return updated.Status.Phase
			}, 30*time.Second, 1*time.Second).Should(Equal(workflowexecutionv1alpha1.PhaseRunning))

			// Get PipelineRun and mark as succeeded
			var pr tektonv1.PipelineRun
			prName := workflowexecution.PipelineRunName(wfe.Spec.TargetResource)
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKey{
					Name:      prName,
					Namespace: WorkflowExecutionNS,
				}, &pr)
			}, 10*time.Second, 1*time.Second).Should(Succeed())

			// Simulate successful completion
			now := metav1.Now()
			pr.Status.Conditions = duckv1.Conditions{
				{
					Type:   apis.ConditionSucceeded,
					Status: corev1.ConditionTrue,
					Reason: "Succeeded",
				},
			}
			pr.Status.CompletionTime = &now
			Expect(k8sClient.Status().Update(ctx, &pr)).To(Succeed())

			// ✅ REQUIRED: Eventually() for condition check
			Eventually(func() bool {
				updated := &workflowexecutionv1alpha1.WorkflowExecution{}
				_ = k8sClient.Get(ctx, key, updated)
				return weconditions.IsConditionTrue(updated, weconditions.ConditionTektonPipelineComplete)
			}, 30*time.Second, 1*time.Second).Should(BeTrue(),
				"TektonPipelineComplete condition should be True when pipeline succeeds")

			// Verify WFE reached Completed phase
			Eventually(func() string {
				updated := &workflowexecutionv1alpha1.WorkflowExecution{}
				_ = k8sClient.Get(ctx, key, updated)
				return updated.Status.Phase
			}, 10*time.Second, 1*time.Second).Should(Equal(workflowexecutionv1alpha1.PhaseCompleted))

			// Verify condition reason is PipelineSucceeded
			updated := &workflowexecutionv1alpha1.WorkflowExecution{}
			Expect(k8sClient.Get(ctx, key, updated)).To(Succeed())
			condition := weconditions.GetCondition(updated, weconditions.ConditionTektonPipelineComplete)
			Expect(condition.Reason).To(Equal(weconditions.ReasonPipelineSucceeded))
		})

		// REMOVED: Moved to E2E suite
		// See: test/e2e/workflowexecution/01_lifecycle_test.go (BR-WE-004)
		// Test: "should populate failure details when workflow fails"
		// Reason: EnvTest doesn't trigger reconciliation on cross-namespace PipelineRun status updates
	})

	Context("AuditRecorded condition", func() {
		It("should be set after audit event emission", func() {
			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "wfe-condition-audit",
					Namespace:  DefaultNamespace,
					Generation: 1, // K8s increments on create/update
				},
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					RemediationRequestRef: corev1.ObjectReference{
						APIVersion: "remediation.kubernaut.ai/v1alpha1",
						Kind:       "RemediationRequest",
						Name:       "test-rr-condition-audit",
						Namespace:  DefaultNamespace,
					},
					WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
						WorkflowID:     "test-workflow",
						Version:        "v1.0.0",
						ContainerImage: "quay.io/kubernaut/workflows/test-hello-world:v1.0.0",
					},
					TargetResource: "default/deployment/audit-test-app",
				},
			}
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			// Wait for WFE to reach Running phase (audit event emitted)
			key := client.ObjectKeyFromObject(wfe)
			Eventually(func() string {
				updated := &workflowexecutionv1alpha1.WorkflowExecution{}
				_ = k8sClient.Get(ctx, key, updated)
				return updated.Status.Phase
			}, 30*time.Second, 1*time.Second).Should(Equal(workflowexecutionv1alpha1.PhaseRunning))

			// ✅ REQUIRED: Eventually() for condition check
			// Note: AuditRecorded may be True or False depending on mock audit store
			// We just verify the condition is SET (not nil)
			Eventually(func() *metav1.Condition {
				updated := &workflowexecutionv1alpha1.WorkflowExecution{}
				_ = k8sClient.Get(ctx, key, updated)
				return weconditions.GetCondition(updated, weconditions.ConditionAuditRecorded)
			}, 10*time.Second, 1*time.Second).ShouldNot(BeNil(),
				"AuditRecorded condition should be set after audit event emission")

			// Verify condition has appropriate reason (either Succeeded or Failed)
			updated := &workflowexecutionv1alpha1.WorkflowExecution{}
			Expect(k8sClient.Get(ctx, key, updated)).To(Succeed())
			condition := weconditions.GetCondition(updated, weconditions.ConditionAuditRecorded)
			Expect(condition.Reason).To(Or(
				Equal(weconditions.ReasonAuditSucceeded),
				Equal(weconditions.ReasonAuditFailed),
			), "AuditRecorded condition should have Succeeded or Failed reason")
		})
	})

	Context("Complete lifecycle with all conditions", func() {
		It("should set all applicable conditions during successful execution", func() {
			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "wfe-condition-full-lifecycle",
					Namespace:  DefaultNamespace,
					Generation: 1, // K8s increments on create/update
				},
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					RemediationRequestRef: corev1.ObjectReference{
						APIVersion: "remediation.kubernaut.ai/v1alpha1",
						Kind:       "RemediationRequest",
						Name:       "test-rr-condition-full-lifecycle",
						Namespace:  DefaultNamespace,
					},
					WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
						WorkflowID:     "test-workflow",
						Version:        "v1.0.0",
						ContainerImage: "quay.io/kubernaut/workflows/test-hello-world:v1.0.0",
					},
					TargetResource: "default/deployment/full-lifecycle-app",
				},
			}
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			key := client.ObjectKeyFromObject(wfe)

			// 1. TektonPipelineCreated should be set
			// Timeout increased to 60s to allow multiple reconciliation cycles in EnvTest (10s requeue interval)
			Eventually(func() bool {
				updated := &workflowexecutionv1alpha1.WorkflowExecution{}
				_ = k8sClient.Get(ctx, key, updated)
				return weconditions.IsConditionTrue(updated, weconditions.ConditionTektonPipelineCreated)
			}, 60*time.Second, 1*time.Second).Should(BeTrue())

			// 2. TektonPipelineRunning should be set
			// Timeout increased to 60s to allow multiple reconciliation cycles in EnvTest (10s requeue interval)
			Eventually(func() bool {
				updated := &workflowexecutionv1alpha1.WorkflowExecution{}
				_ = k8sClient.Get(ctx, key, updated)
				return weconditions.IsConditionTrue(updated, weconditions.ConditionTektonPipelineRunning)
			}, 60*time.Second, 1*time.Second).Should(BeTrue())

			// 3. Complete the PipelineRun
			var pr tektonv1.PipelineRun
			prName := workflowexecution.PipelineRunName(wfe.Spec.TargetResource)
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKey{
					Name:      prName,
					Namespace: WorkflowExecutionNS,
				}, &pr)
			}, 10*time.Second, 1*time.Second).Should(Succeed())

			now := metav1.Now()
			pr.Status.Conditions = duckv1.Conditions{
				{
					Type:   apis.ConditionSucceeded,
					Status: corev1.ConditionTrue,
					Reason: "Succeeded",
				},
			}
			pr.Status.CompletionTime = &now
			Expect(k8sClient.Status().Update(ctx, &pr)).To(Succeed())

			// 4. TektonPipelineComplete should be set to True
			// Timeout increased to 60s to allow multiple reconciliation cycles in EnvTest (10s requeue interval)
			Eventually(func() bool {
				updated := &workflowexecutionv1alpha1.WorkflowExecution{}
				_ = k8sClient.Get(ctx, key, updated)
				return weconditions.IsConditionTrue(updated, weconditions.ConditionTektonPipelineComplete)
			}, 60*time.Second, 1*time.Second).Should(BeTrue())

			// 5. Verify WFE reached Completed phase
			Eventually(func() string {
				updated := &workflowexecutionv1alpha1.WorkflowExecution{}
				_ = k8sClient.Get(ctx, key, updated)
				return updated.Status.Phase
			}, 10*time.Second, 1*time.Second).Should(Equal(workflowexecutionv1alpha1.PhaseCompleted))

			// 6. Verify all expected conditions are present
			updated := &workflowexecutionv1alpha1.WorkflowExecution{}
			Expect(k8sClient.Get(ctx, key, updated)).To(Succeed())
			Expect(updated.Status.Conditions).To(HaveLen(4),
				"Complete lifecycle should have 4 conditions: Created, Running, Complete, AuditRecorded")

			// Verify all conditions are True (success scenario)
			Expect(weconditions.IsConditionTrue(updated, weconditions.ConditionTektonPipelineCreated)).To(BeTrue())
			Expect(weconditions.IsConditionTrue(updated, weconditions.ConditionTektonPipelineRunning)).To(BeTrue())
			Expect(weconditions.IsConditionTrue(updated, weconditions.ConditionTektonPipelineComplete)).To(BeTrue())
			// AuditRecorded may be True or False depending on mock - just verify it exists
			Expect(weconditions.GetCondition(updated, weconditions.ConditionAuditRecorded)).ToNot(BeNil())
		})
	})
})
