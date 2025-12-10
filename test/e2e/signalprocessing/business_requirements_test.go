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

// Package signalprocessing_e2e contains E2E/BR tests for SignalProcessing business requirements.
// These tests validate business value delivery - SLAs, efficiency, reliability.
//
// Defense-in-Depth Strategy (per 03-testing-strategy.mdc):
// - Unit tests (70%+): Business logic in isolation (test/unit/signalprocessing/)
// - Integration tests (>50%): CRD coordination (test/integration/signalprocessing/)
// - E2E/BR tests (10-15%): Complete workflow validation (this directory)
//
// TDD Phase: RED - Tests define expected business behavior
// These tests will FAIL until controller implementation is complete (GREEN phase)
//
// Purpose: Validate that SignalProcessing delivers business value as specified
// Audience: Business stakeholders + developers
// Execution: make test-e2e-signalprocessing
//
// Business Requirements Validated:
// - BR-SP-051: Environment classification from namespace labels
// - BR-SP-070: Priority assignment (P0-P3) based on environment + severity
// - BR-SP-100: Owner chain traversal for enrichment
// - BR-SP-101: Detected labels (PDB, HPA, NetworkPolicy)
// - BR-SP-102: CustomLabels from Rego policies
//
// NOTE: These tests duplicate some integration test scenarios intentionally
// for defense-in-depth coverage. E2E tests run against real Kind cluster
// while integration tests use ENVTEST.
package signalprocessing_e2e

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// BR-SP-070: Priority Assignment
// BUSINESS VALUE: Operations team gets correct priority for alert triage
// STAKEHOLDER: On-call engineers need accurate priority for response decisions
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

