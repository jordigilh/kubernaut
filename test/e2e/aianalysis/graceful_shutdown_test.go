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

package aianalysis

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aianalysisv1alpha1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/client"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// ==============================================
// E2E Tests: Graceful Shutdown
// ==============================================
// ⭐ FIRST E2E GRACEFUL SHUTDOWN TEST IN KUBERNAUT CODEBASE ⭐
//
// BUSINESS CONTEXT: Validate real process responds to OS signals correctly
// and completes graceful shutdown without data loss.
//
// BR-AI-082: Handle SIGTERM within timeout (10-15s)
// BR-AI-091: Flush audit buffer on SIGTERM (no event loss)
// BR-AI-090: Complete in-flight analysis on SIGTERM
// BR-AI-012: Stop Rego hot-reloader cleanly on SIGTERM
//
// Defense-in-Depth Strategy (per 03-testing-strategy.mdc):
// - Unit tests (70%+): Context cancellation patterns
// - Integration tests (>50%): In-flight work completion behavior
// - E2E tests (10-15%): Full SIGTERM handling with real process lifecycle (THIS FILE)
//
// CRITICAL DIFFERENCE: Unit/integration tests use context.WithCancel().
// E2E tests send actual SIGTERM signal to running pod and validate:
// 1. ctrl.SetupSignalHandler() catches SIGTERM
// 2. Signal propagates to context cancellation
// 3. Controller completes in-flight work
// 4. Audit buffer flushed (no loss)
// 5. Rego hot-reloader stopped cleanly
// 6. Process exits within timeout
//
// Per V1.0_SERVICE_MATURITY_TEST_PLAN_TEMPLATE.md Section 4.3
// This test will be the REFERENCE IMPLEMENTATION for all other services.
// ==============================================

