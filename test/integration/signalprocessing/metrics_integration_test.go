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

package signalprocessing

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	dto "github.com/prometheus/client_model/go"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
)

// ========================================
// METRICS INTEGRATION TESTS
// Business Requirement: BR-SIGNALPROCESSING-OBSERVABILITY-001
// v2.0: BUSINESS FLOW VALIDATION (not direct method calls)
// ========================================
//
// Integration Test Strategy (per DD-TEST-001 and METRICS_ANTI_PATTERN_TRIAGE):
// ✅ CORRECT: Validate metrics as SIDE EFFECTS of business logic
// ❌ WRONG: Direct calls to metrics methods (spMetrics.IncrementXxx())
//
// These tests verify that metrics are:
// 1. Emitted during actual SignalProcessing CRD reconciliation
// 2. Correctly reflect business outcomes (phase transitions, failures, etc.)
// 3. Available in Prometheus registry after business flows complete
//
// Pattern: CREATE CRD → WAIT FOR RECONCILIATION → VERIFY METRICS
//
// Metrics Tested (per pkg/signalprocessing/metrics/metrics.go):
// - signalprocessing_processing_total{phase, result}
// - signalprocessing_processing_duration_seconds{phase}
// - signalprocessing_enrichment_total{result}
// - signalprocessing_enrichment_duration_seconds{resource_kind}
// - signalprocessing_enrichment_errors_total{error_type}
// ========================================

