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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	aianalysisv1alpha1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	remediationprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/remediationprocessing/v1alpha1"
	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

// Phase 4: Resilience Integration Tests
// Tests for timeout handling, failure recovery, and retention
var _ = Describe("RemediationRequest Controller - Resilience Features", func() {
	const (
		timeout  = time.Second * 30
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

	// ========================================
	// Phase 4.1: Timeout Integration Tests
	// ========================================

	Context("Timeout Handling", func() {
		It("should detect timeout in processing phase", func() {
			// GIVEN: RemediationRequest stuck in processing phase
			now := metav1.Now()
			tenMinutesAgo := metav1.NewTime(now.Add(-10 * time.Minute))

			remediationRequest := &remediationv1alpha1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-timeout-processing",
					Namespace: namespace,
				},
				Spec: remediationv1alpha1.RemediationRequestSpec{
					SignalFingerprint: "a1b2c3d4e5f61111111111111111111111111111111111111111111111111111",
					SignalName:        "test-timeout-processing",
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

			// Wait for controller to initialize
			Eventually(func() error {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: remediationRequest.Name, Namespace: namespace}, remediationRequest)
				if err != nil {
					return err
				}
				if remediationRequest.Status.OverallPhase == "" {
					return fmt.Errorf("status not initialized")
				}
				return nil
			}, timeout, interval).Should(Succeed())

			// Manually set StartTime to 10 minutes ago (exceeds 5 min processing timeout)
			Eventually(func() error {
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: remediationRequest.Name, Namespace: namespace}, remediationRequest); err != nil {
					return err
				}
				remediationRequest.Status.OverallPhase = "processing"
				remediationRequest.Status.StartTime = &tenMinutesAgo
				remediationRequest.Status.RemediationProcessingRef = &corev1.ObjectReference{
					Name:      remediationRequest.Name + "-processing",
					Namespace: namespace,
				}
				return k8sClient.Status().Update(ctx, remediationRequest)
			}, timeout, interval).Should(Succeed())

			// WHEN: Controller reconciles and detects timeout
			// THEN: Should transition to failed state
			Eventually(func() string {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: remediationRequest.Name, Namespace: namespace}, remediationRequest)
				if err != nil {
					return ""
				}
				return remediationRequest.Status.OverallPhase
			}, timeout, interval).Should(Equal("failed"))

			// Verify CompletedAt is set
			Expect(remediationRequest.Status.CompletedAt).NotTo(BeNil())
		})

		It("should detect timeout in analyzing phase", func() {
			// GIVEN: RemediationRequest stuck in analyzing phase
			now := metav1.Now()
			fifteenMinutesAgo := metav1.NewTime(now.Add(-15 * time.Minute))

			remediationRequest := &remediationv1alpha1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-timeout-analyzing",
					Namespace: namespace,
				},
				Spec: remediationv1alpha1.RemediationRequestSpec{
					SignalFingerprint: "a1b2c3d4e5f62222222222222222222222222222222222222222222222222222",
					SignalName:        "test-timeout-analyzing",
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

			// Wait for initialization
			Eventually(func() error {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: remediationRequest.Name, Namespace: namespace}, remediationRequest)
				if err != nil {
					return err
				}
				if remediationRequest.Status.OverallPhase == "" {
					return fmt.Errorf("status not initialized")
				}
				return nil
			}, timeout, interval).Should(Succeed())

			// Set to analyzing phase with old start time (exceeds 10 min timeout)
			Eventually(func() error {
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: remediationRequest.Name, Namespace: namespace}, remediationRequest); err != nil {
					return err
				}
				remediationRequest.Status.OverallPhase = "analyzing"
				remediationRequest.Status.StartTime = &fifteenMinutesAgo
				remediationRequest.Status.AIAnalysisRef = &corev1.ObjectReference{
					Name:      remediationRequest.Name + "-aianalysis",
					Namespace: namespace,
				}
				return k8sClient.Status().Update(ctx, remediationRequest)
			}, timeout, interval).Should(Succeed())

			// WHEN: Controller reconciles
			// THEN: Should transition to failed due to timeout
			Eventually(func() string {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: remediationRequest.Name, Namespace: namespace}, remediationRequest)
				if err != nil {
					return ""
				}
				return remediationRequest.Status.OverallPhase
			}, timeout, interval).Should(Equal("failed"))

			Expect(remediationRequest.Status.CompletedAt).NotTo(BeNil())
		})
	})

	// ========================================
	// Phase 4.2: Failure Recovery Integration Tests
	// ========================================

	Context("Failure Handling", func() {
		It("should transition to failed when RemediationProcessing fails", func() {
			// GIVEN: RemediationRequest with failed RemediationProcessing
			now := metav1.Now()

			remediationRequest := &remediationv1alpha1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-failure-processing",
					Namespace: namespace,
				},
				Spec: remediationv1alpha1.RemediationRequestSpec{
					SignalFingerprint: "a1b2c3d4e5f63333333333333333333333333333333333333333333333333333",
					SignalName:        "test-failure-processing",
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

			// Wait for controller to create RemediationProcessing
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

			// Get the RemediationProcessing and mark it as failed
			remediationProcessing := &remediationprocessingv1alpha1.RemediationProcessing{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      remediationRequest.Status.RemediationProcessingRef.Name,
					Namespace: namespace,
				}, remediationProcessing)
			}, timeout, interval).Should(Succeed())

			// Mark as failed
			Eventually(func() error {
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      remediationProcessing.Name,
					Namespace: namespace,
				}, remediationProcessing); err != nil {
					return err
				}
				remediationProcessing.Status.Phase = "failed"
				return k8sClient.Status().Update(ctx, remediationProcessing)
			}, timeout, interval).Should(Succeed())

			// WHEN: Controller detects child CRD failure
			// THEN: Should transition RemediationRequest to failed
			Eventually(func() string {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: remediationRequest.Name, Namespace: namespace}, remediationRequest)
				if err != nil {
					return ""
				}
				return remediationRequest.Status.OverallPhase
			}, timeout, interval).Should(Equal("failed"))

			Expect(remediationRequest.Status.CompletedAt).NotTo(BeNil())
		})

		It("should transition to failed when AIAnalysis fails", func() {
			// GIVEN: RemediationRequest with completed RemediationProcessing and failed AIAnalysis
			now := metav1.Now()

			remediationRequest := &remediationv1alpha1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-failure-aianalysis",
					Namespace: namespace,
				},
				Spec: remediationv1alpha1.RemediationRequestSpec{
					SignalFingerprint: "a1b2c3d4e5f64444444444444444444444444444444444444444444444444444",
					SignalName:        "test-failure-aianalysis",
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

			// Wait for RemediationProcessing to be created
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

			// Mark RemediationProcessing as completed
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

			// Wait for AIAnalysis to be created
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

			// Mark AIAnalysis as failed
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

			// THEN: Should transition to failed
			Eventually(func() string {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: remediationRequest.Name, Namespace: namespace}, remediationRequest)
				if err != nil {
					return ""
				}
				return remediationRequest.Status.OverallPhase
			}, timeout, interval).Should(Equal("failed"))
		})

		It("should transition to failed when WorkflowExecution fails", func() {
			// GIVEN: RemediationRequest with all prerequisites completed and failed WorkflowExecution
			now := metav1.Now()

			remediationRequest := &remediationv1alpha1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-failure-workflow",
					Namespace: namespace,
				},
				Spec: remediationv1alpha1.RemediationRequestSpec{
					SignalFingerprint: "a1b2c3d4e5f65555555555555555555555555555555555555555555555555555",
					SignalName:        "test-failure-workflow",
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

			// Complete AIAnalysis
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

			aiAnalysis := &aianalysisv1alpha1.AIAnalysis{}
			Eventually(func() error {
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      remediationRequest.Status.AIAnalysisRef.Name,
					Namespace: namespace,
				}, aiAnalysis); err != nil {
					return err
				}
				aiAnalysis.Status.Phase = "Completed"
				aiAnalysis.Status.RecommendedAction = "restart_pod"
				return k8sClient.Status().Update(ctx, aiAnalysis)
			}, timeout, interval).Should(Succeed())

			// Wait for WorkflowExecution creation
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

			// Mark WorkflowExecution as failed
			workflow := &workflowexecutionv1alpha1.WorkflowExecution{}
			Eventually(func() error {
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      remediationRequest.Status.WorkflowExecutionRef.Name,
					Namespace: namespace,
				}, workflow); err != nil {
					return err
				}
				workflow.Status.Phase = "failed"
				return k8sClient.Status().Update(ctx, workflow)
			}, timeout, interval).Should(Succeed())

			// THEN: Should transition to failed
			Eventually(func() string {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: remediationRequest.Name, Namespace: namespace}, remediationRequest)
				if err != nil {
					return ""
				}
				return remediationRequest.Status.OverallPhase
			}, timeout, interval).Should(Equal("failed"))
		})
	})

	// ========================================
	// Phase 4.3: Retention Integration Tests
	// ========================================

	Context("24-Hour Retention", func() {
		It("should add finalizer automatically on creation", func() {
			// GIVEN: New RemediationRequest
			now := metav1.Now()

			remediationRequest := &remediationv1alpha1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-finalizer-added",
					Namespace: namespace,
				},
				Spec: remediationv1alpha1.RemediationRequestSpec{
					SignalFingerprint: "a1b2c3d4e5f66666666666666666666666666666666666666666666666666666",
					SignalName:        "test-finalizer-added",
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

			// THEN: Finalizer should be added
			Eventually(func() []string {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: remediationRequest.Name, Namespace: namespace}, remediationRequest)
				if err != nil {
					return nil
				}
				return remediationRequest.ObjectMeta.Finalizers
			}, timeout, interval).Should(ContainElement("kubernaut.io/remediation-retention"))
		})

		It("should NOT delete completed RemediationRequest before retention expires", func() {
			// GIVEN: Completed RemediationRequest with recent completion time
			now := metav1.Now()
			oneHourAgo := metav1.NewTime(now.Add(-1 * time.Hour))

			remediationRequest := &remediationv1alpha1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-retention-not-expired",
					Namespace: namespace,
				},
				Spec: remediationv1alpha1.RemediationRequestSpec{
					SignalFingerprint: "a1b2c3d4e5f67777777777777777777777777777777777777777777777777777",
					SignalName:        "test-retention-not-expired",
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

			// Wait for initialization and mark as completed
			Eventually(func() error {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: remediationRequest.Name, Namespace: namespace}, remediationRequest)
				if err != nil {
					return err
				}
				if remediationRequest.Status.OverallPhase == "" {
					return fmt.Errorf("status not initialized")
				}
				return nil
			}, timeout, interval).Should(Succeed())

			Eventually(func() error {
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: remediationRequest.Name, Namespace: namespace}, remediationRequest); err != nil {
					return err
				}
				remediationRequest.Status.OverallPhase = "completed"
				remediationRequest.Status.CompletedAt = &oneHourAgo // 1 hour ago, not expired (24h retention)
				return k8sClient.Status().Update(ctx, remediationRequest)
			}, timeout, interval).Should(Succeed())

			// THEN: Should still exist after multiple reconciliations
			Consistently(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: remediationRequest.Name, Namespace: namespace}, remediationRequest)
			}, time.Second*5, interval).Should(Succeed())

			// Should still be in completed phase
			Expect(remediationRequest.Status.OverallPhase).To(Equal("completed"))
		})
	})
})