var _ = Describe("BR-SP-070: Priority Assignment Delivers Correct Business Outcomes", func() {

	Context("Production Environment Prioritization", func() {
		var testNs string

		BeforeEach(func() {
			testNs = fmt.Sprintf("e2e-prod-%d", time.Now().UnixNano())
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: testNs,
					Labels: map[string]string{
						"kubernaut.ai/environment": "production",
					},
				},
			}
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		})

		AfterEach(func() {
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNs}}
			_ = k8sClient.Delete(ctx, ns)
		})

		// TDD RED: This test will FAIL until controller assigns P0 priority
		It("BR-SP-070: should assign P0 to production critical alerts (highest urgency)", func() {
			By("Creating SignalProcessing CR for production critical alert")
			sp := &signalprocessingv1alpha1.SignalProcessing{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-priority-p0",
					Namespace: testNs,
				},
				Spec: signalprocessingv1alpha1.SignalProcessingSpec{
					Signal: signalprocessingv1alpha1.SignalData{
						Fingerprint:  "a0b1c2d3e4f5a6b7c8d9e0f1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1",
						Name:         "HighCPU",
						Severity:     "critical",
						Type:         "prometheus",
						TargetType:   "kubernetes",
						ReceivedTime: metav1.Now(),
						TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
							Kind:      "Pod",
							Name:      "api-server-xyz",
							Namespace: testNs,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, sp)).To(Succeed())

			By("Waiting for priority assignment")
			// TDD RED: Controller stub won't set this - test will FAIL
			Eventually(func() string {
				var updated signalprocessingv1alpha1.SignalProcessing
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(sp), &updated); err != nil {
					return ""
				}
				return updated.Status.PriorityAssignment.Priority
			}, timeout, interval).Should(Equal("P0"))

			By("Verifying business outcome: production critical = highest urgency")
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(sp), &final)).To(Succeed())
			Expect(final.Status.PriorityAssignment.Priority).To(Equal("P0"))
			Expect(final.Status.PriorityAssignment.Confidence).To(BeNumerically(">=", 0.9))
		})

		// TDD RED: This test will FAIL until controller assigns P1 priority
		It("BR-SP-070: should assign P1 to production warning alerts (high urgency)", func() {
			By("Creating SignalProcessing CR for production warning alert")
			sp := &signalprocessingv1alpha1.SignalProcessing{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-priority-p1",
					Namespace: testNs,
				},
				Spec: signalprocessingv1alpha1.SignalProcessingSpec{
					Signal: signalprocessingv1alpha1.SignalData{
						Fingerprint:  "b1c2d3e4f5a6b7c8d9e0f1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2",
						Name:         "MemoryPressure",
						Severity:     "warning",
						Type:         "prometheus",
						TargetType:   "kubernetes",
						ReceivedTime: metav1.Now(),
						TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
							Kind:      "Pod",
							Name:      "worker-abc",
							Namespace: testNs,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, sp)).To(Succeed())

			By("Waiting for priority assignment")
			Eventually(func() string {
				var updated signalprocessingv1alpha1.SignalProcessing
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(sp), &updated); err != nil {
					return ""
				}
				return updated.Status.PriorityAssignment.Priority
			}, timeout, interval).Should(Equal("P1"))
		})
	})

	Context("Non-Production Environment Prioritization", func() {
		var stagingNs, devNs string

		BeforeEach(func() {
			stagingNs = fmt.Sprintf("e2e-staging-%d", time.Now().UnixNano())
			devNs = fmt.Sprintf("e2e-dev-%d", time.Now().UnixNano())

			// Create staging namespace
			Expect(k8sClient.Create(ctx, &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:   stagingNs,
					Labels: map[string]string{"kubernaut.ai/environment": "staging"},
				},
			})).To(Succeed())

			// Create development namespace
			Expect(k8sClient.Create(ctx, &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:   devNs,
					Labels: map[string]string{"kubernaut.ai/environment": "development"},
				},
			})).To(Succeed())
		})

		AfterEach(func() {
			_ = k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: stagingNs}})
			_ = k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: devNs}})
		})

		// TDD RED: This test will FAIL until controller assigns P2 priority
		It("BR-SP-070: should assign P2 to staging critical alerts (medium urgency)", func() {
			sp := &signalprocessingv1alpha1.SignalProcessing{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-priority-p2",
					Namespace: stagingNs,
				},
				Spec: signalprocessingv1alpha1.SignalProcessingSpec{
					Signal: signalprocessingv1alpha1.SignalData{
						Fingerprint:  "c2d3e4f5a6b7c8d9e0f1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3",
						Name:         "StagingCritical",
						Severity:     "critical",
						Type:         "prometheus",
						TargetType:   "kubernetes",
						ReceivedTime: metav1.Now(),
						TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
							Kind:      "Pod",
							Name:      "staging-pod",
							Namespace: stagingNs,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, sp)).To(Succeed())

			Eventually(func() string {
				var updated signalprocessingv1alpha1.SignalProcessing
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(sp), &updated); err != nil {
					return ""
				}
				return updated.Status.PriorityAssignment.Priority
			}, timeout, interval).Should(Equal("P2"))
		})

		// TDD RED: This test will FAIL until controller assigns P3 priority
		It("BR-SP-070: should assign P3 to development alerts (low urgency)", func() {
			sp := &signalprocessingv1alpha1.SignalProcessing{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-priority-p3",
					Namespace: devNs,
				},
				Spec: signalprocessingv1alpha1.SignalProcessingSpec{
					Signal: signalprocessingv1alpha1.SignalData{
						Fingerprint:  "d3e4f5a6b7c8d9e0f1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4",
						Name:         "DevInfo",
						Severity:     "info",
						Type:         "prometheus",
						TargetType:   "kubernetes",
						ReceivedTime: metav1.Now(),
						TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
							Kind:      "Pod",
							Name:      "dev-pod",
							Namespace: devNs,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, sp)).To(Succeed())

			Eventually(func() string {
				var updated signalprocessingv1alpha1.SignalProcessing
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(sp), &updated); err != nil {
					return ""
				}
				return updated.Status.PriorityAssignment.Priority
			}, timeout, interval).Should(Equal("P3"))
		})
	})
})

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// BR-SP-051: Environment Classification
// BUSINESS VALUE: Alerts are routed to correct team based on environment
// STAKEHOLDER: Operations team needs environment context for escalation
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

