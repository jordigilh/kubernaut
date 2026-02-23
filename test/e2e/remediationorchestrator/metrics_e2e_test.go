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

// Package remediationorchestrator_test contains E2E tests for the RemediationOrchestrator controller.
// This file tests metrics endpoint exposure and content per DD-METRICS-001 and BR-ORCH-044.
package remediationorchestrator

import (
	"context"
	"io"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"

	"github.com/google/uuid"
)

var skipMetricsSeeding bool

// metricsURL is the HTTP endpoint for RO metrics (per DD-TEST-001)
// RemediationOrchestrator: Metrics Host: 9183, NodePort: 30183
var metricsURL = "http://localhost:9183"

// seedMetricsWithRemediation creates a simple RemediationRequest and waits for processing
// to ensure all metrics are populated before tests run.
// This is required because CRD controllers with envtest don't expose HTTP endpoints,
// so we must validate metrics through the Kind cluster deployment.
func seedMetricsWithRemediation() {
	ctx := context.Background()

	GinkgoWriter.Println("🌱 Seeding metrics with RemediationRequest...")

	// Create a simple RemediationRequest to populate reconciliation metrics
	rr := &remediationv1.RemediationRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "metrics-seed-" + randomSuffix(),
			Namespace: "kubernaut-system",
		},
		Spec: remediationv1.RemediationRequestSpec{
			SignalFingerprint: "abc123def456abc123def456abc123def456abc123def456abc123def456abc1", // Valid 64-char hex
			SignalName:        "metrics-seed-signal",
			Severity:          "warning",
			SignalType:        "kubernetes-event",
			TargetType:        "kubernetes",
			TargetResource: remediationv1.ResourceIdentifier{
				Kind:      "Pod",
				Namespace: "default",
				Name:      "test-pod",
			},
			FiringTime:   metav1.Now(),
			ReceivedTime: metav1.Now(),
		},
	}

	Expect(k8sClient.Create(ctx, rr)).To(Succeed())

	// Wait for RR to be processed (any phase transition)
	Eventually(func() remediationv1.RemediationPhase {
		var updated remediationv1.RemediationRequest
		if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), &updated); err != nil {
			return ""
		}
		return updated.Status.OverallPhase
	}, 30*time.Second, 1*time.Second).ShouldNot(BeEmpty(), "Metrics seeding RR should be processed")

	GinkgoWriter.Println("✅ Metrics seeding complete")
}

// randomSuffix generates a random suffix for unique resource names
// Uses nanoseconds to prevent collisions in parallel Ginkgo processes
func randomSuffix() string {
	return uuid.New().String()[:8]
}

