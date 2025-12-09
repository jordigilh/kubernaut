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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// WorkflowExecution Observability E2E Tests
//
// These tests validate business outcomes related to operational visibility:
// - BR-WE-005: Audit Events for Execution Lifecycle
// - BR-WE-007: Handle Externally Deleted PipelineRun
// - BR-WE-008: Prometheus Metrics for Execution Outcomes
//
// Per TESTING_GUIDELINES.md: E2E tests validate business value delivery

var _ = Describe("WorkflowExecution Observability E2E", func() {
	Context("BR-WE-005: Audit Events for Execution Lifecycle", func() {
		It("should emit Kubernetes events for phase transitions", func() {
			// Business Outcome: Operators can track workflow lifecycle via K8s events
			testName := fmt.Sprintf("e2e-events-%d", time.Now().UnixNano())
			targetResource := fmt.Sprintf("default/deployment/events-test-%d", time.Now().UnixNano())
			wfe := createTestWFE(testName, targetResource)

			defer func() {
				_ = deleteWFE(wfe)
			}()

			// Create WorkflowExecution
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			// Wait for completion (success or failure)
			Eventually(func() bool {
				updated, _ := getWFE(wfe.Name, wfe.Namespace)
				if updated != nil {
					phase := updated.Status.Phase
					return phase == workflowexecutionv1alpha1.PhaseCompleted ||
						phase == workflowexecutionv1alpha1.PhaseFailed
				}
				return false
			}, 120*time.Second).Should(BeTrue())

			// Verify events were emitted for this WFE
			// Business Behavior: Events should be visible via kubectl get events
			eventList := &corev1.EventList{}
			Eventually(func() bool {
				err := k8sClient.List(ctx, eventList)
				if err != nil {
					return false
				}
				for _, event := range eventList.Items {
					if event.InvolvedObject.Name == wfe.Name &&
						event.InvolvedObject.Kind == "WorkflowExecution" {
						return true
					}
				}
				return false
			}, 30*time.Second).Should(BeTrue(), "Expected Kubernetes events for WFE lifecycle")

			// Verify event content shows lifecycle transition
			var wfeEvents []corev1.Event
			for _, event := range eventList.Items {
				if event.InvolvedObject.Name == wfe.Name {
					wfeEvents = append(wfeEvents, event)
				}
			}
			Expect(len(wfeEvents)).To(BeNumerically(">", 0),
				"Expected at least one event for WFE lifecycle")

			GinkgoWriter.Printf("✅ BR-WE-005: Found %d events for WFE lifecycle\n", len(wfeEvents))
			for _, e := range wfeEvents {
				GinkgoWriter.Printf("   Event: %s - %s\n", e.Reason, e.Message)
			}
		})
	})

	Context("BR-WE-007: Handle Externally Deleted PipelineRun", func() {
		It("should mark WFE as Failed when PipelineRun is deleted externally", func() {
			// Business Outcome: Operators see clear failure reason when PR deleted
			testName := fmt.Sprintf("e2e-extdel-%d", time.Now().UnixNano())
			targetResource := fmt.Sprintf("default/deployment/extdel-test-%d", time.Now().UnixNano())
			wfe := createTestWFE(testName, targetResource)

			defer func() {
				_ = deleteWFE(wfe)
			}()

			// Create WorkflowExecution
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			// Wait for Running phase (PipelineRun created)
			Eventually(func() string {
				updated, _ := getWFE(wfe.Name, wfe.Namespace)
				if updated != nil {
					return updated.Status.Phase
				}
				return ""
			}, 60*time.Second).Should(Equal(workflowexecutionv1alpha1.PhaseRunning))

			GinkgoWriter.Println("✅ WFE is Running, PipelineRun exists")

			// Find and delete the PipelineRun externally (simulating operator action)
			prList := &tektonv1.PipelineRunList{}
			Expect(k8sClient.List(ctx, prList)).To(Succeed())

			var targetPR *tektonv1.PipelineRun
			for i := range prList.Items {
				pr := &prList.Items[i]
				// PipelineRuns created by WFE have our tracking labels
				if labels := pr.Labels; labels != nil {
					if labels["kubernaut.ai/workflow-execution"] == wfe.Name {
						targetPR = pr
						break
					}
				}
			}
			Expect(targetPR).ToNot(BeNil(), "Expected to find PipelineRun for WFE")

			// Delete the PipelineRun (external deletion)
			GinkgoWriter.Printf("🗑️  Deleting PipelineRun %s externally...\n", targetPR.Name)
			Expect(k8sClient.Delete(ctx, targetPR)).To(Succeed())

			// Business Behavior: WFE should detect deletion and mark as Failed
			Eventually(func() string {
				updated, _ := getWFE(wfe.Name, wfe.Namespace)
				if updated != nil {
					return updated.Status.Phase
				}
				return ""
			}, 60*time.Second).Should(Equal(workflowexecutionv1alpha1.PhaseFailed))

			// Verify failure details explain the external deletion
			failed, err := getWFE(wfe.Name, wfe.Namespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(failed.Status.FailureDetails).ToNot(BeNil())
			Expect(failed.Status.FailureDetails.Message).To(
				Or(
					ContainSubstring("not found"),
					ContainSubstring("deleted"),
					ContainSubstring("NotFound"),
				),
				"Failure message should indicate external deletion",
			)

			GinkgoWriter.Printf("✅ BR-WE-007: WFE correctly marked as Failed after external PR deletion\n")
			GinkgoWriter.Printf("   Failure reason: %s\n", failed.Status.FailureDetails.Reason)
			GinkgoWriter.Printf("   Failure message: %s\n", failed.Status.FailureDetails.Message)
		})
	})

	Context("BR-WE-008: Prometheus Metrics for Execution Outcomes", func() {
		It("should expose metrics on /metrics endpoint", func() {
			// Business Outcome: SREs can monitor workflow execution via Prometheus
			// Note: Metrics endpoint is exposed via NodePort in E2E environment

			// First, run a workflow to generate metrics
			testName := fmt.Sprintf("e2e-metrics-%d", time.Now().UnixNano())
			targetResource := fmt.Sprintf("default/deployment/metrics-test-%d", time.Now().UnixNano())
			wfe := createTestWFE(testName, targetResource)

			defer func() {
				_ = deleteWFE(wfe)
			}()

			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			// Wait for completion to generate metrics
			Eventually(func() bool {
				updated, _ := getWFE(wfe.Name, wfe.Namespace)
				if updated != nil {
					phase := updated.Status.Phase
					return phase == workflowexecutionv1alpha1.PhaseCompleted ||
						phase == workflowexecutionv1alpha1.PhaseFailed
				}
				return false
			}, 120*time.Second).Should(BeTrue())

			GinkgoWriter.Println("✅ WFE completed, checking metrics...")

			// Query metrics endpoint via NodePort
			// Per DD-TEST-001: Metrics NodePort is 30185
			metricsURL := fmt.Sprintf("http://localhost:%d/metrics", infrastructure.WorkflowExecutionMetricsHostPort)

			// Business Behavior: Metrics should be scrapable by Prometheus
			var metricsBody string
			Eventually(func() error {
				resp, err := http.Get(metricsURL)
				if err != nil {
					return err
				}
				defer resp.Body.Close()

				body, err := io.ReadAll(resp.Body)
				if err != nil {
					return err
				}
				metricsBody = string(body)
				return nil
			}, 30*time.Second).Should(Succeed(), "Should be able to scrape metrics endpoint")

			// Verify expected business metrics are present
			expectedMetrics := []string{
				"workflowexecution_total",                // Execution count by outcome
				"workflowexecution_duration_seconds",     // Execution duration histogram
				"workflowexecution_pipelinerun_creation", // PipelineRun creation counter
			}

			for _, metric := range expectedMetrics {
				Expect(metricsBody).To(ContainSubstring(metric),
					fmt.Sprintf("Expected metric %s to be exposed", metric))
				GinkgoWriter.Printf("✅ Metric found: %s\n", metric)
			}

			// Verify metrics have correct labels for business context
			Expect(metricsBody).To(
				Or(
					ContainSubstring("outcome="),
					ContainSubstring(`outcome"`),
				),
				"Metrics should include outcome label for SLO tracking",
			)

			GinkgoWriter.Println("✅ BR-WE-008: All expected Prometheus metrics exposed")
		})
	})

	// ========================================
	// BR-WE-005: Audit Persistence E2E (BLOCKED)
	// ========================================
	// This test validates audit events reach the Data Storage PostgreSQL database
	//
	// EXPECTED TO FAIL: Until Data Storage batch endpoint is fixed
	// See: NOTICE_DATASTORAGE_AUDIT_BATCH_ENDPOINT_MISSING.md
	//
	// Prerequisites:
	// 1. Data Storage service deployed in Kind cluster
	// 2. PostgreSQL database accessible
	// 3. Controller configured with --datastorage-url
	Context("BR-WE-005: Audit Persistence in PostgreSQL (E2E)", Label("datastorage", "audit"), func() {
		const dataStorageServiceURL = "http://datastorage-service.kubernaut-system:8080"

		It("should persist audit events to Data Storage for completed workflow", func() {
			// Per TESTING_GUIDELINES.md: Skip() is ABSOLUTELY FORBIDDEN - NO EXCEPTIONS
			// Per TESTING_GUIDELINES.md: E2E tests MUST use real infrastructure
			// Per DD-AUDIT-003: WorkflowExecution is P0 - MUST generate audit traces
			//
			// If Data Storage is not deployed, test FAILS (not skip)
			Expect(isDataStorageDeployed()).To(BeTrue(),
				"Data Storage REQUIRED but not deployed in cluster\n"+
					"  Per DD-AUDIT-003: WorkflowExecution is P0 - MUST generate audit traces\n"+
					"  Per TESTING_GUIDELINES.md: E2E tests MUST use real infrastructure\n"+
					"  Per TESTING_GUIDELINES.md: Skip() is FORBIDDEN - tests must FAIL\n\n"+
					"  Deploy Data Storage to Kind cluster before running E2E tests")

			By("Creating a WorkflowExecution to generate audit events")
			testName := fmt.Sprintf("e2e-audit-%d", time.Now().UnixNano())
			targetResource := fmt.Sprintf("default/deployment/audit-test-%d", time.Now().UnixNano())
			wfe := createTestWFE(testName, targetResource)

			defer func() {
				_ = deleteWFE(wfe)
			}()

			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			By("Waiting for workflow to complete")
			Eventually(func() bool {
				updated, _ := getWFE(wfe.Name, wfe.Namespace)
				if updated != nil {
					phase := updated.Status.Phase
					return phase == workflowexecutionv1alpha1.PhaseCompleted ||
						phase == workflowexecutionv1alpha1.PhaseFailed
				}
				return false
			}, 120*time.Second).Should(BeTrue())

			By("Querying Data Storage for audit events")
			// Query DS audit events API for events with this WFE's correlation ID
			// This verifies the full flow: Controller -> pkg/audit -> DS -> PostgreSQL
			//
			// EXPECTED TO FAIL: DS batch endpoint returns error
			// Error: "json: cannot unmarshal array into Go value"
			auditQueryURL := fmt.Sprintf("%s/api/v1/audit/events?correlation_id=%s",
				dataStorageServiceURL, wfe.Name)

			var auditEvents []map[string]interface{}
			Eventually(func() int {
				resp, err := http.Get(auditQueryURL)
				if err != nil {
					GinkgoWriter.Printf("⚠️ Audit query failed: %v\n", err)
					return 0
				}
				defer resp.Body.Close()

				if resp.StatusCode != http.StatusOK {
					GinkgoWriter.Printf("⚠️ Audit query returned %d\n", resp.StatusCode)
					return 0
				}

				// Parse response - expecting array of audit events
				body, _ := io.ReadAll(resp.Body)
				if err := json.Unmarshal(body, &auditEvents); err != nil {
					GinkgoWriter.Printf("⚠️ Failed to parse audit response: %v\n", err)
					return 0
				}

				return len(auditEvents)
			}, 60*time.Second).Should(BeNumerically(">=", 2),
				"BLOCKED: Expected at least 2 audit events (started + completed/failed). "+
					"If this fails, verify Data Storage batch endpoint is implemented. "+
					"See NOTICE_DATASTORAGE_AUDIT_BATCH_ENDPOINT_MISSING.md")

			By("Verifying audit event content")
			// Verify we have the expected event types
			eventTypes := make(map[string]bool)
			for _, event := range auditEvents {
				if eventType, ok := event["event_type"].(string); ok {
					eventTypes[eventType] = true
					GinkgoWriter.Printf("✅ Found audit event: %s\n", eventType)
				}
			}

			Expect(eventTypes).To(HaveKey("workflowexecution.workflow.started"),
				"Expected workflow.started audit event")
			Expect(eventTypes).To(Or(
				HaveKey("workflowexecution.workflow.completed"),
				HaveKey("workflowexecution.workflow.failed"),
			), "Expected workflow.completed or workflow.failed audit event")

			GinkgoWriter.Println("✅ BR-WE-005: Audit events persisted to Data Storage PostgreSQL")
		})
	})

	Context("BR-WE-003: Monitor Execution Status (Status Sync)", func() {
		It("should sync WFE status with PipelineRun status accurately", func() {
			// Business Outcome: WFE status accurately reflects execution state
			testName := fmt.Sprintf("e2e-sync-%d", time.Now().UnixNano())
			targetResource := fmt.Sprintf("default/deployment/sync-test-%d", time.Now().UnixNano())
			wfe := createTestWFE(testName, targetResource)

			defer func() {
				_ = deleteWFE(wfe)
			}()

			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			// Business Behavior: WFE should have PipelineRunRef after Running
			Eventually(func() bool {
				updated, _ := getWFE(wfe.Name, wfe.Namespace)
				if updated != nil && updated.Status.Phase == workflowexecutionv1alpha1.PhaseRunning {
					return updated.Status.PipelineRunRef != nil
				}
				return false
			}, 60*time.Second).Should(BeTrue(), "WFE should track PipelineRun reference")

			runningWFE, _ := getWFE(wfe.Name, wfe.Namespace)
			Expect(runningWFE.Status.PipelineRunRef).ToNot(BeNil())
			GinkgoWriter.Printf("✅ WFE tracks PipelineRun: %s\n",
				runningWFE.Status.PipelineRunRef.Name)

			// Wait for completion
			Eventually(func() bool {
				updated, _ := getWFE(wfe.Name, wfe.Namespace)
				if updated != nil {
					phase := updated.Status.Phase
					return phase == workflowexecutionv1alpha1.PhaseCompleted ||
						phase == workflowexecutionv1alpha1.PhaseFailed
				}
				return false
			}, 120*time.Second).Should(BeTrue())

			// Business Behavior: Completion should include timing information
			completedWFE, err := getWFE(wfe.Name, wfe.Namespace)
			Expect(err).ToNot(HaveOccurred())

			// Verify timing fields populated (for SLA tracking)
			Expect(completedWFE.Status.StartTime).ToNot(BeNil(),
				"StartTime should be set for SLA calculation")
			Expect(completedWFE.Status.CompletionTime).ToNot(BeNil(),
				"CompletionTime should be set for SLA calculation")
			Expect(completedWFE.Status.Duration).ToNot(BeEmpty(),
				"Duration should be set for metrics")

			GinkgoWriter.Printf("✅ BR-WE-003: Status sync complete\n")
			GinkgoWriter.Printf("   StartTime: %v\n", completedWFE.Status.StartTime.Time)
			GinkgoWriter.Printf("   CompletionTime: %v\n", completedWFE.Status.CompletionTime.Time)
			GinkgoWriter.Printf("   Duration: %s\n", completedWFE.Status.Duration)
		})
	})
})

// Helper to check if metrics contain expected labels
func metricsContainLabel(metrics, label string) bool {
	return strings.Contains(metrics, label)
}

// getPipelineRunForWFE finds the PipelineRun created by a WorkflowExecution
func getPipelineRunForWFE(wfeName, wfeNamespace string) (*tektonv1.PipelineRun, error) {
	prList := &tektonv1.PipelineRunList{}
	if err := k8sClient.List(ctx, prList); err != nil {
		return nil, err
	}

	for i := range prList.Items {
		pr := &prList.Items[i]
		if labels := pr.Labels; labels != nil {
			if labels["kubernaut.ai/workflow-execution"] == wfeName {
				return pr, nil
			}
		}
	}
	return nil, fmt.Errorf("PipelineRun not found for WFE %s", wfeName)
}

// isDataStorageDeployed checks if Data Storage service is deployed in the cluster
// This is used to skip audit persistence tests when DS infrastructure is not available
func isDataStorageDeployed() bool {
	deployment := &appsv1.Deployment{}
	err := k8sClient.Get(ctx, types.NamespacedName{
		Name:      "datastorage",
		Namespace: "kubernaut-system",
	}, deployment)

	if err != nil {
		// Also check default namespace
		err = k8sClient.Get(ctx, types.NamespacedName{
			Name:      "datastorage",
			Namespace: "default",
		}, deployment)
	}

	return err == nil && deployment.Status.ReadyReplicas > 0
}