var _ = Describe("BR-SP-051: Environment Classification Enables Correct Routing", func() {
	var testNs string

	BeforeEach(func() {
		testNs = fmt.Sprintf("e2e-env-%d", time.Now().UnixNano())
	})

	AfterEach(func() {
		_ = k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNs}})
	})

	// TDD RED: This test will FAIL until controller classifies environment
	It("BR-SP-051: should classify production from namespace label with high confidence", func() {
		By("Creating namespace with production label")
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testNs,
				Labels: map[string]string{
					"kubernaut.ai/environment": "production",
				},
			},
		}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())

		By("Creating SignalProcessing CR")
		sp := &signalprocessingv1alpha1.SignalProcessing{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "e2e-env-prod",
				Namespace: testNs,
			},
			Spec: signalprocessingv1alpha1.SignalProcessingSpec{
				Signal: signalprocessingv1alpha1.SignalData{
					Fingerprint:  "e4f5a6b7c8d9e0f1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5",
					Name:         "TestAlert",
					Severity:     "warning",
					Type:         "prometheus",
					TargetType:   "kubernetes",
					ReceivedTime: metav1.Now(),
					TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
						Kind:      "Pod",
						Name:      "test-pod",
						Namespace: testNs,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, sp)).To(Succeed())

		By("Waiting for environment classification")
		Eventually(func() string {
			var updated signalprocessingv1alpha1.SignalProcessing
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(sp), &updated); err != nil {
				return ""
			}
			return updated.Status.EnvironmentClassification.Environment
		}, timeout, interval).Should(Equal("production"))

		By("Verifying high confidence classification")
		var final signalprocessingv1alpha1.SignalProcessing
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(sp), &final)).To(Succeed())
		Expect(final.Status.EnvironmentClassification.Confidence).To(BeNumerically(">=", 0.95))
	})

	// TDD RED: This test will FAIL until controller defaults to unknown
	It("BR-SP-053: should default to unknown for unclassifiable namespaces", func() {
		By("Creating namespace without environment label")
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testNs,
				// No kubernaut.ai/environment label
			},
		}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())

		By("Creating SignalProcessing CR")
		sp := &signalprocessingv1alpha1.SignalProcessing{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "e2e-env-unknown",
				Namespace: testNs,
			},
			Spec: signalprocessingv1alpha1.SignalProcessingSpec{
				Signal: signalprocessingv1alpha1.SignalData{
					Fingerprint:  "f5a6b7c8d9e0f1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6",
					Name:         "UnclassifiedAlert",
					Severity:     "warning",
					Type:         "prometheus",
					TargetType:   "kubernetes",
					ReceivedTime: metav1.Now(),
					TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
						Kind:      "Pod",
						Name:      "unclassified-pod",
						Namespace: testNs,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, sp)).To(Succeed())

		By("Waiting for default environment classification")
		Eventually(func() string {
			var updated signalprocessingv1alpha1.SignalProcessing
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(sp), &updated); err != nil {
				return ""
			}
			return updated.Status.EnvironmentClassification.Environment
		}, timeout, interval).Should(Equal("unknown"))
	})
})

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// BR-SP-100: Owner Chain Traversal
// BUSINESS VALUE: AI analysis can identify deployment-level issues from pod alerts
// STAKEHOLDER: AI Analysis service needs owner context for recommendations
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

