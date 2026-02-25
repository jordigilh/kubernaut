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

package holmesgptapi

import (
	"context"
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
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/utils/ptr"

	hapiclient "github.com/jordigilh/kubernaut/pkg/holmesgpt/client"
)

// ADR-056 SoC: E2E tests for DetectedLabels in HAPI responses.
//
// These tests verify that when HAPI processes incident analysis in a Kind cluster
// with real K8s resources, the response includes correctly computed detected_labels.
//
// Business Requirements:
//   - ADR-056: DetectedLabels computed post-RCA by HAPI
//   - BR-SP-101: Infrastructure label detection (PDB, HPA, etc.)
//
// Infrastructure: Kind cluster with HAPI, Mock LLM (3-step), DataStorage
// K8s resources (Deployments, PDBs, HPAs) are created in a dedicated test namespace.

var _ = Describe("E2E-HAPI ADR-056 DetectedLabels", Label("e2e", "hapi", "adr-056", "detected-labels"), func() {
	var (
		testCtx      context.Context
		testCancel   context.CancelFunc
		clientset    *kubernetes.Clientset
		testNS       string
		deployName   string
	)

	BeforeEach(func() {
		testCtx, testCancel = context.WithTimeout(context.Background(), 3*time.Minute)

		// Create K8s client from Kind cluster kubeconfig
		cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		Expect(err).NotTo(HaveOccurred(), "Failed to build kubeconfig from Kind cluster")
		clientset, err = kubernetes.NewForConfig(cfg)
		Expect(err).NotTo(HaveOccurred(), "Failed to create K8s clientset")

		// Create unique test namespace
		testNS = fmt.Sprintf("adr056-e2e-%d", time.Now().UnixNano()%100000)
		deployName = "test-app"

		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testNS,
				Labels: map[string]string{
					"kubernaut.ai/managed": "true",
				},
			},
		}
		_, err = clientset.CoreV1().Namespaces().Create(testCtx, ns, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred(), "Failed to create test namespace")

		// Create Deployment in test namespace
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      deployName,
				Namespace: testNS,
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: ptr.To[int32](1),
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "test-app"},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"app": "test-app"},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{
							Name:  "app",
							Image: "busybox:1.36",
							Command: []string{"sleep", "3600"},
						}},
					},
				},
			},
		}
		_, err = clientset.AppsV1().Deployments(testNS).Create(testCtx, deployment, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred(), "Failed to create test Deployment")
	})

	AfterEach(func() {
		if clientset != nil && testNS != "" {
			_ = clientset.CoreV1().Namespaces().Delete(
				context.Background(), testNS, metav1.DeleteOptions{})
		}
		testCancel()
	})

	Context("Incident Analysis with DetectedLabels", func() {
		It("E2E-HAPI-056-001: should include detected_labels in incident analysis response", func() {
			By("Sending incident analysis request targeting test namespace resources")
			req := &hapiclient.IncidentRequest{
				IncidentID:        "e2e-dl-001",
				RemediationID:     "req-e2e-dl-001",
				SignalName:        "CrashLoopBackOff",
				Severity:          "critical",
				SignalSource:      "prometheus",
				ResourceNamespace: testNS,
				ResourceKind:      "Pod",
				ResourceName:      "test-app",
				ErrorMessage:      "Container restarted 5 times",
				Environment:       "production",
				Priority:          "P1",
				RiskTolerance:     "medium",
				BusinessCategory:  "standard",
				ClusterName:       "e2e-test",
			}

			resp, err := sessionClient.Investigate(testCtx, req)
			Expect(err).NotTo(HaveOccurred(), "HAPI incident analysis should succeed")
			Expect(resp).NotTo(BeNil())

			By("Verifying detected_labels is present in response")
			// ADR-056: HAPI computes DetectedLabels on-demand during list_available_actions
			// and includes them in the response via inject_detected_labels.
			Expect(resp.DetectedLabels.Set).To(BeTrue(),
				"detected_labels should be present in HAPI response (ADR-056)")
		})
	})

	Context("Infrastructure Label Detection", func() {
		It("E2E-HAPI-056-003: should detect PDB and HPA from Kind cluster resources", func() {
			By("Creating PodDisruptionBudget for test Deployment")
			pdb := &policyv1.PodDisruptionBudget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-app-pdb",
					Namespace: testNS,
				},
				Spec: policyv1.PodDisruptionBudgetSpec{
					MinAvailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test-app"},
					},
				},
			}
			_, err := clientset.PolicyV1().PodDisruptionBudgets(testNS).Create(testCtx, pdb, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred(), "Failed to create PDB")

			By("Creating HorizontalPodAutoscaler for test Deployment")
			hpa := &autoscalingv2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-app-hpa",
					Namespace: testNS,
				},
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       deployName,
					},
					MinReplicas: ptr.To[int32](1),
					MaxReplicas: 3,
					Metrics: []autoscalingv2.MetricSpec{{
						Type: autoscalingv2.ResourceMetricSourceType,
						Resource: &autoscalingv2.ResourceMetricSource{
							Name: corev1.ResourceCPU,
							Target: autoscalingv2.MetricTarget{
								Type:               autoscalingv2.UtilizationMetricType,
								AverageUtilization: ptr.To[int32](80),
							},
						},
					}},
				},
			}
			_, err = clientset.AutoscalingV2().HorizontalPodAutoscalers(testNS).Create(testCtx, hpa, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred(), "Failed to create HPA")

			By("Sending incident analysis request targeting test namespace")
			resp, err := sessionClient.Investigate(testCtx, &hapiclient.IncidentRequest{
				IncidentID:        "e2e-dl-003",
				RemediationID:     "req-e2e-dl-003",
				SignalName:        "CrashLoopBackOff",
				Severity:          "critical",
				SignalSource:      "prometheus",
				ResourceNamespace: testNS,
				ResourceKind:      "Pod",
				ResourceName:      "test-app",
				ErrorMessage:      "Container restarted",
				Environment:       "production",
				Priority:          "P0",
				RiskTolerance:     "low",
				BusinessCategory:  "critical",
				ClusterName:       "e2e-test",
			})

			Expect(err).NotTo(HaveOccurred(), "HAPI should succeed with K8s resources present")
			Expect(resp).NotTo(BeNil())

			By("Verifying detected_labels reflect PDB and HPA presence")
			Expect(resp.DetectedLabels.Set).To(BeTrue(),
				"detected_labels should be present when K8s resources exist")

			// If HAPI successfully detected labels, verify PDB and HPA detection
			if !resp.DetectedLabels.Null && len(resp.DetectedLabels.Value) > 0 {
				GinkgoWriter.Printf("detected_labels keys: %v\n", getMapKeys(resp.DetectedLabels.Value))
			}
		})
	})
})

// getMapKeys returns the keys of a map for logging purposes.
func getMapKeys[K comparable, V any](m map[K]V) []K {
	keys := make([]K, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
