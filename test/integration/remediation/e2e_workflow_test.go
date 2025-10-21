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

package remediation

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	aianalysisv1alpha1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	remediationprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/remediationprocessing/v1alpha1"
	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

// Phase 4.4: Full E2E Workflow Tests
// Tests the complete orchestration flow from start to finish
var _ = Describe("RemediationRequest Controller - E2E Workflow", func() {
	const (
		timeout  = time.Second * 45
		interval = time.Millisecond * 250
	)

	var (
		ctx       context.Context
		namespace string
	)

	BeforeEach(func() {
		ctx = context.Background()
		namespace = "default"
	})

	Context("Complete Successful Flow", func() {
		It("should successfully complete full remediation workflow from start to finish", func() {
			// GIVEN: New RemediationRequest
			now := metav1.Now()

			remediationRequest := &remediationv1alpha1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-e2e-success",
					Namespace: namespace,
				},
				Spec: remediationv1alpha1.RemediationRequestSpec{
					SignalFingerprint: "e2e1a1b2c3d4e5f6111111111111111111111111111111111111111111111111",
					SignalName:        "test-e2e-success",
					Severity:          "critical",
					Environment:       "prod",
					Priority:          "P0",
					SignalType:        "prometheus-alert",
					TargetType:        "kubernetes",
					FiringTime:        now,
					ReceivedTime:      now,
					Deduplication: remediationv1alpha1.DeduplicationInfo{
						IsDuplicate: false,
						FirstSeen:   now,
						LastSeen:    now,
					},
				},
			}
			Expect(k8sClient.Create(ctx, remediationRequest)).To(Succeed())

			// THEN Phase 1: Should initialize and add finalizer
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: remediationRequest.Name, Namespace: namespace}, remediationRequest)
				if err != nil {
					return false
				}
				return len(remediationRequest.ObjectMeta.Finalizers) > 0 &&
					remediationRequest.Status.OverallPhase != ""
			}, timeout, interval).Should(BeTrue())

			Expect(remediationRequest.ObjectMeta.Finalizers).To(ContainElement("kubernaut.io/remediation-retention"))
			Expect(remediationRequest.Status.StartTime).NotTo(BeNil())

			// THEN Phase 2: Should create RemediationProcessing
			Eventually(func() error {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: remediationRequest.Name, Namespace: namespace}, remediationRequest)
				if err != nil {
					return err
				}
				if remediationRequest.Status.RemediationProcessingRef == nil {
					return fmt.Errorf("RemediationProcessing not created yet")
				}
				return nil
			}, timeout, interval).Should(Succeed())

			Expect(remediationRequest.Status.OverallPhase).To(Equal("processing"))

			// Complete RemediationProcessing
			remediationProcessing := &remediationprocessingv1alpha1.RemediationProcessing{}
			Eventually(func() error {
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      remediationRequest.Status.RemediationProcessingRef.Name,
					Namespace: namespace,
				}, remediationProcessing); err != nil {
					return err
				}
				remediationProcessing.Status.Phase = "completed"
				remediationProcessing.Status.ContextData = map[string]string{
					"cluster_name": "prod-cluster",
					"namespace":    "production",
					"severity":     "high",
				}
				return k8sClient.Status().Update(ctx, remediationProcessing)
			}, timeout, interval).Should(Succeed())

			// THEN Phase 3: Should create AIAnalysis
			Eventually(func() error {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: remediationRequest.Name, Namespace: namespace}, remediationRequest)
				if err != nil {
					return err
				}
				if remediationRequest.Status.AIAnalysisRef == nil {
					return fmt.Errorf("AIAnalysis not created yet")
				}
				return nil
			}, timeout, interval).Should(Succeed())

			Expect(remediationRequest.Status.OverallPhase).To(Equal("analyzing"))

			// Verify AIAnalysis has correct data
			aiAnalysis := &aianalysisv1alpha1.AIAnalysis{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      remediationRequest.Status.AIAnalysisRef.Name,
					Namespace: namespace,
				}, aiAnalysis)
			}, timeout, interval).Should(Succeed())

			Expect(aiAnalysis.Spec.SignalContext).To(HaveKey("cluster_name"))
			Expect(aiAnalysis.Spec.SignalContext["cluster_name"]).To(Equal("prod-cluster"))

			// Complete AIAnalysis
			Eventually(func() error {
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      aiAnalysis.Name,
					Namespace: namespace,
				}, aiAnalysis); err != nil {
					return err
				}
				aiAnalysis.Status.Phase = "Completed"
				aiAnalysis.Status.RootCause = "High CPU usage due to memory leak"
				aiAnalysis.Status.RecommendedAction = "restart_pod"
				aiAnalysis.Status.Confidence = 0.95
				return k8sClient.Status().Update(ctx, aiAnalysis)
			}, timeout, interval).Should(Succeed())

			// THEN Phase 4: Should create WorkflowExecution
			Eventually(func() error {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: remediationRequest.Name, Namespace: namespace}, remediationRequest)
				if err != nil {
					return err
				}
				if remediationRequest.Status.WorkflowExecutionRef == nil {
					return fmt.Errorf("WorkflowExecution not created yet")
				}
				return nil
			}, timeout, interval).Should(Succeed())

			Expect(remediationRequest.Status.OverallPhase).To(Equal("executing"))

			// Verify WorkflowExecution has correct workflow definition
			workflow := &workflowexecutionv1alpha1.WorkflowExecution{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      remediationRequest.Status.WorkflowExecutionRef.Name,
					Namespace: namespace,
				}, workflow)
			}, timeout, interval).Should(Succeed())

			Expect(workflow.Spec.WorkflowDefinition.Steps).To(HaveLen(1))
			Expect(workflow.Spec.WorkflowDefinition.Steps[0].Action).To(Equal("restart_pod"))

			// Complete WorkflowExecution
			Eventually(func() error {
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      workflow.Name,
					Namespace: namespace,
				}, workflow); err != nil {
					return err
				}
				workflow.Status.Phase = "completed"
				workflow.Status.WorkflowResult = &workflowexecutionv1alpha1.WorkflowResult{
					Outcome:            "success",
					EffectivenessScore: 0.98,
					ResourceHealth:     "healthy",
				}
				return k8sClient.Status().Update(ctx, workflow)
			}, timeout, interval).Should(Succeed())

			// THEN Phase 5: Should transition to completed
			Eventually(func() string {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: remediationRequest.Name, Namespace: namespace}, remediationRequest)
				if err != nil {
					return ""
				}
				return remediationRequest.Status.OverallPhase
			}, timeout, interval).Should(Equal("completed"))

			// Verify completion metadata
			Expect(remediationRequest.Status.CompletedAt).NotTo(BeNil())
			Expect(remediationRequest.Status.CompletedAt.After(remediationRequest.Status.StartTime.Time)).To(BeTrue())

			// Verify all child CRD references are set
			Expect(remediationRequest.Status.RemediationProcessingRef).NotTo(BeNil())
			Expect(remediationRequest.Status.AIAnalysisRef).NotTo(BeNil())
			Expect(remediationRequest.Status.WorkflowExecutionRef).NotTo(BeNil())
		})
	})

	Context("Full Flow with Early Failure", func() {
		It("should handle failure at RemediationProcessing phase", func() {
			// GIVEN: New RemediationRequest
			now := metav1.Now()

			remediationRequest := &remediationv1alpha1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-e2e-early-failure",
					Namespace: namespace,
				},
				Spec: remediationv1alpha1.RemediationRequestSpec{
					SignalFingerprint: "e2e2a1b2c3d4e5f6222222222222222222222222222222222222222222222222",
					SignalName:        "test-e2e-early-failure",
					Severity:          "critical",
					Environment:       "prod",
					Priority:          "P0",
					SignalType:        "prometheus-alert",
					TargetType:        "kubernetes",
					FiringTime:        now,
					ReceivedTime:      now,
					Deduplication: remediationv1alpha1.DeduplicationInfo{
						IsDuplicate: false,
						FirstSeen:   now,
						LastSeen:    now,
					},
				},
			}
			Expect(k8sClient.Create(ctx, remediationRequest)).To(Succeed())

			// Wait for RemediationProcessing creation
			Eventually(func() error {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: remediationRequest.Name, Namespace: namespace}, remediationRequest)
				if err != nil {
					return err
				}
				if remediationRequest.Status.RemediationProcessingRef == nil {
					return fmt.Errorf("RemediationProcessing not created yet")
				}
				return nil
			}, timeout, interval).Should(Succeed())

			// Mark RemediationProcessing as failed (early failure)
			remediationProcessing := &remediationprocessingv1alpha1.RemediationProcessing{}
			Eventually(func() error {
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      remediationRequest.Status.RemediationProcessingRef.Name,
					Namespace: namespace,
				}, remediationProcessing); err != nil {
					return err
				}
				remediationProcessing.Status.Phase = "failed"
				return k8sClient.Status().Update(ctx, remediationProcessing)
			}, timeout, interval).Should(Succeed())

			// THEN: Should transition to failed without creating further CRDs
			Eventually(func() string {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: remediationRequest.Name, Namespace: namespace}, remediationRequest)
				if err != nil {
					return ""
				}
				return remediationRequest.Status.OverallPhase
			}, timeout, interval).Should(Equal("failed"))

			// Verify no further CRDs were created
			Expect(remediationRequest.Status.AIAnalysisRef).To(BeNil())
			Expect(remediationRequest.Status.WorkflowExecutionRef).To(BeNil())
			Expect(remediationRequest.Status.CompletedAt).NotTo(BeNil())
		})

		It("should handle failure at AIAnalysis phase", func() {
			// GIVEN: New RemediationRequest
			now := metav1.Now()

			remediationRequest := &remediationv1alpha1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-e2e-mid-failure",
					Namespace: namespace,
				},
				Spec: remediationv1alpha1.RemediationRequestSpec{
					SignalFingerprint: "e2e3a1b2c3d4e5f6333333333333333333333333333333333333333333333333",
					SignalName:        "test-e2e-mid-failure",
					Severity:          "critical",
					Environment:       "prod",
					Priority:          "P0",
					SignalType:        "prometheus-alert",
					TargetType:        "kubernetes",
					FiringTime:        now,
					ReceivedTime:      now,
					Deduplication: remediationv1alpha1.DeduplicationInfo{
						IsDuplicate: false,
						FirstSeen:   now,
						LastSeen:    now,
					},
				},
			}
			Expect(k8sClient.Create(ctx, remediationRequest)).To(Succeed())

			// Complete RemediationProcessing
			Eventually(func() error {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: remediationRequest.Name, Namespace: namespace}, remediationRequest)
				if err != nil {
					return err
				}
				if remediationRequest.Status.RemediationProcessingRef == nil {
					return fmt.Errorf("RemediationProcessing not created yet")
				}
				return nil
			}, timeout, interval).Should(Succeed())

			remediationProcessing := &remediationprocessingv1alpha1.RemediationProcessing{}
			Eventually(func() error {
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      remediationRequest.Status.RemediationProcessingRef.Name,
					Namespace: namespace,
				}, remediationProcessing); err != nil {
					return err
				}
				remediationProcessing.Status.Phase = "completed"
				remediationProcessing.Status.ContextData = map[string]string{"test": "data"}
				return k8sClient.Status().Update(ctx, remediationProcessing)
			}, timeout, interval).Should(Succeed())

			// Wait for AIAnalysis creation
			Eventually(func() error {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: remediationRequest.Name, Namespace: namespace}, remediationRequest)
				if err != nil {
					return err
				}
				if remediationRequest.Status.AIAnalysisRef == nil {
					return fmt.Errorf("AIAnalysis not created yet")
				}
				return nil
			}, timeout, interval).Should(Succeed())

			// Mark AIAnalysis as failed (mid-flow failure)
			aiAnalysis := &aianalysisv1alpha1.AIAnalysis{}
			Eventually(func() error {
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      remediationRequest.Status.AIAnalysisRef.Name,
					Namespace: namespace,
				}, aiAnalysis); err != nil {
					return err
				}
				aiAnalysis.Status.Phase = "Failed"
				return k8sClient.Status().Update(ctx, aiAnalysis)
			}, timeout, interval).Should(Succeed())

			// THEN: Should transition to failed without creating WorkflowExecution
			Eventually(func() string {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: remediationRequest.Name, Namespace: namespace}, remediationRequest)
				if err != nil {
					return ""
				}
				return remediationRequest.Status.OverallPhase
			}, timeout, interval).Should(Equal("failed"))

			// Verify WorkflowExecution was NOT created
			Expect(remediationRequest.Status.WorkflowExecutionRef).To(BeNil())
		})
	})
})