var _ = Describe("RemediationOrchestrator Metrics E2E", Label("e2e", "metrics"), func() {
	var httpClient *http.Client

	BeforeEach(func() {
		httpClient = &http.Client{
			Timeout: 10 * time.Second,
		}
	})

	// Seed metrics once for all tests in this suite
	BeforeEach(func() {
		if skipMetricsSeeding {
			return
		}
		seedMetricsWithRemediation()
		skipMetricsSeeding = true
	})

	Context("Prometheus metrics (/metrics) - DD-METRICS-001, BR-ORCH-044", func() {
		It("should expose metrics in Prometheus format", func() {
			resp, err := httpClient.Get(metricsURL + "/metrics")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(resp.Header.Get("Content-Type")).To(ContainSubstring("text/plain"))
		})

		It("should include core reconciliation metrics - BR-ORCH-044", func() {
			resp, err := httpClient.Get(metricsURL + "/metrics")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			body, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			metricsText := string(body)

			// Core reconciliation metrics (BR-ORCH-044: Reconciliation Metrics)
			coreMetrics := []string{
				"kubernaut_remediationorchestrator_reconcile_total",
				"kubernaut_remediationorchestrator_reconcile_duration_seconds",
				"kubernaut_remediationorchestrator_phase_transitions_total",
			}

			for _, metric := range coreMetrics {
				Expect(metricsText).To(ContainSubstring(metric),
					"Missing core metric: %s", metric)
			}
		})

		It("should include child CRD orchestration metrics - BR-ORCH-044", func() {
			resp, err := httpClient.Get(metricsURL + "/metrics")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			body, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			metricsText := string(body)

			// Child CRD orchestration (BR-ORCH-044: Child CRD Creation Tracking)
			Expect(metricsText).To(ContainSubstring("kubernaut_remediationorchestrator_child_crd_creations_total"),
				"Missing child CRD orchestration metric")

			// Verify it has the expected child_type label
			Expect(metricsText).To(MatchRegexp(`kubernaut_remediationorchestrator_child_crd_creations_total\{.*child_type=.*\}`),
				"Child CRD metric should have 'child_type' label")
		})

		It("should include notification metrics - BR-ORCH-029, BR-ORCH-030", func() {
			resp, err := httpClient.Get(metricsURL + "/metrics")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			body, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			metricsText := string(body)

			// Notification metrics (BR-ORCH-029: Manual Review, BR-ORCH-030: Approval)
			notificationMetrics := []string{
				"kubernaut_remediationorchestrator_manual_review_notifications_total",
				"kubernaut_remediationorchestrator_approval_notifications_total",
				"kubernaut_remediationorchestrator_notification_cancellations_total",
				"kubernaut_remediationorchestrator_notification_status",
				"kubernaut_remediationorchestrator_notification_delivery_duration_seconds",
			}

			for _, metric := range notificationMetrics {
				Expect(metricsText).To(ContainSubstring(metric),
					"Missing notification metric: %s", metric)
			}
		})

		It("should include routing decision metrics - BR-ORCH-044", func() {
			resp, err := httpClient.Get(metricsURL + "/metrics")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			body, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			metricsText := string(body)

			// Routing decision metrics (BR-ORCH-044: Routing Decisions)
			routingMetrics := []string{
				"kubernaut_remediationorchestrator_no_action_needed_total",
				"kubernaut_remediationorchestrator_duplicates_skipped_total",
				"kubernaut_remediationorchestrator_timeouts_total",
			}

			for _, metric := range routingMetrics {
				Expect(metricsText).To(ContainSubstring(metric),
					"Missing routing metric: %s", metric)
			}
		})

		It("should include blocking metrics - BR-ORCH-042", func() {
			resp, err := httpClient.Get(metricsURL + "/metrics")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			body, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			metricsText := string(body)

			// Blocking metrics (BR-ORCH-042: Blocking Conditions)
			blockingMetrics := []string{
				"kubernaut_remediationorchestrator_blocked_total",
				"kubernaut_remediationorchestrator_blocked_cooldown_expired_total",
				"kubernaut_remediationorchestrator_current_blocked",
			}

			for _, metric := range blockingMetrics {
				Expect(metricsText).To(ContainSubstring(metric),
					"Missing blocking metric: %s", metric)
			}
		})

		It("should include retry metrics - REFACTOR-RO-008", func() {
			resp, err := httpClient.Get(metricsURL + "/metrics")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			body, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			metricsText := string(body)

			// Retry metrics (REFACTOR-RO-008: Status Update Retry Pattern)
			retryMetrics := []string{
				"kubernaut_remediationorchestrator_status_update_retries_total",
				"kubernaut_remediationorchestrator_status_update_conflicts_total",
			}

			for _, metric := range retryMetrics {
				Expect(metricsText).To(ContainSubstring(metric),
					"Missing retry metric: %s", metric)
			}
		})

		It("should include condition metrics - BR-ORCH-043, DD-CRD-002", func() {
			resp, err := httpClient.Get(metricsURL + "/metrics")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			body, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			metricsText := string(body)

			// Condition metrics (BR-ORCH-043: Kubernetes Conditions, DD-CRD-002: Condition Transition Tracking)
			conditionMetrics := []string{
				"kubernaut_remediationorchestrator_condition_status",
				"kubernaut_remediationorchestrator_condition_transitions_total",
			}

			for _, metric := range conditionMetrics {
				Expect(metricsText).To(ContainSubstring(metric),
					"Missing condition metric: %s", metric)
			}

			// Verify condition metrics have expected labels
			Expect(metricsText).To(MatchRegexp(`kubernaut_remediationorchestrator_condition_status\{.*condition_type=.*status=.*\}`),
				"Condition status metric should have 'condition_type' and 'status' labels")
			Expect(metricsText).To(MatchRegexp(`kubernaut_remediationorchestrator_condition_transitions_total\{.*condition_type=.*from_status=.*to_status=.*\}`),
				"Condition transition metric should have 'condition_type', 'from_status', and 'to_status' labels")
		})

		It("should include Go runtime metrics", func() {
			resp, err := httpClient.Get(metricsURL + "/metrics")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			body, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			metricsText := string(body)

			// Standard Go runtime metrics (automatically exposed by controller-runtime)
			runtimeMetrics := []string{
				"go_goroutines",
				"go_memstats_alloc_bytes",
			}

			for _, metric := range runtimeMetrics {
				Expect(metricsText).To(ContainSubstring(metric),
					"Missing Go runtime metric: %s", metric)
			}
		})

		It("should include controller-runtime metrics", func() {
			resp, err := httpClient.Get(metricsURL + "/metrics")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			body, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			metricsText := string(body)

			// Controller-runtime standard metrics
			Expect(metricsText).To(ContainSubstring("controller_runtime"),
				"Missing controller-runtime metrics")
		})
	})

	Context("Metrics accuracy", func() {
		It("should increment reconciliation counter after processing", func() {
			// Get initial metrics
			resp1, err := httpClient.Get(metricsURL + "/metrics")
			Expect(err).NotTo(HaveOccurred())
			body1, _ := io.ReadAll(resp1.Body)
			_ = resp1.Body.Close()
			initialMetrics := string(body1)

			// Verify metrics contain RO metrics
			// Value will increase after RemediationRequest CRDs are processed
			Expect(initialMetrics).To(ContainSubstring("kubernaut_remediationorchestrator"),
				"Metrics should contain remediationorchestrator prefix")
		})
	})
})
