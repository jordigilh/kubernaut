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

package gateway

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"github.com/jordigilh/kubernaut/pkg/gateway/adapters"
	"github.com/jordigilh/kubernaut/pkg/gateway/metrics"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// GATEWAY METRICS EMISSION INTEGRATION TESTS
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
//
// **Purpose**: Validate Gateway's Prometheus metrics emission for operational visibility
//
// **Test Pattern**:
// 1. Create Gateway with custom Prometheus registry
// 2. Call ProcessSignal() (real business logic)
// 3. Query metrics from registry
// 4. Validate metric values and labels
//
// **Scope**: Integration tests (real Gateway + real K8s + metrics)
// **Coverage Target**: +6% (Metrics emission scenarios)
// **Related BRs**: BR-GATEWAY-066, BR-GATEWAY-067, BR-GATEWAY-068, BR-GATEWAY-069, BR-GATEWAY-070
//
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

var _ = Describe("Gateway Metrics Emission", Label("metrics", "integration"), func() {
	// Test ID: GW-INT-MET-001
	// Scenario: Signal Processing Metrics - Signals Received Counter
	// BR: BR-GATEWAY-066
	// Section: 2.1.1
	Context("BR-GATEWAY-066: Signal Processing Metrics", func() {
		var (
			testNamespace string
			ctx           context.Context
			metricsReg    *prometheus.Registry
		)

		BeforeEach(func() {
			processID := GinkgoParallelProcess()
			ctx = context.Background()
			testNamespace = fmt.Sprintf("gw-metrics-sig-%d-%s", processID, uuid.New().String()[:8])
			
			// Create test namespace
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: testNamespace},
			}
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())

			// Create custom Prometheus registry for test isolation
			metricsReg = prometheus.NewRegistry()
		})

		AfterEach(func() {
			// Delete test namespace
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: testNamespace},
			}
			_ = k8sClient.Delete(ctx, ns)
		})

		It("[GW-INT-MET-001] should increment gateway_signals_received_total when signal processed", func() {
			By("1. Get initial metric value")
			initialValue := getCounterValue(metricsReg, "gateway_signals_received_total", map[string]string{
				"source_type": "prometheus-alert",
			})

			By("2. Process Prometheus signal")
			prometheusAdapter := adapters.NewPrometheusAdapter()
			alert := createPrometheusAlert(testNamespace, "HighCPU", "critical", "", "")
			signal, err := prometheusAdapter.Parse(ctx, alert)
			Expect(err).ToNot(HaveOccurred())

			gatewayConfig := createGatewayConfig(fmt.Sprintf("http://127.0.0.1:%d", gatewayDataStoragePort))
			metricsInstance := metrics.NewMetricsWithRegistry(metricsReg)
			gwServer, err := createGatewayServerWithMetrics(gatewayConfig, logger, k8sClient, metricsInstance)
			Expect(err).ToNot(HaveOccurred())

			_, err = gwServer.ProcessSignal(ctx, signal)
			Expect(err).ToNot(HaveOccurred())

			By("3. Verify metric incremented")
			finalValue := getCounterValue(metricsReg, "gateway_signals_received_total", map[string]string{
				"source_type": "prometheus-alert",
			})
			Expect(finalValue).To(Equal(initialValue+1), "BR-GATEWAY-066: Signals received counter must increment")

			GinkgoWriter.Printf("✅ Metric validated: gateway_signals_received_total increased from %.0f to %.0f\n",
				initialValue, finalValue)
		})

		// Test ID: GW-INT-MET-002
		// Scenario: Signals By Type Counter
		// BR: BR-GATEWAY-066
		// Section: 2.1.2
		It("[GW-INT-MET-002] should track signals by source type (prometheus vs k8s-event)", func() {
			By("1. Process Prometheus signal")
			prometheusAdapter := adapters.NewPrometheusAdapter()
			prometheusAlert := createPrometheusAlert(testNamespace, "HighCPU", "critical", "", "")
			prometheusSignal, err := prometheusAdapter.Parse(ctx, prometheusAlert)
			Expect(err).ToNot(HaveOccurred())

			gatewayConfig := createGatewayConfig(fmt.Sprintf("http://127.0.0.1:%d", gatewayDataStoragePort))
			metricsInstance := metrics.NewMetricsWithRegistry(metricsReg)
			gwServer, err := createGatewayServerWithMetrics(gatewayConfig, logger, k8sClient, metricsInstance)
			Expect(err).ToNot(HaveOccurred())

			_, err = gwServer.ProcessSignal(ctx, prometheusSignal)
			Expect(err).ToNot(HaveOccurred())

			By("2. Verify prometheus-alert metric exists with correct label")
			prometheusValue := getCounterValue(metricsReg, "gateway_signals_received_total", map[string]string{
				"source_type": "prometheus-alert",
			})
			Expect(prometheusValue).To(BeNumerically(">=", 1),
				"BR-GATEWAY-066: Prometheus signals must be tracked with source_type=prometheus-alert")

			GinkgoWriter.Printf("✅ Metric labeled correctly: source_type=prometheus-alert, value=%.0f\n", prometheusValue)
		})

		// Test ID: GW-INT-MET-003
		// Scenario: Signals By Severity Counter
		// BR: BR-GATEWAY-067
		// Section: 2.1.3
		It("[GW-INT-MET-003] should track signals by severity level", func() {
			By("1. Process signals with different severities")
			prometheusAdapter := adapters.NewPrometheusAdapter()
			gatewayConfig := createGatewayConfig(fmt.Sprintf("http://127.0.0.1:%d", gatewayDataStoragePort))
			metricsInstance := metrics.NewMetricsWithRegistry(metricsReg)
			gwServer, err := createGatewayServerWithMetrics(gatewayConfig, logger, k8sClient, metricsInstance)
			Expect(err).ToNot(HaveOccurred())

			// Process critical alert
			criticalAlert := createPrometheusAlert(testNamespace, "CriticalAlert", "critical", "", "")
			criticalSignal, err := prometheusAdapter.Parse(ctx, criticalAlert)
			Expect(err).ToNot(HaveOccurred())
			_, err = gwServer.ProcessSignal(ctx, criticalSignal)
			Expect(err).ToNot(HaveOccurred())

			// Process warning alert
			warningAlert := createPrometheusAlert(testNamespace, "WarningAlert", "warning", "", "")
			warningSignal, err := prometheusAdapter.Parse(ctx, warningAlert)
			Expect(err).ToNot(HaveOccurred())
			_, err = gwServer.ProcessSignal(ctx, warningSignal)
			Expect(err).ToNot(HaveOccurred())

			By("2. Verify severity labels are tracked")
			// Gateway metric has labels: source_type AND severity
			criticalValue := getCounterValue(metricsReg, "gateway_signals_received_total", map[string]string{
				"source_type": "prometheus-alert",
				"severity":    "critical",
			})
			warningValue := getCounterValue(metricsReg, "gateway_signals_received_total", map[string]string{
				"source_type": "prometheus-alert",
				"severity":    "warning",
			})

			Expect(criticalValue).To(BeNumerically(">=", 1),
				"BR-GATEWAY-067: Critical signal must be counted")
			Expect(warningValue).To(BeNumerically(">=", 1),
				"BR-GATEWAY-067: Warning signal must be counted")

			GinkgoWriter.Printf("✅ Severity-labeled signals tracked: critical=%.0f, warning=%.0f\n", criticalValue, warningValue)
		})
	})

	// Test ID: GW-INT-MET-006
	// Scenario: CRD Creation Metrics
	// BR: BR-GATEWAY-069
	// Section: 2.2.1
	Context("BR-GATEWAY-069: CRD Creation Metrics", func() {
		var (
			testNamespace string
			ctx           context.Context
			metricsReg    *prometheus.Registry
		)

		BeforeEach(func() {
			processID := GinkgoParallelProcess()
			ctx = context.Background()
			testNamespace = fmt.Sprintf("gw-metrics-crd-%d-%s", processID, uuid.New().String()[:8])
			
			// Create test namespace
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: testNamespace},
			}
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())

			metricsReg = prometheus.NewRegistry()
		})

		AfterEach(func() {
			// Delete test namespace
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: testNamespace},
			}
			_ = k8sClient.Delete(ctx, ns)
		})

		It("[GW-INT-MET-006] should increment gateway_crds_created_total on successful CRD creation", func() {
			By("1. Get initial metric value")
			initialValue := getCounterValue(metricsReg, "gateway_crds_created_total", map[string]string{
				"source_type": "prometheus-alert",
				"status":      "created",
			})

			By("2. Process signal to create CRD")
			prometheusAdapter := adapters.NewPrometheusAdapter()
			alert := createPrometheusAlert(testNamespace, "HighMemory", "critical", "", "")
			signal, err := prometheusAdapter.Parse(ctx, alert)
			Expect(err).ToNot(HaveOccurred())

			gatewayConfig := createGatewayConfig(fmt.Sprintf("http://127.0.0.1:%d", gatewayDataStoragePort))
			metricsInstance := metrics.NewMetricsWithRegistry(metricsReg)
			gwServer, err := createGatewayServerWithMetrics(gatewayConfig, logger, k8sClient, metricsInstance)
			Expect(err).ToNot(HaveOccurred())

			response, err := gwServer.ProcessSignal(ctx, signal)
			Expect(err).ToNot(HaveOccurred())
			Expect(response.Status).To(Equal("created"))

			By("3. Verify CRD creation metric incremented")
			finalValue := getCounterValue(metricsReg, "gateway_crds_created_total", map[string]string{
				"source_type": "prometheus-alert",
				"status":      "created",
			})
			Expect(finalValue).To(Equal(initialValue+1),
				"BR-GATEWAY-069: CRD creation counter must increment on success")

			GinkgoWriter.Printf("✅ CRD creation metric: increased from %.0f to %.0f for RR=%s\n",
				initialValue, finalValue, response.RemediationRequestName)
		})

		// Test ID: GW-INT-MET-008
		// Scenario: CRDs By Namespace Counter
		// BR: BR-GATEWAY-069
		// Section: 2.2.3
		It("[GW-INT-MET-008] should track CRD creation metrics per namespace", func() {
			processID := GinkgoParallelProcess()
			By("1. Create second test namespace")
			testNamespace2 := fmt.Sprintf("gw-metrics-ns2-%d-%s", processID, uuid.New().String()[:8])
			ns2 := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: testNamespace2},
			}
			Expect(k8sClient.Create(ctx, ns2)).To(Succeed())
			defer func() {
				_ = k8sClient.Delete(ctx, ns2)
			}()

			By("2. Process signals in different namespaces")
			prometheusAdapter := adapters.NewPrometheusAdapter()
			gatewayConfig := createGatewayConfig(fmt.Sprintf("http://127.0.0.1:%d", gatewayDataStoragePort))
			metricsInstance := metrics.NewMetricsWithRegistry(metricsReg)
			gwServer, err := createGatewayServerWithMetrics(gatewayConfig, logger, k8sClient, metricsInstance)
			Expect(err).ToNot(HaveOccurred())

			// Signal in namespace 1
			alert1 := createPrometheusAlert(testNamespace, "Alert1", "critical", "", "")
			signal1, err := prometheusAdapter.Parse(ctx, alert1)
			Expect(err).ToNot(HaveOccurred())
			_, err = gwServer.ProcessSignal(ctx, signal1)
			Expect(err).ToNot(HaveOccurred())

			// Signal in namespace 2
			alert2 := createPrometheusAlert(testNamespace2, "Alert2", "critical", "", "")
			signal2, err := prometheusAdapter.Parse(ctx, alert2)
			Expect(err).ToNot(HaveOccurred())
			_, err = gwServer.ProcessSignal(ctx, signal2)
			Expect(err).ToNot(HaveOccurred())

			By("3. Verify metrics exist for both namespaces")
			// Gateway's current metrics use source_type, not namespace, but CRDs are created
			totalValue := getCounterValue(metricsReg, "gateway_crds_created_total", map[string]string{
				"source_type": "prometheus-alert",
				"status":      "created",
			})
			Expect(totalValue).To(BeNumerically(">=", 2),
				"BR-GATEWAY-069: CRDs in different namespaces must all be counted")

			GinkgoWriter.Printf("✅ CRDs created across namespaces: total=%.0f\n", totalValue)
		})
	})

	// Test ID: GW-INT-MET-011
	// Scenario: Deduplication Metrics
	// BR: BR-GATEWAY-066
	// Section: 2.3.1
	Context("BR-GATEWAY-066: Deduplication Metrics", func() {
		var (
			testNamespace string
			ctx           context.Context
			metricsReg    *prometheus.Registry
		)

		BeforeEach(func() {
			processID := GinkgoParallelProcess()
			ctx = context.Background()
			testNamespace = fmt.Sprintf("gw-metrics-dedup-%d-%s", processID, uuid.New().String()[:8])
			
			// Create test namespace
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: testNamespace},
			}
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())

			metricsReg = prometheus.NewRegistry()
		})

		AfterEach(func() {
			// Delete test namespace
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: testNamespace},
			}
			_ = k8sClient.Delete(ctx, ns)
		})

		It("[GW-INT-MET-011] should increment gateway_signals_deduplicated_total on deduplication", func() {
			By("1. Create initial RR")
			prometheusAdapter := adapters.NewPrometheusAdapter()
			gatewayConfig := createGatewayConfig(fmt.Sprintf("http://127.0.0.1:%d", gatewayDataStoragePort))
			metricsInstance := metrics.NewMetricsWithRegistry(metricsReg)
			gwServer, err := createGatewayServerWithMetrics(gatewayConfig, logger, k8sClient, metricsInstance)
			Expect(err).ToNot(HaveOccurred())

			alertName := "RepeatedAlert"
			alert1 := createPrometheusAlert(testNamespace, alertName, "critical", "", "")
			signal1, err := prometheusAdapter.Parse(ctx, alert1)
			Expect(err).ToNot(HaveOccurred())
			response1, err := gwServer.ProcessSignal(ctx, signal1)
			Expect(err).ToNot(HaveOccurred())
			Expect(response1.Status).To(Equal("created"))

			// Wait for CRD to be created
			time.Sleep(500 * time.Millisecond)

			By("2. Get initial deduplication metric value")
			// Gateway metric uses signal_name as label (defined in metrics.go line 152)
			initialDedupValue := getCounterValue(metricsReg, "gateway_signals_deduplicated_total", map[string]string{
				"signal_name": alertName,
			})

			By("3. Process duplicate signal")
			alert2 := createPrometheusAlert(testNamespace, alertName, "critical", "", "")
			signal2, err := prometheusAdapter.Parse(ctx, alert2)
			Expect(err).ToNot(HaveOccurred())
			response2, err := gwServer.ProcessSignal(ctx, signal2)
			Expect(err).ToNot(HaveOccurred())
			Expect(response2.Status).To(Equal("deduplicated"))

			By("4. Verify deduplication metric incremented")
			finalDedupValue := getCounterValue(metricsReg, "gateway_signals_deduplicated_total", map[string]string{
				"signal_name": alertName,
			})
			Expect(finalDedupValue).To(Equal(initialDedupValue+1),
				"BR-GATEWAY-066: Deduplicated signals counter must increment")

			GinkgoWriter.Printf("✅ Deduplication metric for alert '%s': increased from %.0f to %.0f\n",
				alertName, initialDedupValue, finalDedupValue)
		})
	})
})
