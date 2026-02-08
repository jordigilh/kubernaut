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
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	weaudit "github.com/jordigilh/kubernaut/pkg/workflowexecution/audit"
	wemetrics "github.com/jordigilh/kubernaut/pkg/workflowexecution/metrics"
	"github.com/jordigilh/kubernaut/test/infrastructure"
	testauth "github.com/jordigilh/kubernaut/test/shared/auth"
	"github.com/jordigilh/kubernaut/test/shared/validators"

	"github.com/google/uuid"
)

// WorkflowExecution Observability E2E Tests
//
// These tests validate business outcomes related to operational visibility:
// - BR-WE-005: Audit Events for Execution Lifecycle
// - BR-WE-007: Handle Externally Deleted PipelineRun
// - BR-WE-008: Prometheus Metrics for Execution Outcomes
//
// Per TESTING_GUIDELINES.md: E2E tests validate business value delivery
//
// V1.0 Maturity Requirement: Audit validation uses validators.ValidateAuditEvent (P0 - MANDATORY)
// Per SERVICE_MATURITY_REQUIREMENTS.md v1.2.0: Tests MUST use testutil validators
//
// V1.0 Maturity Requirement: Use OpenAPI client instead of raw HTTP (P1 enhancement)
// Per V1_0_SERVICE_MATURITY_TEST_PLAN_TEMPLATE.md: E2E tests SHOULD use OpenAPI client for type-safe responses
//
// All audit queries now use ogenclient.NewClientWithResponses() for type-safe ogenclient.AuditEvent responses.

