/*
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package remediation_test

import (
	"context"
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

// ========================================
// TEST SUITE: RemediationRequest Controller - Phase 1
// Business Requirement: BR-ORCHESTRATION-001
// ========================================

var _ = Describe("RemediationRequest Controller - Task 1.1: AIAnalysis CRD Creation", func() {
	const (
		timeout  = time.Second * 10
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
	// TEST 1: Create AIAnalysis when RemediationProcessing completes
	// Business Requirement: BR-ORCHESTRATION-001
	// ========================================
	It("should create AIAnalysis CRD when RemediationProcessing phase is 'completed'", func() {
		// GIVEN: A RemediationRequest exists
		now := metav1.Now()
		remediationRequest := &remediationv1alpha1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-remediation-001",
				Namespace: namespace,
			},
			Spec: remediationv1alpha1.RemediationRequestSpec{
				SignalFingerprint: "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
				SignalName:        "high-cpu-usage",
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

		// WHEN: RemediationProcessing CRD exists and is marked as 'completed'
		remediationProcessing := &remediationprocessingv1alpha1.RemediationProcessing{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remediationRequest.Name + "-processing",
				Namespace: namespace,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: "remediation.kubernaut.io/v1alpha1",
						Kind:       "RemediationRequest",
						Name:       remediationRequest.Name,
						UID:        remediationRequest.UID,
						Controller: func() *bool { b := true; return &b }(),
					},
				},
			},
			Spec: remediationprocessingv1alpha1.RemediationProcessingSpec{
				SignalFingerprint: remediationRequest.Spec.SignalFingerprint,
				SignalName:        remediationRequest.Spec.SignalName,
				Severity:          remediationRequest.Spec.Severity,
				Environment:       remediationRequest.Spec.Environment,
				Priority:          remediationRequest.Spec.Priority,
				SignalType:        remediationRequest.Spec.SignalType,
				TargetType:        remediationRequest.Spec.TargetType,
				ReceivedTime:      now,
				Deduplication: remediationprocessingv1alpha1.DeduplicationContext{
					FirstOccurrence: now,
					LastOccurrence:  now,
				},
			},
		}
		Expect(k8sClient.Create(ctx, remediationProcessing)).To(Succeed())

		// Update RemediationProcessing status to 'completed'
		remediationProcessing.Status.Phase = "completed"
		remediationProcessing.Status.ContextData = map[string]string{
			"test-key": "test-value",
		}
		Expect(k8sClient.Status().Update(ctx, remediationProcessing)).To(Succeed())

		// Update RemediationRequest status to reference RemediationProcessing
		// Refetch to get latest resourceVersion (controller may have modified it)
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: remediationRequest.Name, Namespace: remediationRequest.Namespace}, remediationRequest)).To(Succeed())
		remediationRequest.Status.OverallPhase = "processing"
		remediationRequest.Status.RemediationProcessingRef = &corev1.ObjectReference{
			Name:      remediationProcessing.Name,
			Namespace: remediationProcessing.Namespace,
		}
		Expect(k8sClient.Status().Update(ctx, remediationRequest)).To(Succeed())

		// THEN: AIAnalysis CRD should be created
		aiAnalysisName := remediationRequest.Name + "-aianalysis"
		aiAnalysis := &aianalysisv1alpha1.AIAnalysis{}
		Eventually(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{
				Name:      aiAnalysisName,
				Namespace: namespace,
			}, aiAnalysis)
		}, timeout, interval).Should(Succeed())

		// AND: AIAnalysis should have correct parent reference
		Expect(aiAnalysis.Spec.RemediationRequestRef).To(Equal(remediationRequest.Name))

		// AND: AIAnalysis should have self-contained signal context
		Expect(aiAnalysis.Spec.SignalType).To(Equal("prometheus-alert"))
		Expect(aiAnalysis.Spec.SignalContext).NotTo(BeEmpty())

		// AND: AIAnalysis should have owner reference for cascade deletion
		Expect(aiAnalysis.OwnerReferences).To(HaveLen(1))
		Expect(aiAnalysis.OwnerReferences[0].Name).To(Equal(remediationRequest.Name))
	})

	// ========================================
	// TEST 2: AIAnalysis includes enriched context from RemediationProcessing
	// Business Requirement: BR-ORCHESTRATION-001
	// ========================================
	It("should include enriched context from RemediationProcessing in AIAnalysis spec", func() {
		// GIVEN: A RemediationRequest with RemediationProcessing completed
		now := metav1.Now()
		remediationRequest := &remediationv1alpha1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-remediation-002",
				Namespace: namespace,
			},
			Spec: remediationv1alpha1.RemediationRequestSpec{
				SignalFingerprint: "def0123456789abcdef0123456789abcdef0123456789abcdef01234567896ab",
				SignalName:        "pod-crashloop",
				Severity:          "critical",
				Environment:       "prod",
				Priority:          "P0",
				SignalType:        "kubernetes-event",
				TargetType:        "kubernetes",
				FiringTime:        now,
				ReceivedTime:      now,
				Deduplication: remediationv1alpha1.DeduplicationInfo{
					IsDuplicate: false,
					FirstSeen:   now,
					LastSeen:    now,
				},
				SignalLabels: map[string]string{
					"namespace": "production",
					"pod":       "api-server-xyz",
				},
			},
		}
		Expect(k8sClient.Create(ctx, remediationRequest)).To(Succeed())

		// GIVEN: Controller creates RemediationProcessing in pending phase
		// WHEN: We update RemediationProcessing status to 'completed' with enriched context
		remediationProcessingName := remediationRequest.Name + "-processing"
		remediationProcessing := &remediationprocessingv1alpha1.RemediationProcessing{}
		
		// Wait for controller to create RemediationProcessing
		Eventually(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{
				Name:      remediationProcessingName,
				Namespace: namespace,
			}, remediationProcessing)
		}, timeout, interval).Should(Succeed())
		
		// Update RemediationProcessing status to 'completed' with enriched context
		remediationProcessing.Status.Phase = "completed"
		remediationProcessing.Status.ContextData = map[string]string{
			"cluster_state":      "degraded",
			"recent_deployments": "3",
			"metrics_available":  "true",
		}
		Expect(k8sClient.Status().Update(ctx, remediationProcessing)).To(Succeed())

		// WHEN: Controller processes the request
		// THEN: AIAnalysis should include enriched context
		aiAnalysisName := remediationRequest.Name + "-aianalysis"
		aiAnalysis := &aianalysisv1alpha1.AIAnalysis{}
		Eventually(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{
				Name:      aiAnalysisName,
				Namespace: namespace,
			}, aiAnalysis)
		}, timeout, interval).Should(Succeed())

		// Verify context data from RemediationProcessing is copied
		Expect(aiAnalysis.Spec.SignalContext).To(HaveKey("cluster_state"))
		Expect(aiAnalysis.Spec.SignalContext["cluster_state"]).To(Equal("degraded"))
		Expect(aiAnalysis.Spec.SignalContext).To(HaveKey("recent_deployments"))
	})

	// ========================================
	// TEST 3: Do NOT create AIAnalysis if RemediationProcessing is not completed
	// Business Requirement: BR-ORCHESTRATION-001
	// ========================================
	It("should NOT create AIAnalysis CRD when RemediationProcessing phase is 'enriching'", func() {
		// GIVEN: A RemediationRequest with RemediationProcessing still enriching
		now := metav1.Now()
		remediationRequest := &remediationv1alpha1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-remediation-003",
				Namespace: namespace,
			},
			Spec: remediationv1alpha1.RemediationRequestSpec{
				SignalFingerprint: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				SignalName:        "disk-full",
				Severity:          "warning",
				Environment:       "staging",
				Priority:          "P1",
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

		remediationProcessing := &remediationprocessingv1alpha1.RemediationProcessing{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remediationRequest.Name + "-processing",
				Namespace: namespace,
			},
			Spec: remediationprocessingv1alpha1.RemediationProcessingSpec{
				SignalFingerprint: remediationRequest.Spec.SignalFingerprint,
				SignalName:        remediationRequest.Spec.SignalName,
				Severity:          remediationRequest.Spec.Severity,
				Environment:       remediationRequest.Spec.Environment,
				Priority:          remediationRequest.Spec.Priority,
				SignalType:        remediationRequest.Spec.SignalType,
				TargetType:        remediationRequest.Spec.TargetType,
				ReceivedTime:      now,
				Deduplication: remediationprocessingv1alpha1.DeduplicationContext{
					FirstOccurrence: now,
					LastOccurrence:  now,
				},
			},
			Status: remediationprocessingv1alpha1.RemediationProcessingStatus{
				Phase: "enriching", // NOT completed
			},
		}
		Expect(k8sClient.Create(ctx, remediationProcessing)).To(Succeed())
		Expect(k8sClient.Status().Update(ctx, remediationProcessing)).To(Succeed())

		// Refetch to get latest resourceVersion (controller may have modified it)
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: remediationRequest.Name, Namespace: remediationRequest.Namespace}, remediationRequest)).To(Succeed())
		remediationRequest.Status.OverallPhase = "processing"
		remediationRequest.Status.RemediationProcessingRef = &corev1.ObjectReference{
			Name:      remediationProcessing.Name,
			Namespace: remediationProcessing.Namespace,
		}
		Expect(k8sClient.Status().Update(ctx, remediationRequest)).To(Succeed())

		// WHEN: Controller processes the request
		// THEN: AIAnalysis should NOT be created
		aiAnalysisName := remediationRequest.Name + "-aianalysis"
		aiAnalysis := &aianalysisv1alpha1.AIAnalysis{}
		Consistently(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{
				Name:      aiAnalysisName,
				Namespace: namespace,
			}, aiAnalysis)
		}, time.Second*2, interval).ShouldNot(Succeed())
	})
})

// ========================================
// TEST SUITE: Task 1.2 - WorkflowExecution CRD Creation
// Business Requirement: BR-ORCHESTRATION-002
// ========================================

var _ = Describe("RemediationRequest Controller - Task 1.2: WorkflowExecution CRD Creation", func() {
	const (
		timeout  = time.Second * 10
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
	// TEST 4: Create WorkflowExecution when AIAnalysis completes
	// Business Requirement: BR-ORCHESTRATION-002
	// ========================================
	It("should create WorkflowExecution CRD when AIAnalysis phase is 'completed'", func() {
		// GIVEN: A RemediationRequest with completed AIAnalysis
		now := metav1.Now()
		remediationRequest := &remediationv1alpha1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-remediation-004",
				Namespace: namespace,
			},
			Spec: remediationv1alpha1.RemediationRequestSpec{
				SignalFingerprint: "123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0",
				SignalName:        "memory-leak",
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

		// WHEN: AIAnalysis CRD exists and is marked as 'completed'
		aiAnalysis := &aianalysisv1alpha1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remediationRequest.Name + "-aianalysis",
				Namespace: namespace,
			},
			Spec: aianalysisv1alpha1.AIAnalysisSpec{
				RemediationRequestRef: remediationRequest.Name,
				SignalType:            remediationRequest.Spec.SignalType,
				SignalContext:         map[string]string{"test": "data"},
				LLMProvider:           "holmesgpt",
				LLMModel:              "gpt-4",
				MaxTokens:             4000,
				Temperature:           0.7,
			},
			Status: aianalysisv1alpha1.AIAnalysisStatus{
				Phase:             "Completed",
				RecommendedAction: "restart_pod",
				Confidence:        0.95,
			},
		}
		Expect(k8sClient.Create(ctx, aiAnalysis)).To(Succeed())
		
		// Refetch AIAnalysis to get created object (status is reset on create)
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: aiAnalysis.Name, Namespace: aiAnalysis.Namespace}, aiAnalysis)).To(Succeed())
		aiAnalysis.Status.Phase = "Completed"
		aiAnalysis.Status.RecommendedAction = "restart_pod"
		aiAnalysis.Status.Confidence = 0.95
		Expect(k8sClient.Status().Update(ctx, aiAnalysis)).To(Succeed())

		// Refetch and update status with retry (controller is also reconciling)
		Eventually(func() error {
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: remediationRequest.Name, Namespace: remediationRequest.Namespace}, remediationRequest); err != nil {
				return err
			}
			remediationRequest.Status.OverallPhase = "analyzing"
			remediationRequest.Status.AIAnalysisRef = &corev1.ObjectReference{
				Name:      aiAnalysis.Name,
				Namespace: aiAnalysis.Namespace,
			}
			return k8sClient.Status().Update(ctx, remediationRequest)
		}, time.Second*3, interval).Should(Succeed())

		// THEN: WorkflowExecution CRD should be created
		workflowName := remediationRequest.Name + "-workflow"
		workflow := &workflowexecutionv1alpha1.WorkflowExecution{}
		Eventually(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{
				Name:      workflowName,
				Namespace: namespace,
			}, workflow)
		}, timeout, interval).Should(Succeed())

		// AND: WorkflowExecution should have correct parent reference
		Expect(workflow.Spec.RemediationRequestRef.Name).To(Equal(remediationRequest.Name))
		Expect(workflow.Spec.RemediationRequestRef.UID).To(Equal(remediationRequest.UID))

		// AND: WorkflowExecution should include AI recommendations
		Expect(workflow.Spec.WorkflowDefinition).NotTo(BeNil())

		// AND: WorkflowExecution should have owner reference
		Expect(workflow.OwnerReferences).To(HaveLen(1))
		Expect(workflow.OwnerReferences[0].Name).To(Equal(remediationRequest.Name))
	})

	// ========================================
	// TEST 5: Do NOT create WorkflowExecution if AIAnalysis is not completed
	// Business Requirement: BR-ORCHESTRATION-002
	// ========================================
	It("should NOT create WorkflowExecution CRD when AIAnalysis phase is 'Analyzing'", func() {
		// GIVEN: A RemediationRequest with AIAnalysis still analyzing
		now := metav1.Now()
		remediationRequest := &remediationv1alpha1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-remediation-005",
				Namespace: namespace,
			},
			Spec: remediationv1alpha1.RemediationRequestSpec{
				SignalFingerprint: "23456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef01",
				SignalName:        "network-latency",
				Severity:          "warning",
				Environment:       "staging",
				Priority:          "P1",
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

		aiAnalysis := &aianalysisv1alpha1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      remediationRequest.Name + "-aianalysis",
				Namespace: namespace,
			},
			Spec: aianalysisv1alpha1.AIAnalysisSpec{
				RemediationRequestRef: remediationRequest.Name,
				SignalContext:         map[string]string{"test": "data"},
				LLMProvider:           "holmesgpt",
				LLMModel:              "gpt-4",
				MaxTokens:             4000,
				Temperature:           0.7,
			},
			Status: aianalysisv1alpha1.AIAnalysisStatus{
				Phase: "Analyzing", // NOT completed
			},
		}
		Expect(k8sClient.Create(ctx, aiAnalysis)).To(Succeed())
		
		// Refetch AIAnalysis to get created object (status is reset on create)
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: aiAnalysis.Name, Namespace: aiAnalysis.Namespace}, aiAnalysis)).To(Succeed())
		aiAnalysis.Status.Phase = "Analyzing"
		Expect(k8sClient.Status().Update(ctx, aiAnalysis)).To(Succeed())

		// Refetch and update status with retry (controller is also reconciling)
		Eventually(func() error {
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: remediationRequest.Name, Namespace: remediationRequest.Namespace}, remediationRequest); err != nil {
				return err
			}
			remediationRequest.Status.OverallPhase = "analyzing"
			remediationRequest.Status.AIAnalysisRef = &corev1.ObjectReference{
				Name:      aiAnalysis.Name,
				Namespace: aiAnalysis.Namespace,
			}
			return k8sClient.Status().Update(ctx, remediationRequest)
		}, time.Second*3, interval).Should(Succeed())

		// WHEN: Controller processes the request
		// THEN: WorkflowExecution should NOT be created
		workflowName := remediationRequest.Name + "-workflow"
		workflow := &workflowexecutionv1alpha1.WorkflowExecution{}
		Consistently(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{
				Name:      workflowName,
				Namespace: namespace,
			}, workflow)
		}, time.Second*2, interval).ShouldNot(Succeed())
	})
})