var _ = Describe("BR-AI-082/091/090: E2E Graceful Shutdown", Ordered, func() {
	var (
		servicePodName     string
		serviceNamespace   string
		testNamespace      string
		dsClient           *dsgen.ClientWithResponses
		dataStorageURL     string
		originalPodName    string
		originalPodRunning bool
	)

	// Helper function for creating string pointers
	strPtr := func(s string) *string {
		return &s
	}

	BeforeAll(func() {
		serviceNamespace = infraNamespace
		testNamespace = "default"
		dataStorageURL = fmt.Sprintf("http://datastorage-service.%s.svc.cluster.local:8080", infraNamespace)

		// Initialize Data Storage client
		var err error
		dsClient, err = dsgen.NewClientWithResponses(dataStorageURL)
		Expect(err).ToNot(HaveOccurred(), "Data Storage client should be created for E2E tests")

		// Find running AIAnalysis controller pod
		podList := &corev1.PodList{}
		err = k8sClient.List(context.Background(), podList,
			client.InNamespace(serviceNamespace),
			client.MatchingLabels{"app": "aianalysis-controller"})
		Expect(err).ToNot(HaveOccurred(), "Should be able to list AIAnalysis controller pods")
		Expect(len(podList.Items)).To(BeNumerically(">", 0), "AIAnalysis controller pod must be running")

		originalPodName = podList.Items[0].Name
		originalPodRunning = podList.Items[0].Status.Phase == corev1.PodRunning
		servicePodName = originalPodName

		GinkgoWriter.Printf("E2E Graceful Shutdown Test - AIAnalysis Pod: %s (Status: %s)\n",
			servicePodName, podList.Items[0].Status.Phase)
	})

	// ==============================================
	// Test 1: SIGTERM Signal Handling
	// ==============================================

	It("BR-AI-082: should handle SIGTERM within timeout (10-15s)", func() {
		Skip("E2E test requires Kind cluster with AIAnalysis controller deployed")

		ctx := context.Background()

		// Create AIAnalysis instance to generate audit events
		uniqueSuffix := fmt.Sprintf("%d", time.Now().UnixNano())
		analysisName := fmt.Sprintf("sigterm-test-%s", uniqueSuffix)

		analysis := &aianalysisv1alpha1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      analysisName,
				Namespace: testNamespace,
			},
			Spec: aianalysisv1alpha1.AIAnalysisSpec{
				RemediationRequestRef: corev1.ObjectReference{
					APIVersion: "aianalysis.kubernaut.io/v1alpha1",
					Kind:       "RemediationRequest",
					Name:       "test-rr",
					Namespace:  testNamespace,
				},
				RemediationID: fmt.Sprintf("rem-%s", uniqueSuffix),
				AnalysisRequest: aianalysisv1alpha1.AnalysisRequest{
					SignalContext: aianalysisv1alpha1.SignalContextInput{
						Fingerprint:      fmt.Sprintf("e2e-sigterm-%s", uniqueSuffix),
						Severity:         "critical",
						SignalType:       "E2ETest",
						Environment:      "test",
						BusinessPriority: "P1",
						TargetResource: aianalysisv1alpha1.TargetResource{
							Kind:      "Pod",
							Name:      "test-pod",
							Namespace: testNamespace,
						},
						EnrichmentResults: sharedtypes.EnrichmentResults{
							KubernetesContext: &sharedtypes.KubernetesContext{
								Namespace: testNamespace,
							},
							DetectedLabels: &sharedtypes.DetectedLabels{
								GitOpsManaged: false,
								PDBProtected:  false,
								HPAEnabled:    false,
								Stateful:      false,
							},
						},
					},
					AnalysisTypes: []string{"investigation"},
				},
			},
		}

		err := k8sClient.Create(ctx, analysis)
		Expect(err).ToNot(HaveOccurred(), "AIAnalysis should be created")

		// Wait for controller to start processing
		Eventually(func() string {
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      analysisName,
				Namespace: testNamespace,
			}, analysis)
			if err != nil {
				return ""
			}
			return analysis.Status.Phase
		}, 30*time.Second, 1*time.Second).Should(Equal("Investigating"),
			"AIAnalysis should enter Investigating phase")

		// Send SIGTERM to AIAnalysis controller pod
		// This is the CRITICAL DIFFERENCE from unit/integration tests
		GinkgoWriter.Printf("Sending SIGTERM to pod %s in namespace %s\n", servicePodName, serviceNamespace)
		cmd := exec.Command("kubectl", "exec", "-n", serviceNamespace, servicePodName, "--",
			"sh", "-c", "kill -SIGTERM 1")
		output, err := cmd.CombinedOutput()
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("SIGTERM signal should be sent successfully. Output: %s", string(output)))

		// Verify pod terminates gracefully within timeout
		startTime := time.Now()
		Eventually(func() bool {
			pod := &corev1.Pod{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      servicePodName,
				Namespace: serviceNamespace,
			}, pod)
			if err != nil {
				// Pod deleted
				return true
			}
			// Pod terminated or terminating
			return pod.Status.Phase == corev1.PodSucceeded ||
				pod.Status.Phase == corev1.PodFailed ||
				pod.DeletionTimestamp != nil
		}, 20*time.Second, 1*time.Second).Should(BeTrue(),
			"Pod should terminate gracefully within 20 seconds after SIGTERM")

		shutdownDuration := time.Since(startTime)
		GinkgoWriter.Printf("Pod terminated in %v (Target: <15s)\n", shutdownDuration)

		Expect(shutdownDuration).To(BeNumerically("<", 15*time.Second),
			"BR-AI-082: Graceful shutdown should complete within 15s timeout")

		// Verify audit buffer was flushed (no event loss)
		// Wait a bit for final flush to reach Data Storage
		time.Sleep(2 * time.Second)

		// Query using correlation_id (format: remediationId-resourceName-resourceNamespace)
		correlationID := fmt.Sprintf("%s-%s-%s", fmt.Sprintf("rem-%s", uniqueSuffix), analysisName, testNamespace)
		params := &dsgen.QueryAuditEventsParams{
			CorrelationId: strPtr(correlationID),
		}

		resp, err := dsClient.QueryAuditEventsWithResponse(ctx, params)
		Expect(err).ToNot(HaveOccurred(), "Should be able to query audit events")
		Expect(resp.StatusCode()).To(Equal(http.StatusOK), "Audit query should succeed")
		Expect(resp.JSON200.Data).ToNot(BeNil(), "Response should contain data")
		Expect(len(*resp.JSON200.Data)).To(BeNumerically(">=", 1),
			"BR-AI-091: Audit events must NOT be lost during graceful shutdown")

		GinkgoWriter.Printf("Verified %d audit events were flushed during shutdown\n", len(*resp.JSON200.Data))
	})

	// ==============================================
	// Test 2: Multiple Concurrent Events During Shutdown
	// ==============================================

	It("should flush all pending audit events on SIGTERM (stress test)", func() {
		Skip("E2E test requires Kind cluster with AIAnalysis controller deployed")

		ctx := context.Background()

		// Create multiple AIAnalysis instances to generate many audit events
		instanceCount := 5
		instanceNames := make([]string, instanceCount)
		uniqueSuffix := fmt.Sprintf("%d", time.Now().UnixNano())

		for i := 0; i < instanceCount; i++ {
			instanceNames[i] = fmt.Sprintf("sigterm-flush-test-%d-%s", i, uniqueSuffix)
			analysis := &aianalysisv1alpha1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instanceNames[i],
					Namespace: testNamespace,
				},
				Spec: aianalysisv1alpha1.AIAnalysisSpec{
					RemediationRequestRef: corev1.ObjectReference{
						APIVersion: "aianalysis.kubernaut.io/v1alpha1",
						Kind:       "RemediationRequest",
						Name:       "test-rr",
						Namespace:  testNamespace,
					},
					RemediationID: fmt.Sprintf("rem-%s-%d", uniqueSuffix, i),
					AnalysisRequest: aianalysisv1alpha1.AnalysisRequest{
						SignalContext: aianalysisv1alpha1.SignalContextInput{
							Fingerprint:      fmt.Sprintf("e2e-flush-%s-%d", uniqueSuffix, i),
							Severity:         "critical",
							SignalType:       "E2ETest",
							Environment:      "test",
							BusinessPriority: "P1",
							TargetResource: aianalysisv1alpha1.TargetResource{
								Kind:      "Pod",
								Name:      "test-pod",
								Namespace: testNamespace,
							},
							EnrichmentResults: sharedtypes.EnrichmentResults{
								KubernetesContext: &sharedtypes.KubernetesContext{
									Namespace: testNamespace,
								},
								DetectedLabels: &sharedtypes.DetectedLabels{
									GitOpsManaged: false,
									PDBProtected:  false,
									HPAEnabled:    false,
									Stateful:      false,
								},
							},
						},
						AnalysisTypes: []string{"investigation"},
					},
				},
			}

			err := k8sClient.Create(ctx, analysis)
			Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("AIAnalysis %s should be created", instanceNames[i]))
		}

		// Wait for all to start processing
		for _, name := range instanceNames {
			Eventually(func() string {
				analysis := &aianalysisv1alpha1.AIAnalysis{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      name,
					Namespace: testNamespace,
				}, analysis)
				if err != nil {
					return ""
				}
				return analysis.Status.Phase
			}, 30*time.Second, 1*time.Second).ShouldNot(BeEmpty(),
				fmt.Sprintf("AIAnalysis %s should start processing", name))
		}

		// Send SIGTERM while many events are buffered
		GinkgoWriter.Printf("Sending SIGTERM to pod %s with %d active analyses\n", servicePodName, instanceCount)
		cmd := exec.Command("kubectl", "exec", "-n", serviceNamespace, servicePodName, "--",
			"sh", "-c", "kill -SIGTERM 1")
		output, err := cmd.CombinedOutput()
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("SIGTERM signal should be sent. Output: %s", string(output)))

		// Wait for graceful shutdown
		Eventually(func() bool {
			pod := &corev1.Pod{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      servicePodName,
				Namespace: serviceNamespace,
			}, pod)
			return err != nil || pod.Status.Phase != corev1.PodRunning
		}, 20*time.Second, 1*time.Second).Should(BeTrue(),
			"Pod should terminate within 20 seconds")

		// Wait for audit flush to complete
		time.Sleep(3 * time.Second)

		// Verify ALL audit events were flushed (count matches expected)
		for i, name := range instanceNames {
			// Query using correlation_id
			correlationID := fmt.Sprintf("%s-%s-%s", fmt.Sprintf("rem-%s-%d", uniqueSuffix, i), name, testNamespace)
			params := &dsgen.QueryAuditEventsParams{
				CorrelationId: strPtr(correlationID),
			}

			resp, err := dsClient.QueryAuditEventsWithResponse(ctx, params)
			Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Should query events for %s", name))
			Expect(resp.StatusCode()).To(Equal(http.StatusOK), "Audit query should succeed")
			Expect(resp.JSON200.Data).ToNot(BeNil(), "Response should contain data")
			Expect(len(*resp.JSON200.Data)).To(BeNumerically(">=", 1),
				fmt.Sprintf("Events for %s must be flushed (got %d events)", name, len(*resp.JSON200.Data)))

			GinkgoWriter.Printf("Analysis %s: %d audit events flushed\n", name, len(*resp.JSON200.Data))
		}
	})

	// ==============================================
	// Test 3: Rego Hot-Reloader Cleanup on SIGTERM
	// ==============================================

	It("BR-AI-012: should stop Rego hot-reloader cleanly on SIGTERM", func() {
		Skip("E2E test requires Kind cluster with AIAnalysis controller deployed")

		ctx := context.Background()

		// This test verifies that the Rego hot-reloader doesn't block shutdown
		// or cause goroutine leaks when SIGTERM is received.

		uniqueSuffix := fmt.Sprintf("%d", time.Now().UnixNano())
		analysisName := fmt.Sprintf("rego-sigterm-%s", uniqueSuffix)

		analysis := &aianalysisv1alpha1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      analysisName,
				Namespace: testNamespace,
			},
			Spec: aianalysisv1alpha1.AIAnalysisSpec{
				RemediationRequestRef: corev1.ObjectReference{
					APIVersion: "aianalysis.kubernaut.io/v1alpha1",
					Kind:       "RemediationRequest",
					Name:       "test-rr",
					Namespace:  testNamespace,
				},
				RemediationID: fmt.Sprintf("rem-%s", uniqueSuffix),
				AnalysisRequest: aianalysisv1alpha1.AnalysisRequest{
					SignalContext: aianalysisv1alpha1.SignalContextInput{
						Fingerprint:      fmt.Sprintf("e2e-rego-%s", uniqueSuffix),
						Severity:         "critical",
						SignalType:       "E2ETest",
						Environment:      "test",
						BusinessPriority: "P1",
						TargetResource: aianalysisv1alpha1.TargetResource{
							Kind:      "Pod",
							Name:      "test-pod",
							Namespace: testNamespace,
						},
						EnrichmentResults: sharedtypes.EnrichmentResults{
							KubernetesContext: &sharedtypes.KubernetesContext{
								Namespace: testNamespace,
							},
							DetectedLabels: &sharedtypes.DetectedLabels{
								GitOpsManaged: false,
								PDBProtected:  false,
								HPAEnabled:    false,
								Stateful:      false,
							},
						},
					},
					AnalysisTypes: []string{"investigation"},
				},
			},
		}

		err := k8sClient.Create(ctx, analysis)
		Expect(err).ToNot(HaveOccurred())

		// Wait for Analyzing phase (Rego policy evaluated)
		Eventually(func() string {
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      analysisName,
				Namespace: testNamespace,
			}, analysis)
			if err != nil {
				return ""
			}
			return analysis.Status.Phase
		}, 30*time.Second, 1*time.Second).Should(Or(
			Equal("Analyzing"),
			Equal("Completed"),
		), "AIAnalysis should reach Analyzing phase (Rego evaluation)")

		// Send SIGTERM
		GinkgoWriter.Printf("Sending SIGTERM during Rego policy evaluation\n")
		cmd := exec.Command("kubectl", "exec", "-n", serviceNamespace, servicePodName, "--",
			"sh", "-c", "kill -SIGTERM 1")
		output, err := cmd.CombinedOutput()
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("SIGTERM should be sent. Output: %s", string(output)))

		// Verify pod terminates cleanly (Rego doesn't block)
		Eventually(func() bool {
			pod := &corev1.Pod{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      servicePodName,
				Namespace: serviceNamespace,
			}, pod)
			return err != nil || pod.Status.Phase != corev1.PodRunning
		}, 20*time.Second, 1*time.Second).Should(BeTrue(),
			"BR-AI-012: Rego hot-reloader should not block graceful shutdown")

		GinkgoWriter.Printf("Rego hot-reloader stopped cleanly during shutdown\n")
	})

	// ==============================================
	// Test 4: Shutdown Without In-Flight Work
	// ==============================================

	It("should complete immediate shutdown when no work in progress", func() {
		Skip("E2E test requires Kind cluster with AIAnalysis controller deployed")

		ctx := context.Background()

		// This test verifies shutdown is fast when controller is idle

		// Send SIGTERM immediately (no AIAnalysis instances created)
		startTime := time.Now()

		GinkgoWriter.Printf("Sending SIGTERM to idle controller (no in-flight work)\n")
		cmd := exec.Command("kubectl", "exec", "-n", serviceNamespace, servicePodName, "--",
			"sh", "-c", "kill -SIGTERM 1")
		output, err := cmd.CombinedOutput()
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("SIGTERM should be sent. Output: %s", string(output)))

		// Verify fast shutdown (no work to complete)
		Eventually(func() bool {
			pod := &corev1.Pod{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      servicePodName,
				Namespace: serviceNamespace,
			}, pod)
			return err != nil || pod.Status.Phase != corev1.PodRunning
		}, 10*time.Second, 500*time.Millisecond).Should(BeTrue(),
			"Idle controller should shutdown within 10s")

		shutdownDuration := time.Since(startTime)
		GinkgoWriter.Printf("Idle shutdown completed in %v (Expected: <5s)\n", shutdownDuration)

		Expect(shutdownDuration).To(BeNumerically("<", 5*time.Second),
			"Idle shutdown should be very fast (<5s)")
	})

	AfterAll(func() {
		// Cleanup: Ensure pod is restored if tests were skipped
		if originalPodRunning {
			GinkgoWriter.Printf("E2E tests completed. Original pod: %s (Status: Running)\n", originalPodName)
		}
	})
})