var _ = Describe("BR-SP-100: Owner Chain Enables Root Cause Analysis", func() {
	var testNs string

	BeforeEach(func() {
		testNs = fmt.Sprintf("e2e-owner-%d", time.Now().UnixNano())
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: testNs},
		}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
	})

	AfterEach(func() {
		_ = k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNs}})
	})

	// TDD RED: This test will FAIL until controller builds owner chain
	It("BR-SP-100: should build complete owner chain for accurate root cause identification", func() {
		By("Creating Deployment with Pod")
		replicas := int32(1)
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "api-deployment",
				Namespace: testNs,
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "api"},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"app": "api"},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{
							Name:  "api",
							Image: "nginx:latest",
						}},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, deployment)).To(Succeed())

		By("Waiting for Pod to be created by Deployment")
		var podName string
		Eventually(func() bool {
			pods := &corev1.PodList{}
			if err := k8sClient.List(ctx, pods, client.InNamespace(testNs)); err != nil {
				return false
			}
			for _, pod := range pods.Items {
				if len(pod.OwnerReferences) > 0 {
					podName = pod.Name
					return true
				}
			}
			return false
		}, timeout, interval).Should(BeTrue())

		By("Creating SignalProcessing CR targeting the Pod")
		sp := &signalprocessingv1alpha1.SignalProcessing{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "e2e-owner-chain",
				Namespace: testNs,
			},
			Spec: signalprocessingv1alpha1.SignalProcessingSpec{
				Signal: signalprocessingv1alpha1.SignalData{
					Fingerprint:  "a6b7c8d9e0f1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7",
					Name:         "PodAlert",
					Severity:     "critical",
					Type:         "prometheus",
					TargetType:   "kubernetes",
					ReceivedTime: metav1.Now(),
					TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
						Kind:      "Pod",
						Name:      podName,
						Namespace: testNs,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, sp)).To(Succeed())

		By("Waiting for owner chain to be populated")
		Eventually(func() int {
			var updated signalprocessingv1alpha1.SignalProcessing
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(sp), &updated); err != nil {
				return 0
			}
			return len(updated.Status.KubernetesContext.OwnerChain)
		}, timeout, interval).Should(BeNumerically(">=", 2))

		By("Verifying owner chain includes ReplicaSet and Deployment")
		var final signalprocessingv1alpha1.SignalProcessing
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(sp), &final)).To(Succeed())
		ownerKinds := make([]string, len(final.Status.KubernetesContext.OwnerChain))
		for i, owner := range final.Status.KubernetesContext.OwnerChain {
			ownerKinds[i] = owner.Kind
		}
		Expect(ownerKinds).To(ContainElement("ReplicaSet"))
		Expect(ownerKinds).To(ContainElement("Deployment"))
	})
})

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// BR-SP-101: Detected Labels (PDB, HPA)
// BUSINESS VALUE: Remediation workflows respect cluster safety features
// STAKEHOLDER: Platform team needs remediation to honor PDB/HPA
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

var _ = Describe("BR-SP-101: Detected Labels Enable Safe Remediation Decisions", func() {
	var testNs string

	BeforeEach(func() {
		testNs = fmt.Sprintf("e2e-detect-%d", time.Now().UnixNano())
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: testNs},
		}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
	})

	AfterEach(func() {
		_ = k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNs}})
	})

	// TDD RED: This test will FAIL until controller detects PDB
	It("BR-SP-101: should detect PDB protection to prevent unsafe pod deletion", func() {
		By("Creating Pod with labels")
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "protected-pod",
				Namespace: testNs,
				Labels:    map[string]string{"app": "protected"},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{
					Name:  "main",
					Image: "nginx:latest",
				}},
			},
		}
		Expect(k8sClient.Create(ctx, pod)).To(Succeed())

		By("Creating PDB matching pod labels")
		minAvailable := intstr.FromInt(1)
		pdb := &policyv1.PodDisruptionBudget{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "protected-pdb",
				Namespace: testNs,
			},
			Spec: policyv1.PodDisruptionBudgetSpec{
				MinAvailable: &minAvailable,
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "protected"},
				},
			},
		}
		Expect(k8sClient.Create(ctx, pdb)).To(Succeed())

		By("Creating SignalProcessing CR")
		sp := &signalprocessingv1alpha1.SignalProcessing{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "e2e-pdb-detect",
				Namespace: testNs,
			},
			Spec: signalprocessingv1alpha1.SignalProcessingSpec{
				Signal: signalprocessingv1alpha1.SignalData{
					Fingerprint:  "b7c8d9e0f1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c8",
					Name:         "PDBAlert",
					Severity:     "critical",
					Type:         "prometheus",
					TargetType:   "kubernetes",
					ReceivedTime: metav1.Now(),
					TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
						Kind:      "Pod",
						Name:      "protected-pod",
						Namespace: testNs,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, sp)).To(Succeed())

		By("Waiting for PDB detection")
		Eventually(func() bool {
			var updated signalprocessingv1alpha1.SignalProcessing
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(sp), &updated); err != nil {
				return false
			}
			if updated.Status.KubernetesContext == nil || updated.Status.KubernetesContext.DetectedLabels == nil {
				return false
			}
			return updated.Status.KubernetesContext.DetectedLabels.HasPDB
		}, timeout, interval).Should(BeTrue())
	})

	// TDD RED: This test will FAIL until controller detects HPA
	It("BR-SP-101: should detect HPA to prevent conflicting scale operations", func() {
		By("Creating Deployment")
		replicas := int32(1)
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "scalable-deployment",
				Namespace: testNs,
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "scalable"},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"app": "scalable"},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{
							Name:  "main",
							Image: "nginx:latest",
						}},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, deployment)).To(Succeed())

		By("Creating HPA targeting deployment")
		minReplicas := int32(1)
		hpa := &autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "scalable-hpa",
				Namespace: testNs,
			},
			Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
				ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "scalable-deployment",
				},
				MinReplicas: &minReplicas,
				MaxReplicas: 5,
			},
		}
		Expect(k8sClient.Create(ctx, hpa)).To(Succeed())

		By("Waiting for Pod to be created")
		var podName string
		Eventually(func() bool {
			pods := &corev1.PodList{}
			if err := k8sClient.List(ctx, pods, client.InNamespace(testNs)); err != nil {
				return false
			}
			for _, pod := range pods.Items {
				if pod.Labels["app"] == "scalable" {
					podName = pod.Name
					return true
				}
			}
			return false
		}, timeout, interval).Should(BeTrue())

		By("Creating SignalProcessing CR targeting the Pod")
		sp := &signalprocessingv1alpha1.SignalProcessing{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "e2e-hpa-detect",
				Namespace: testNs,
			},
			Spec: signalprocessingv1alpha1.SignalProcessingSpec{
				Signal: signalprocessingv1alpha1.SignalData{
					Fingerprint:  "c8d9e0f1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c8d9",
					Name:         "HPAAlert",
					Severity:     "warning",
					Type:         "prometheus",
					TargetType:   "kubernetes",
					ReceivedTime: metav1.Now(),
					TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
						Kind:      "Pod",
						Name:      podName,
						Namespace: testNs,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, sp)).To(Succeed())

		By("Waiting for HPA detection")
		Eventually(func() bool {
			var updated signalprocessingv1alpha1.SignalProcessing
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(sp), &updated); err != nil {
				return false
			}
			if updated.Status.KubernetesContext == nil || updated.Status.KubernetesContext.DetectedLabels == nil {
				return false
			}
			return updated.Status.KubernetesContext.DetectedLabels.HasHPA
		}, timeout, interval).Should(BeTrue())
	})
})

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// BR-SP-102: CustomLabels from Rego
// BUSINESS VALUE: Customer-defined labels enable custom alert routing
// STAKEHOLDER: Platform customers need custom classification rules
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