// DD-TEST-010: Multi-Controller Pattern - Metrics tests now run in parallel
// Each process has its own controller with isolated Prometheus registry
var _ = Describe("Metrics Integration via Business Flows", Label("integration", "metrics"), func() {
	var (
		ctx context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	// Helper to gather all metrics from controller-runtime global registry
	// Per AIAnalysis pattern: Query ctrlmetrics.Registry (controller-runtime global registry)
	// Controller (Process 1) registers metrics with ctrlmetrics.Registry
	// All processes (1-4) can query the same truly global registry (works with --procs=4)
	gatherMetrics := func() (map[string]*dto.MetricFamily, error) {
		families, err := ctrlmetrics.Registry.Gather()
		if err != nil {
			return nil, err
		}
		result := make(map[string]*dto.MetricFamily)
		for _, family := range families {
			result[family.GetName()] = family
		}
		return result, nil
	}

	// Helper to get list of metric names for debugging
	getMetricNames := func(families map[string]*dto.MetricFamily) []string {
		names := make([]string, 0, len(families))
		for name := range families {
			names = append(names, name)
		}
		return names
	}

	// Helper to get counter value with specific labels
	getCounterValue := func(name string, labels map[string]string) float64 {
		families, err := gatherMetrics()
		if err != nil {
			GinkgoWriter.Printf("⚠️  Failed to gather metrics: %v\n", err)
			return 0
		}
		family, exists := families[name]
		if !exists {
			GinkgoWriter.Printf("⚠️  Metric '%s' not found in registry. Available metrics: %v\n", name, getMetricNames(families))
			return 0
		}
		GinkgoWriter.Printf("🔍 Searching for metric '%s' with labels %v\n", name, labels)
		for _, m := range family.GetMetric() {
			labelMatch := true
			for wantKey, wantValue := range labels {
				found := false
				for _, l := range m.GetLabel() {
					if l.GetName() == wantKey && l.GetValue() == wantValue {
						found = true
						break
					}
				}
				if !found {
					labelMatch = false
					break
				}
			}
			if labelMatch && m.GetCounter() != nil {
				return m.GetCounter().GetValue()
			}
		}
		return 0
	}

	// Helper to get histogram sample count
	getHistogramCount := func(name string, labels map[string]string) uint64 {
		families, err := gatherMetrics()
		if err != nil {
			GinkgoWriter.Printf("⚠️  Failed to gather metrics: %v\n", err)
			return 0
		}
		family, exists := families[name]
		if !exists {
			GinkgoWriter.Printf("⚠️  Histogram '%s' not found in registry\n", name)
			return 0
		}
		GinkgoWriter.Printf("🔍 Searching for histogram '%s' with labels %v\n", name, labels)
		for _, m := range family.GetMetric() {
			labelMatch := true
			for wantKey, wantValue := range labels {
				found := false
				for _, l := range m.GetLabel() {
					if l.GetName() == wantKey && l.GetValue() == wantValue {
						found = true
						break
					}
				}
				if !found {
					labelMatch = false
					break
				}
			}
			if labelMatch && m.GetHistogram() != nil {
				return m.GetHistogram().GetSampleCount()
			}
		}
		return 0
	}

	// ========================================
	// PROCESSING METRICS (BR-SIGNALPROCESSING-OBSERVABILITY-001)
	// ========================================
	Context("Processing Metrics via SignalProcessing Lifecycle", func() {
		It("should emit processing metrics during successful Signal lifecycle - BR-SIGNALPROCESSING-OBSERVABILITY-001", func() {
			// 1. Create test infrastructure
			ns := createTestNamespaceWithLabels(fmt.Sprintf("metrics-test-%s", uuid.New().String()[:8]), map[string]string{
				"kubernaut.ai/environment": "production",
			})
			defer deleteTestNamespace(ns)

			podLabels := map[string]string{"app": "metrics-test"}
			_ = createTestPod(ns, "metrics-test-pod", podLabels, nil)

			// 2. Create RemediationRequest (parent)
			targetResource := signalprocessingv1alpha1.ResourceIdentifier{
				Kind:      "Pod",
				Name:      "metrics-test-pod",
				Namespace: ns,
			}
			rr := CreateTestRemediationRequest(fmt.Sprintf("metrics-rr-%s", uuid.New().String()[:8]), ns, ValidTestFingerprints["metrics-001"], "warning", targetResource)
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			// 3. Create SignalProcessing CR (triggers business logic)
			sp := CreateTestSignalProcessingWithParent(fmt.Sprintf("metrics-sp-%s", uuid.New().String()[:8]), ns, rr, ValidTestFingerprints["metrics-001"], targetResource)
			Expect(k8sClient.Create(ctx, sp)).To(Succeed())

			// 4. Wait for business outcome (reconciliation completes)
			Eventually(func() signalprocessingv1alpha1.SignalProcessingPhase {
				var updated signalprocessingv1alpha1.SignalProcessing
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      sp.Name,
					Namespace: sp.Namespace,
				}, &updated)
				if err != nil {
					return ""
				}
				return updated.Status.Phase
			}, 30*time.Second, 500*time.Millisecond).Should(Equal(signalprocessingv1alpha1.PhaseCompleted))

			// 5. Verify processing metrics were emitted as side effects
			Eventually(func() float64 {
				return getCounterValue("signalprocessing_processing_total",
					map[string]string{"phase": "enriching", "result": "success"})
			}, 10*time.Second, 500*time.Millisecond).Should(BeNumerically(">", 0),
				"Controller should emit enriching phase metrics during reconciliation")

			Eventually(func() float64 {
				return getCounterValue("signalprocessing_processing_total",
					map[string]string{"phase": "classifying", "result": "success"})
			}, 10*time.Second, 500*time.Millisecond).Should(BeNumerically(">", 0),
				"Controller should emit classifying phase metrics during reconciliation")

			// 6. Verify duration histogram was populated
			Eventually(func() uint64 {
				return getHistogramCount("signalprocessing_processing_duration_seconds",
					map[string]string{"phase": "enriching"})
			}, 5*time.Second, 500*time.Millisecond).Should(BeNumerically(">", 0),
				"Duration histogram should record enriching phase duration")
		})
	})

	// ========================================
	// ENRICHMENT METRICS (BR-SIGNALPROCESSING-OBSERVABILITY-001)
	// ========================================
	Context("Enrichment Metrics via K8s Resource Processing", func() {
		It("should emit enrichment metrics during Pod enrichment - BR-SIGNALPROCESSING-OBSERVABILITY-001", func() {
			// 1. Create test infrastructure with Pod
			ns := createTestNamespaceWithLabels(fmt.Sprintf("metrics-enrich-%s", uuid.New().String()[:8]), map[string]string{
				"kubernaut.ai/environment": "staging",
			})
			defer deleteTestNamespace(ns)

			podLabels := map[string]string{"app": "enrichment-test"}
			_ = createTestPod(ns, "enrichment-test-pod", podLabels, nil)

			// 2. Create RemediationRequest and SignalProcessing
			targetResource := signalprocessingv1alpha1.ResourceIdentifier{
				Kind:      "Pod",
				Name:      "enrichment-test-pod",
				Namespace: ns,
			}
			rr := CreateTestRemediationRequest(fmt.Sprintf("enrich-rr-%s", uuid.New().String()[:8]), ns, ValidTestFingerprints["enrich-001"], "warning", targetResource)
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			sp := CreateTestSignalProcessingWithParent(fmt.Sprintf("enrich-sp-%s", uuid.New().String()[:8]), ns, rr, ValidTestFingerprints["enrich-001"], targetResource)
			Expect(k8sClient.Create(ctx, sp)).To(Succeed())

			// 3. Wait for enrichment to complete
			Eventually(func() bool {
				var updated signalprocessingv1alpha1.SignalProcessing
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(sp), &updated)
				if err != nil {
					return false
				}
				// Enrichment completes after Enriching phase (transitions to Classifying or beyond)
				return updated.Status.Phase == signalprocessingv1alpha1.PhaseClassifying ||
					updated.Status.Phase == signalprocessingv1alpha1.PhaseCategorizing ||
					updated.Status.Phase == signalprocessingv1alpha1.PhaseCompleted
			}, 30*time.Second, 500*time.Millisecond).Should(BeTrue())

			// 4. Verify enrichment metrics were emitted
			Eventually(func() float64 {
				return getCounterValue("signalprocessing_enrichment_total",
					map[string]string{"result": "success"})
			}, 10*time.Second, 500*time.Millisecond).Should(BeNumerically(">", 0),
				"Enrichment total metric should be emitted during Pod enrichment")

			// 5. Verify enrichment duration histogram
			Eventually(func() uint64 {
				return getHistogramCount("signalprocessing_enrichment_duration_seconds",
					map[string]string{"resource_kind": "pod"})
			}, 5*time.Second, 500*time.Millisecond).Should(BeNumerically(">", 0),
				"Enrichment duration should be recorded for Pod resources")
		})
	})

	// ========================================
	// ERROR HANDLING METRICS (BR-SIGNALPROCESSING-OBSERVABILITY-001)
	// ========================================
	Context("Error Metrics via Failure Scenarios", func() {
		It("should emit error metrics when enrichment encounters missing resources - BR-SIGNALPROCESSING-OBSERVABILITY-001", func() {
			// 1. Create test infrastructure WITHOUT the target Pod (will cause enrichment error)
			ns := createTestNamespaceWithLabels(fmt.Sprintf("metrics-error-%s", uuid.New().String()[:8]), map[string]string{
				"kubernaut.ai/environment": "development",
			})
			defer deleteTestNamespace(ns)

			// 2. Create SignalProcessing targeting non-existent Pod
			targetResource := signalprocessingv1alpha1.ResourceIdentifier{
				Kind:      "Pod",
				Name:      "nonexistent-pod", // Pod doesn't exist
				Namespace: ns,
			}
			rr := CreateTestRemediationRequest(fmt.Sprintf("error-rr-%s", uuid.New().String()[:8]), ns, ValidTestFingerprints["error-001"], "warning", targetResource)
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			sp := CreateTestSignalProcessingWithParent(fmt.Sprintf("error-sp-%s", uuid.New().String()[:8]), ns, rr, ValidTestFingerprints["error-001"], targetResource)
			Expect(k8sClient.Create(ctx, sp)).To(Succeed())

			// 3. Wait for processing to reach an error state or complete (may have degraded enrichment)
			Eventually(func() bool {
				var updated signalprocessingv1alpha1.SignalProcessing
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(sp), &updated)
				if err != nil {
					return false
				}
				// May reach Failed or Completed (enrichment may be partial due to missing resource)
				return updated.Status.Phase == signalprocessingv1alpha1.PhaseFailed ||
					updated.Status.Phase == signalprocessingv1alpha1.PhaseCompleted ||
					(updated.Status.Error != "" && updated.Status.Phase != signalprocessingv1alpha1.PhasePending)
			}, 30*time.Second, 500*time.Millisecond).Should(BeTrue())

			// 4. Verify enrichment error metrics were emitted
			Eventually(func() float64 {
				// Check for various error types that might occur
				notFoundErrors := getCounterValue("signalprocessing_enrichment_errors_total",
					map[string]string{"error_type": "not_found"})
				if notFoundErrors > 0 {
					return notFoundErrors
				}
				apiErrors := getCounterValue("signalprocessing_enrichment_errors_total",
					map[string]string{"error_type": "api_error"})
				return apiErrors
			}, 10*time.Second, 500*time.Millisecond).Should(BeNumerically(">", 0),
				"Enrichment error metrics should be emitted when resources are missing")
		})
	})

	// ========================================
	// NOTE: HTTP Endpoint Tests → E2E (Day 8)
	// ========================================
	// HTTP /metrics endpoint accessibility tests are in E2E tier:
	// - test/e2e/signalprocessing/metrics_test.go
	//
	// Rationale (per DD-TEST-001):
	// - Integration tests use envtest (no HTTP server for CRD controllers)
	// - E2E tests deploy full controller with Service (HTTP endpoint available)
	//
	// E2E validates: "Can operators scrape SignalProcessing metrics in production?"
	// ========================================
})