var _ = Describe("WorkflowExecution Observability E2E", func() {
	Context("BR-WE-005: Audit Events for Execution Lifecycle", func() {
		It("should emit Kubernetes events for phase transitions", func() {
			// Business Outcome: Operators can track workflow lifecycle via K8s events
			testName := fmt.Sprintf("e2e-events-%s", uuid.New().String()[:8])
			targetResource := fmt.Sprintf("default/deployment/events-test-%s", uuid.New().String()[:8])
			wfe := createTestWFE(testName, targetResource)

			defer func() {
				_ = deleteWFE(wfe)
			}()

			// Create WorkflowExecution
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

		// Wait for completion (success or failure)
		Eventually(func() bool {
			updated, _ := getWFEDirect(wfe.Name, wfe.Namespace)
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
			testName := fmt.Sprintf("e2e-extdel-%s", uuid.New().String()[:8])
			targetResource := fmt.Sprintf("default/deployment/extdel-test-%s", uuid.New().String()[:8])
			wfe := createTestWFE(testName, targetResource)

			defer func() {
				_ = deleteWFE(wfe)
			}()

			// Create WorkflowExecution
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

		// Wait for Running phase (PipelineRun created)
		Eventually(func() string {
			updated, _ := getWFEDirect(wfe.Name, wfe.Namespace)
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
			updated, _ := getWFEDirect(wfe.Name, wfe.Namespace)
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
			testName := fmt.Sprintf("e2e-metrics-%s", uuid.New().String()[:8])
			targetResource := fmt.Sprintf("default/deployment/metrics-test-%s", uuid.New().String()[:8])
			wfe := createTestWFE(testName, targetResource)

			defer func() {
				_ = deleteWFE(wfe)
			}()

			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

		// Wait for completion to generate metrics
		Eventually(func() bool {
			updated, _ := getWFEDirect(wfe.Name, wfe.Namespace)
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
				defer func() { _ = resp.Body.Close() }()

				body, err := io.ReadAll(resp.Body)
				if err != nil {
					return err
				}
				metricsBody = string(body)
				return nil
			}, 30*time.Second).Should(Succeed(), "Should be able to scrape metrics endpoint")

			// Verify expected business metrics are present
			// Using constants from pkg/workflowexecution/metrics to prevent typos (DRY principle)
			expectedMetrics := []string{
				wemetrics.MetricNameExecutionTotal,       // Execution count by outcome
				wemetrics.MetricNameExecutionDuration,    // Execution duration histogram
				wemetrics.MetricNameExecutionCreations, // PipelineRun creation counter
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

		It("should increment workflowexecution_total{outcome=Completed} on successful completion", func() {
			// Business Outcome: SREs can track completion rate via Prometheus
			// This test validates metrics are actually incremented when workflows complete

			// Query initial metric value
			metricsURL := fmt.Sprintf("http://localhost:%d/metrics", infrastructure.WorkflowExecutionMetricsHostPort)

			var initialMetricsBody string
			Eventually(func() error {
				resp, err := http.Get(metricsURL)
				if err != nil {
					return err
				}
				defer func() { _ = resp.Body.Close() }()
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					return err
				}
				initialMetricsBody = string(body)
				return nil
			}, 30*time.Second).Should(Succeed(), "Should be able to scrape metrics endpoint initially")

			// Extract initial count (parse Prometheus format)
			initialCompletedCount := extractMetricValue(initialMetricsBody, wemetrics.MetricNameExecutionTotal, wemetrics.LabelOutcomeCompleted)
			GinkgoWriter.Printf("Initial %s{outcome=%s}: %.0f\n", wemetrics.MetricNameExecutionTotal, wemetrics.LabelOutcomeCompleted, initialCompletedCount)

			// Run a workflow that will complete successfully
			testName := fmt.Sprintf("e2e-metrics-completed-%s", uuid.New().String()[:8])
			targetResource := fmt.Sprintf("default/deployment/metrics-completed-%s", uuid.New().String()[:8])
			wfe := createTestWFE(testName, targetResource)

			defer func() {
				_ = deleteWFE(wfe)
			}()

			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

		// Wait for completion
		Eventually(func() bool {
			updated, _ := getWFEDirect(wfe.Name, wfe.Namespace)
			if updated != nil {
				return updated.Status.Phase == workflowexecutionv1alpha1.PhaseCompleted
			}
			return false
		}, 120*time.Second, 2*time.Second).Should(BeTrue(), "Workflow should complete")

			GinkgoWriter.Println("✅ Workflow completed, checking metrics...")

			// Verify metric incremented
			Eventually(func() bool {
				resp, err := http.Get(metricsURL)
				if err != nil {
					return false
				}
				defer func() { _ = resp.Body.Close() }()
				body, _ := io.ReadAll(resp.Body)
				metricsBody := string(body)

				currentCount := extractMetricValue(metricsBody, wemetrics.MetricNameExecutionTotal, wemetrics.LabelOutcomeCompleted)
				GinkgoWriter.Printf("Current %s{outcome=%s}: %.0f (initial: %.0f)\n", wemetrics.MetricNameExecutionTotal, wemetrics.LabelOutcomeCompleted, currentCount, initialCompletedCount)

				return currentCount > initialCompletedCount
			}, 30*time.Second, 2*time.Second).Should(BeTrue(),
				fmt.Sprintf("%s{outcome=%s} should increment after workflow completion", wemetrics.MetricNameExecutionTotal, wemetrics.LabelOutcomeCompleted))

			GinkgoWriter.Println("✅ BR-WE-008: Completion metric incremented on successful workflow")
		})

		It("should increment workflowexecution_total{outcome=Failed} on workflow failure", func() {
			// Business Outcome: SREs can track failure rate via Prometheus
			// This test validates metrics are actually incremented when workflows fail

			// Query initial metric value
			metricsURL := fmt.Sprintf("http://localhost:%d/metrics", infrastructure.WorkflowExecutionMetricsHostPort)

			var initialMetricsBody string
			Eventually(func() error {
				resp, err := http.Get(metricsURL)
				if err != nil {
					return err
				}
				defer func() { _ = resp.Body.Close() }()
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					return err
				}
				initialMetricsBody = string(body)
				return nil
			}, 30*time.Second).Should(Succeed(), "Should be able to scrape metrics endpoint initially")

			// Extract initial count
			initialFailedCount := extractMetricValue(initialMetricsBody, wemetrics.MetricNameExecutionTotal, wemetrics.LabelOutcomeFailed)
			GinkgoWriter.Printf("Initial %s{outcome=%s}: %.0f\n", wemetrics.MetricNameExecutionTotal, wemetrics.LabelOutcomeFailed, initialFailedCount)

			// Run a workflow with invalid image (will fail)
			testName := fmt.Sprintf("e2e-metrics-failed-%s", uuid.New().String()[:8])
			targetResource := fmt.Sprintf("default/deployment/metrics-failed-%s", uuid.New().String()[:8])
			wfe := createTestWFE(testName, targetResource)

			// Use invalid workflow image to trigger failure
			wfe.Spec.WorkflowRef.ContainerImage = "ghcr.io/invalid/nonexistent:latest"

			defer func() {
				_ = deleteWFE(wfe)
			}()

			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

		// Wait for failure
		Eventually(func() bool {
			updated, _ := getWFEDirect(wfe.Name, wfe.Namespace)
			if updated != nil {
				return updated.Status.Phase == workflowexecutionv1alpha1.PhaseFailed
			}
			return false
		}, 120*time.Second, 2*time.Second).Should(BeTrue(), "Workflow should fail")

			GinkgoWriter.Println("✅ Workflow failed as expected, checking metrics...")

			// Verify metric incremented
			Eventually(func() bool {
				resp, err := http.Get(metricsURL)
				if err != nil {
					return false
				}
				defer func() { _ = resp.Body.Close() }()
				body, _ := io.ReadAll(resp.Body)
				metricsBody := string(body)

				currentCount := extractMetricValue(metricsBody, wemetrics.MetricNameExecutionTotal, wemetrics.LabelOutcomeFailed)
				GinkgoWriter.Printf("Current %s{outcome=%s}: %.0f (initial: %.0f)\n", wemetrics.MetricNameExecutionTotal, wemetrics.LabelOutcomeFailed, currentCount, initialFailedCount)

				return currentCount > initialFailedCount
			}, 30*time.Second, 2*time.Second).Should(BeTrue(),
				fmt.Sprintf("%s{outcome=%s} should increment after workflow failure", wemetrics.MetricNameExecutionTotal, wemetrics.LabelOutcomeFailed))

			GinkgoWriter.Println("✅ BR-WE-008: Failure metric incremented on failed workflow")
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
		// NodePort access per DD-TEST-001: localhost:8092 → NodePort 30081 → DS pod:8080
		const dataStorageServiceURL = "http://localhost:8092"

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
			testName := fmt.Sprintf("e2e-audit-%s", uuid.New().String()[:8])
			targetResource := fmt.Sprintf("default/deployment/audit-test-%s", uuid.New().String()[:8])
			wfe := createTestWFE(testName, targetResource)

			defer func() {
				_ = deleteWFE(wfe)
			}()

			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

		By("Waiting for workflow to complete")
		Eventually(func() bool {
			updated, _ := getWFEDirect(wfe.Name, wfe.Namespace)
			if updated != nil {
				phase := updated.Status.Phase
				return phase == workflowexecutionv1alpha1.PhaseCompleted ||
					phase == workflowexecutionv1alpha1.PhaseFailed
			}
			return false
		}, 120*time.Second).Should(BeTrue())

			// Wait for audit batch to flush to DataStorage (1s flush interval + buffer)
			time.Sleep(3 * time.Second)

			By("Querying Data Storage for audit events via authenticated OpenAPI client")
			// V1.0 MANDATORY: Use OpenAPI client instead of raw HTTP (per V1_0_SERVICE_MATURITY_TEST_PLAN_TEMPLATE.md)
			// DD-AUTH-014: All DataStorage requests require ServiceAccount Bearer tokens
			// Query DS audit events API for events with this WFE's correlation ID
			// This verifies the full flow: Controller -> pkg/audit -> DS -> PostgreSQL
			saTransport := testauth.NewServiceAccountTransport(e2eAuthToken)
			httpClient := &http.Client{
				Timeout:   20 * time.Second,
				Transport: saTransport,
			}
			auditClient, err := ogenclient.NewClient(dataStorageServiceURL, ogenclient.WithClient(httpClient))
			Expect(err).ToNot(HaveOccurred(), "Failed to create authenticated OpenAPI audit client")

			eventCategory := weaudit.CategoryWorkflowExecution // Per ADR-034 v1.5
			var auditEvents []ogenclient.AuditEvent
			Eventually(func() int {
				resp, err := auditClient.QueryAuditEvents(ctx, ogenclient.QueryAuditEventsParams{
					EventCategory: ogenclient.NewOptString(eventCategory),
					CorrelationID: ogenclient.NewOptString(wfe.Spec.RemediationRequestRef.Name), // Use RemediationRequest Name as correlation ID
				})

				// DEBUG: Comprehensive response logging (Dec 28, 2025 investigation)
				GinkgoWriter.Printf("🔍 Query: event_category=%s, correlation_id=%s\n", eventCategory, wfe.Spec.RemediationRequestRef.Name)

				if err != nil {
					GinkgoWriter.Printf("⚠️ Audit query failed: %v\n", err)
					return 0
				}

				auditEvents = resp.Data
				totalCount := len(auditEvents)
				GinkgoWriter.Printf("✅ Found %d events\n", totalCount)
				if resp.Pagination.IsSet() && resp.Pagination.Value.Total.IsSet() {
					GinkgoWriter.Printf("📊 Total in DB: %d\n", resp.Pagination.Value.Total.Value)
				}
				return totalCount
			}, 60*time.Second).Should(BeNumerically(">=", 2),
				"BLOCKED: Expected at least 2 audit events (started + completed/failed). "+
					"If this fails, verify Data Storage batch endpoint is implemented. "+
					"See NOTICE_DATASTORAGE_AUDIT_BATCH_ENDPOINT_MISSING.md")

			By("Verifying audit event content")
			// Verify we have the expected event types (Per ADR-034 v1.5: workflowexecution.* prefix)
			eventTypes := make(map[string]bool)
			for _, event := range auditEvents {
				eventTypes[event.EventType] = true
				GinkgoWriter.Printf("✅ Found audit event: %s\n", event.EventType)
			}

			Expect(eventTypes).To(HaveKey(weaudit.EventTypeExecutionStarted),
				"Expected workflowexecution.execution.started audit event (Gap #6, ADR-034 v1.5)")
			Expect(eventTypes).To(Or(
				HaveKey(weaudit.EventTypeCompleted),
				HaveKey(weaudit.EventTypeFailed),
			), "Expected workflowexecution.workflow.completed or workflowexecution.workflow.failed audit event (ADR-034 v1.5)")

			GinkgoWriter.Println("✅ BR-WE-005: Audit events persisted to Data Storage PostgreSQL")
		})

		It("should emit workflow.failed audit event with complete failure details", func() {
			// BR-WE-005 EXTENDED: Validate workflow.failed audit event structure
			// This validation moved from integration tests (EnvTest limitation) to E2E
			// Per DD-AUDIT-004: Type-safe audit payloads with complete failure context

			const dataStorageServiceURL = "http://localhost:8092" // DD-TEST-001: WE → DataStorage dependency port

			Expect(isDataStorageDeployed()).To(BeTrue(),
				"Data Storage REQUIRED but not deployed in cluster")

			By("Creating a WorkflowExecution that will fail")
			testName := fmt.Sprintf("e2e-audit-failure-%s", uuid.New().String()[:8])
			targetResource := fmt.Sprintf("default/deployment/audit-fail-test-%s", uuid.New().String()[:8])

			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testName,
					Namespace: "default",
				},
			Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
				ExecutionEngine: "tekton", // BR-WE-014: Required field
				RemediationRequestRef: corev1.ObjectReference{
					APIVersion: "remediationorchestrator.kubernaut.ai/v1alpha1",
					Kind:       "RemediationRequest",
					Name:       "test-rr-" + testName,
					Namespace:  "default",
				},
			WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
				WorkflowID: "test-intentional-failure",
				Version:    "v1.0.0",
				// Use multi-arch bundle from quay.io/kubernaut-cicd (amd64 + arm64)
				ContainerImage: "quay.io/kubernaut-cicd/test-workflows/failing:v1.0.0",
			},
				TargetResource: targetResource,
				Parameters: map[string]string{
					// Per test/fixtures/tekton/failing-pipeline.yaml
					"FAILURE_MODE":    "exit",
					"FAILURE_MESSAGE": "E2E audit failure validation",
				},
			},
			}

			defer func() {
				_ = deleteWFE(wfe)
			}()

			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

		By("Waiting for Failed phase")
		Eventually(func() string {
			updated, _ := getWFEDirect(wfe.Name, wfe.Namespace)
			if updated != nil {
				return updated.Status.Phase
			}
			return ""
		}, 120*time.Second, 2*time.Second).Should(Equal(workflowexecutionv1alpha1.PhaseFailed))

			// Wait for audit batch to flush to DataStorage (1s flush interval + buffer)
			time.Sleep(3 * time.Second)

			By("Querying Data Storage for workflow.failed audit event via authenticated OpenAPI client")
			// V1.0 MANDATORY: Use OpenAPI client instead of raw HTTP (per V1_0_SERVICE_MATURITY_TEST_PLAN_TEMPLATE.md)
			// DD-AUTH-014: All DataStorage requests require ServiceAccount Bearer tokens
			// Use correlation ID to find this specific WFE's events
			saTransport := testauth.NewServiceAccountTransport(e2eAuthToken)
			httpClient := &http.Client{
				Timeout:   20 * time.Second,
				Transport: saTransport,
			}
			auditClient, err := ogenclient.NewClient(dataStorageServiceURL, ogenclient.WithClient(httpClient))
			Expect(err).ToNot(HaveOccurred(), "Failed to create authenticated OpenAPI audit client")

			eventCategory := weaudit.CategoryWorkflowExecution // Per ADR-034 v1.5
			var failedEvent *ogenclient.AuditEvent
			Eventually(func() bool {
				resp, err := auditClient.QueryAuditEvents(ctx, ogenclient.QueryAuditEventsParams{
					EventCategory: ogenclient.NewOptString(eventCategory),
					CorrelationID: ogenclient.NewOptString(wfe.Spec.RemediationRequestRef.Name), // Use RemediationRequest Name as correlation ID
				})

				// DEBUG: Comprehensive response logging (Dec 28, 2025 investigation)
				GinkgoWriter.Printf("🔍 Query: event_category=%s, correlation_id=%s\n", eventCategory, wfe.Spec.RemediationRequestRef.Name)

				if err != nil {
					GinkgoWriter.Printf("⚠️ Audit query failed: %v\n", err)
					return false
				}

				auditEvents := resp.Data
				GinkgoWriter.Printf("✅ Found %d events\n", len(auditEvents))

				// Find workflowexecution.workflow.failed event (per ADR-034 v1.5)
				for i := range auditEvents {
					event := &auditEvents[i]
					GinkgoWriter.Printf("   Event %d: type=%s\n", i, event.EventType)
					if event.EventType == weaudit.EventTypeFailed {
						failedEvent = event
						return true
					}
				}
				GinkgoWriter.Printf("⚠️ %s event not found in %d events\n", weaudit.EventTypeFailed, len(auditEvents))
				return false
			}, 60*time.Second, 2*time.Second).Should(BeTrue(),
				weaudit.EventTypeFailed+" audit event should be present in Data Storage (ADR-034 v1.5)")

			By("Verifying " + weaudit.EventTypeFailed + " event includes complete failure details (ADR-034 v1.5)")
			Expect(failedEvent).ToNot(BeNil())

			// V1.0 Maturity Requirement: Use validators.ValidateAuditEvent (P0 - MANDATORY)
			// Per SERVICE_MATURITY_REQUIREMENTS.md v1.2.0: Tests MUST use testutil validators
			By("Validating audit event structure with validators.ValidateAuditEvent")
			validators.ValidateAuditEvent(*failedEvent, validators.ExpectedAuditEvent{
				EventType:     weaudit.EventTypeFailed, // Per ADR-034 v1.5
				EventCategory: ogenclient.AuditEventEventCategoryWorkflowexecution,
				EventAction:   "failed", // EventAction = last part after "." (audit.go:109)
				EventOutcome:  validators.EventOutcomePtr(ogenclient.AuditEventEventOutcomeFailure),
				CorrelationID: wfe.Spec.RemediationRequestRef.Name, // RemediationRequest name is the correlation ID
			})
			GinkgoWriter.Println("✅ validators.ValidateAuditEvent passed for workflowexecution.workflow.failed event (ADR-034 v1.5)")

			// Additional business-specific validation (complements testutil validation)
			Expect(failedEvent.EventOutcome).To(Equal(ogenclient.AuditEventEventOutcomeFailure))
			Expect(failedEvent.EventData).ToNot(BeNil())

			// Extract and validate event_data payload (type-safe access via ogen client)
			eventData, ok := failedEvent.EventData.GetWorkflowExecutionAuditPayload()
			Expect(ok).To(BeTrue(), "EventData should be WorkflowExecutionAuditPayload")

			// Validate WorkflowExecutionAuditPayload failure fields
			Expect(eventData.WorkflowID).ToNot(BeEmpty())
			Expect(eventData.WorkflowVersion).ToNot(BeEmpty())
			Expect(eventData.TargetResource).ToNot(BeEmpty())
			Expect(string(eventData.Phase)).To(Equal("Failed"))

			// Critical: Verify failure details are included
			Expect(eventData.FailureReason.IsSet()).To(BeTrue(), "failure_reason MUST be populated in workflowexecution.workflow.failed events")
			Expect(eventData.FailureMessage.IsSet()).To(BeTrue(), "failure_message MUST be populated in workflowexecution.workflow.failed events")

			GinkgoWriter.Printf("✅ workflowexecution.workflow.failed audit event validated:\n")
			GinkgoWriter.Printf("   - Failure Reason: %v\n", eventData.FailureReason.Value)
			GinkgoWriter.Printf("   - Failure Message: %v\n", eventData.FailureMessage.Value)
			GinkgoWriter.Printf("   - Execution Phase: %v\n", eventData.Phase)
		})

		It("should persist audit events with correct WorkflowExecutionAuditPayload fields", func() {
			// Per ADR-032: WorkflowExecution is P0 - audit is MANDATORY
			// Per DD-AUDIT-004: Type-safe audit payloads (no map[string]interface{})
			//
			// This test validates that ALL WorkflowExecutionAuditPayload fields
			// are correctly stored in DataStorage PostgreSQL

			Expect(isDataStorageDeployed()).To(BeTrue(),
				"Data Storage REQUIRED but not deployed in cluster")

			By("Creating a successful WorkflowExecution to generate complete audit trail")
			testName := fmt.Sprintf("e2e-audit-fields-%s", uuid.New().String()[:8])
			targetResource := fmt.Sprintf("default/deployment/audit-fields-test-%s", uuid.New().String()[:8])
			wfe := createTestWFE(testName, targetResource)

			defer func() {
				_ = deleteWFE(wfe)
			}()

			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

		By("Waiting for workflow to complete")
		Eventually(func() string {
			updated, _ := getWFEDirect(wfe.Name, wfe.Namespace)
			if updated != nil {
				return updated.Status.Phase
			}
			return ""
		}, 120*time.Second).Should(Or(
			Equal(workflowexecutionv1alpha1.PhaseCompleted),
			Equal(workflowexecutionv1alpha1.PhaseFailed),
		))

			// Wait for audit batch to flush to DataStorage (1s flush interval + buffer)
			time.Sleep(3 * time.Second)

			By("Querying Data Storage for all audit events via authenticated OpenAPI client")
			// V1.0 MANDATORY: Use OpenAPI client instead of raw HTTP (per V1_0_SERVICE_MATURITY_TEST_PLAN_TEMPLATE.md)
			// DD-AUTH-014: All DataStorage requests require ServiceAccount Bearer tokens
			saTransport := testauth.NewServiceAccountTransport(e2eAuthToken)
			httpClient := &http.Client{
				Timeout:   20 * time.Second,
				Transport: saTransport,
			}
			auditClient, err := ogenclient.NewClient(dataStorageServiceURL, ogenclient.WithClient(httpClient))
			Expect(err).ToNot(HaveOccurred(), "Failed to create authenticated OpenAPI audit client")

			eventCategory := weaudit.CategoryWorkflowExecution // Per ADR-034 v1.5
			var auditEvents []ogenclient.AuditEvent
			Eventually(func() int {
				resp, err := auditClient.QueryAuditEvents(ctx, ogenclient.QueryAuditEventsParams{
					EventCategory: ogenclient.NewOptString(eventCategory),
					CorrelationID: ogenclient.NewOptString(wfe.Spec.RemediationRequestRef.Name), // Use RemediationRequest Name as correlation ID
				})

				// DEBUG: Comprehensive response logging (Dec 28, 2025 investigation)
				GinkgoWriter.Printf("🔍 Query: event_category=%s, correlation_id=%s\n", eventCategory, wfe.Spec.RemediationRequestRef.Name)

				if err != nil {
					GinkgoWriter.Printf("⚠️ Audit query failed: %v\n", err)
					return 0
				}

				auditEvents = resp.Data
				totalCount := len(auditEvents)
				GinkgoWriter.Printf("✅ Found %d events\n", totalCount)
				if resp.Pagination.IsSet() && resp.Pagination.Value.Total.IsSet() {
					GinkgoWriter.Printf("📊 Total in DB: %d\n", resp.Pagination.Value.Total.Value)
				}
				for i, event := range auditEvents {
					GinkgoWriter.Printf("   Event %d: type=%s\n", i, event.EventType)
				}
				return totalCount
			}, 60*time.Second).Should(BeNumerically(">=", 2),
				"Expected at least 2 audit events")

			By("Validating workflowexecution.execution.started event payload fields (Gap #6, ADR-034 v1.5)")
			var startedEvent *ogenclient.AuditEvent
			for i := range auditEvents {
				if auditEvents[i].EventType == weaudit.EventTypeExecutionStarted {
					startedEvent = &auditEvents[i]
					break
				}
			}

			// DEBUG: Show all event types if not found
			if startedEvent == nil {
				GinkgoWriter.Printf("⚠️ %s not found. Available event types:\n", weaudit.EventTypeExecutionStarted)
				for i := range auditEvents {
					GinkgoWriter.Printf("   %d: %s\n", i, auditEvents[i].EventType)
				}
			}

			Expect(startedEvent).ToNot(BeNil(), weaudit.EventTypeExecutionStarted+" event not found (Gap #6, ADR-034 v1.5)")

			// Extract event_data using type-safe ogen client method
			eventData, ok := startedEvent.EventData.GetWorkflowExecutionAuditPayload()
			Expect(ok).To(BeTrue(), "EventData should be WorkflowExecutionAuditPayload")

			// Validate CORE fields (5 fields - always present)
			GinkgoWriter.Println("✅ Validating CORE audit fields...")
			Expect(eventData.WorkflowID).To(Equal(wfe.Spec.WorkflowRef.WorkflowID),
				"workflow_id should match")
			Expect(eventData.TargetResource).To(Equal(targetResource),
				"target_resource should match")
			Expect(string(eventData.Phase)).ToNot(BeEmpty(),
				"phase should be present")
			Expect(eventData.ContainerImage).To(Equal(wfe.Spec.WorkflowRef.ContainerImage),
				"container_image should match")
			Expect(eventData.ExecutionName).To(Equal(wfe.Name),
				"execution_name should match")

			By("Validating workflowexecution.workflow.completed or workflowexecution.workflow.failed event payload fields (ADR-034 v1.5)")
			var terminalEvent *ogenclient.AuditEvent
			var terminalEventType string
			for i := range auditEvents {
				if auditEvents[i].EventType == weaudit.EventTypeCompleted || auditEvents[i].EventType == weaudit.EventTypeFailed {
					terminalEvent = &auditEvents[i]
					terminalEventType = auditEvents[i].EventType
					break
				}
			}
			Expect(terminalEvent).ToNot(BeNil(), "terminal event (completed/failed) not found")

			terminalEventData, ok := terminalEvent.EventData.GetWorkflowExecutionAuditPayload()
			Expect(ok).To(BeTrue(), "terminal EventData should be WorkflowExecutionAuditPayload")

			// Validate TIMING fields (3 fields - present when Running/Completed/Failed)
			GinkgoWriter.Printf("✅ Validating TIMING fields for %s...\n", terminalEventType)
			Expect(terminalEventData.StartedAt.IsSet()).To(BeTrue(),
				"started_at should be present in terminal event")
			Expect(terminalEventData.CompletedAt.IsSet()).To(BeTrue(),
				"completed_at should be present in terminal event")
			Expect(terminalEventData.Duration.IsSet()).To(BeTrue(),
				"duration should be present in terminal event")

			// Validate PIPELINERUN REFERENCE field (1 field - present when PipelineRun created)
			GinkgoWriter.Println("✅ Validating PIPELINERUN REFERENCE field...")
			Expect(terminalEventData.PipelinerunName.IsSet()).To(BeTrue(),
				"pipelinerun_name should be present")

			// If workflow failed, validate FAILURE fields (3 fields)
			if terminalEventType == weaudit.EventTypeFailed {
				GinkgoWriter.Println("✅ Validating FAILURE fields...")
				Expect(terminalEventData.FailureReason.IsSet()).To(BeTrue(),
					"failure_reason should be present in failed event")
				Expect(terminalEventData.FailureMessage.IsSet()).To(BeTrue(),
					"failure_message should be present in failed event")
				// failed_task_name is optional (only when specific TaskRun identified)
			}

			GinkgoWriter.Printf("✅ BR-WE-005 + DD-AUDIT-004: All WorkflowExecutionAuditPayload fields validated\n")
			GinkgoWriter.Printf("   Event count: %d\n", len(auditEvents))
			GinkgoWriter.Printf("   Terminal event: %s\n", terminalEventType)
			GinkgoWriter.Printf("   Core fields: ✅ (5/5)\n")
			GinkgoWriter.Printf("   Timing fields: ✅ (3/3)\n")
			GinkgoWriter.Printf("   PipelineRun reference: ✅ (1/1)\n")
			if terminalEventType == weaudit.EventTypeFailed {
				GinkgoWriter.Printf("   Failure fields: ✅ (validated)\n")
			}
		})
	})

	Context("BR-WE-003: Monitor Execution Status (Status Sync)", func() {
		It("should sync WFE status with PipelineRun status accurately", func() {
			// Business Outcome: WFE status accurately reflects execution state
			testName := fmt.Sprintf("e2e-sync-%s", uuid.New().String()[:8])
			targetResource := fmt.Sprintf("default/deployment/sync-test-%s", uuid.New().String()[:8])
			wfe := createTestWFE(testName, targetResource)

			defer func() {
				_ = deleteWFE(wfe)
			}()

			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

		// Business Behavior: WFE should have ExecutionRef after Running
		Eventually(func() bool {
			updated, _ := getWFEDirect(wfe.Name, wfe.Namespace)
			if updated != nil && updated.Status.Phase == workflowexecutionv1alpha1.PhaseRunning {
				return updated.Status.ExecutionRef != nil
			}
			return false
		}, 60*time.Second).Should(BeTrue(), "WFE should track PipelineRun reference")

			runningWFE, _ := getWFE(wfe.Name, wfe.Namespace)
			Expect(runningWFE.Status.ExecutionRef).ToNot(BeNil())
			GinkgoWriter.Printf("✅ WFE tracks PipelineRun: %s\n",
				runningWFE.Status.ExecutionRef.Name)

		// Wait for completion
		Eventually(func() bool {
			updated, _ := getWFEDirect(wfe.Name, wfe.Namespace)
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
// extractMetricValue parses Prometheus metrics format and extracts the value for a specific metric and label
// Example: workflowexecution_total{outcome="Completed"} 5.0
func extractMetricValue(metricsBody, metricName, outcomeLabel string) float64 {
	// Parse Prometheus text format
	// Look for lines like: workflowexecution_total{outcome="Completed"} 5.0
	lines := strings.Split(metricsBody, "\n")

	for _, line := range lines {
		// Skip comments and empty lines
		if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
			continue
		}

		// Check if this line is for our metric
		if !strings.HasPrefix(line, metricName) {
			continue
		}

		// Check if it has the outcome label we're looking for
		expectedLabel := fmt.Sprintf(`outcome="%s"`, outcomeLabel)
		if !strings.Contains(line, expectedLabel) {
			continue
		}

		// Extract the value (last token after space)
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			value, err := strconv.ParseFloat(parts[len(parts)-1], 64)
			if err == nil {
				return value
			}
		}
	}

	// Return 0 if metric not found
	return 0.0
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