var _ = Describe("BR-SP-102: CustomLabels Enable Business-Specific Routing", func() {
	var testNs string

	BeforeEach(func() {
		testNs = fmt.Sprintf("e2e-custom-%d", time.Now().UnixNano())
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testNs,
				Labels: map[string]string{
					"kubernaut.ai/team": "payments",
				},
			},
		}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
	})

	AfterEach(func() {
		_ = k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNs}})
	})

	// TDD RED: This test will FAIL until controller extracts custom labels
	It("BR-SP-102: should extract custom labels from Rego policies", func() {
		By("Creating SignalProcessing CR")
		sp := &signalprocessingv1alpha1.SignalProcessing{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "e2e-custom-labels",
				Namespace: testNs,
			},
			Spec: signalprocessingv1alpha1.SignalProcessingSpec{
				Signal: signalprocessingv1alpha1.SignalData{
					Fingerprint:  "d9e0f1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c8d9e0",
					Name:         "PaymentsAlert",
					Severity:     "critical",
					Type:         "prometheus",
					TargetType:   "kubernetes",
					ReceivedTime: metav1.Now(),
					TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
						Kind:      "Pod",
						Name:      "payments-api",
						Namespace: testNs,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, sp)).To(Succeed())

		By("Waiting for custom labels to be populated")
		Eventually(func() int {
			var updated signalprocessingv1alpha1.SignalProcessing
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(sp), &updated); err != nil {
				return 0
			}
			return len(updated.Status.KubernetesContext.CustomLabels)
		}, timeout, interval).Should(BeNumerically(">", 0))

		By("Verifying team label was extracted")
		var final signalprocessingv1alpha1.SignalProcessing
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(sp), &final)).To(Succeed())
		Expect(final.Status.KubernetesContext.CustomLabels).To(HaveKey("team"))
	})
})
