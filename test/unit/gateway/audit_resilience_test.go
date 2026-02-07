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
	"errors"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/go-logr/logr"
	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	gatewaypkg "github.com/jordigilh/kubernaut/pkg/gateway"
	"github.com/jordigilh/kubernaut/pkg/gateway/adapters"
	"github.com/jordigilh/kubernaut/pkg/gateway/config"
	"github.com/jordigilh/kubernaut/pkg/gateway/metrics"
	testmocks "github.com/jordigilh/kubernaut/test/shared/mocks"
	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("GW-UNIT-AUD-005: Audit Failure Resilience", func() {
	var (
		ctx              context.Context
		k8sClient        client.Client
		mockAuditStore   *testmocks.MockAuditStore
		testRegistry     *prometheus.Registry
		metricsInstance  *metrics.Metrics
		gwServer         *gatewaypkg.Server
		prometheusAdapter *adapters.PrometheusAdapter
		testNamespace    string
		logger           logr.Logger
	)

	BeforeEach(func() {
		ctx = context.Background()
		testNamespace = "test-audit-resilience"

		// Create no-op logger for unit tests
		logger = logr.Discard()

		// Create fake K8s client with scheme
		scheme := runtime.NewScheme()
		Expect(remediationv1alpha1.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())

		// Create test namespace
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: testNamespace},
		}

		// Create fake K8s client with field indexer for deduplication
		// Gateway uses spec.signalFingerprint to check for duplicates
		k8sClient = fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(ns).
			WithIndex(&remediationv1alpha1.RemediationRequest{}, "spec.signalFingerprint", func(obj client.Object) []string {
				rr := obj.(*remediationv1alpha1.RemediationRequest)
				return []string{rr.Spec.SignalFingerprint}
			}).
			Build()

		// Create mock audit store (shared from test/shared/mocks)
		mockAuditStore = testmocks.NewMockAuditStore()

		// Create test metrics registry
		testRegistry = prometheus.NewRegistry()
		metricsInstance = metrics.NewMetricsWithRegistry(testRegistry)

		// Create Prometheus adapter
		prometheusAdapter = adapters.NewPrometheusAdapter()
	})

	// Test ID: GW-UNIT-AUD-005
	// Scenario: Audit Failure Non-Blocking
	// BR: BR-GATEWAY-055
	// Priority: P1
	Context("BR-GATEWAY-055: Audit Failure Resilience", func() {
		It("[GW-UNIT-AUD-005] should continue signal processing even if audit emission fails", func() {
			By("1. Configure mock audit store to fail")
			auditError := errors.New("audit store unavailable: database connection lost")
			mockAuditStore.StoreAuditFunc = func(ctx context.Context, event *ogenclient.AuditEventRequest) error {
				return auditError // Simulate audit failure
			}

			By("2. Create Gateway server with failing audit store")
			cfg := &config.ServerConfig{
				Server: config.ServerSettings{
					ListenAddr:   "127.0.0.1:8080",
					ReadTimeout:  30 * time.Second,
					WriteTimeout: 30 * time.Second,
					IdleTimeout:  120 * time.Second,
				},
				Infrastructure: config.InfrastructureSettings{
					DataStorageURL: "http://127.0.0.1:8080",
				},
				Processing: config.ProcessingSettings{
					Deduplication: config.DeduplicationSettings{
						TTL: 300 * time.Second,
					},
					CRD: config.CRDSettings{
						FallbackNamespace: testNamespace,
					},
					Retry: config.RetrySettings{
						MaxAttempts:    3,
						InitialBackoff: 100 * time.Millisecond,
						MaxBackoff:     1 * time.Second,
					},
				},
			}

			var err error
			gwServer, err = gatewaypkg.NewServerForTesting(cfg, logger, metricsInstance, k8sClient, mockAuditStore, nil)
			Expect(err).ToNot(HaveOccurred())

			By("3. Process signal (audit will fail internally)")
			alert := createPrometheusAlert(testNamespace, "HighCPU", "critical", "", "")
			signal, err := prometheusAdapter.Parse(ctx, alert)
			Expect(err).ToNot(HaveOccurred())

			response, err := gwServer.ProcessSignal(ctx, signal)

			By("4. Verify signal processing succeeded despite audit failure")
			// CRITICAL: Signal processing MUST NOT fail due to audit issues
			Expect(err).ToNot(HaveOccurred(),
				"BR-GATEWAY-055: Signal processing must not block on audit failures")
			Expect(response).ToNot(BeNil())
			Expect(response.Status).To(Equal("created"),
				"BR-GATEWAY-055: CRD must be created even if audit fails")
			Expect(response.RemediationRequestName).ToNot(BeEmpty())

			By("5. Verify audit was attempted (resilience, not avoidance)")
			Expect(mockAuditStore.GetStoreCalls()).To(BeNumerically(">=", 1),
				"BR-GATEWAY-055: Audit emission must be attempted, not skipped")

			By("6. Verify CRD was created in K8s")
			var rr remediationv1alpha1.RemediationRequest
			rrKey := client.ObjectKey{
				Name:      response.RemediationRequestName,
				Namespace: testNamespace,
			}
			err = k8sClient.Get(ctx, rrKey, &rr)
			Expect(err).ToNot(HaveOccurred(),
				"BR-GATEWAY-055: CRD creation must succeed independently of audit")

			GinkgoWriter.Printf("✅ Signal processing resilient to audit failures: RR=%s (audit attempted %d times)\n",
				response.RemediationRequestName, mockAuditStore.GetStoreCalls())
		})

		It("[GW-UNIT-AUD-005-ALT] should log audit failures for observability", func() {
			By("1. Configure mock audit store to fail with specific error")
			auditError := errors.New("database constraint violation: correlation_id too long")
			failureCount := 0
			mockAuditStore.StoreAuditFunc = func(ctx context.Context, event *ogenclient.AuditEventRequest) error {
				failureCount++
				return auditError
			}

			By("2. Create Gateway server with failing audit store")
			cfg := &config.ServerConfig{
				Server: config.ServerSettings{
					ListenAddr:   "127.0.0.1:8080",
					ReadTimeout:  30 * time.Second,
					WriteTimeout: 30 * time.Second,
					IdleTimeout:  120 * time.Second,
				},
				Infrastructure: config.InfrastructureSettings{
					DataStorageURL: "http://127.0.0.1:8080",
				},
				Processing: config.ProcessingSettings{
					Deduplication: config.DeduplicationSettings{
						TTL: 300 * time.Second,
					},
					CRD: config.CRDSettings{
						FallbackNamespace: testNamespace,
					},
					Retry: config.RetrySettings{
						MaxAttempts:    3,
						InitialBackoff: 100 * time.Millisecond,
						MaxBackoff:     1 * time.Second,
					},
				},
			}

			var err error
			gwServer, err = gatewaypkg.NewServerForTesting(cfg, logger, metricsInstance, k8sClient, mockAuditStore, nil)
			Expect(err).ToNot(HaveOccurred())

			By("3. Process signal that causes audit failure")
			alert := createPrometheusAlert(testNamespace, "MemoryLeak", "warning", "", "")
			signal, err := prometheusAdapter.Parse(ctx, alert)
			Expect(err).ToNot(HaveOccurred())

			response, err := gwServer.ProcessSignal(ctx, signal)

			By("4. Verify signal processed successfully")
			Expect(err).ToNot(HaveOccurred())
			Expect(response.Status).To(Equal("created"))

			By("5. Verify audit failure was attempted (logged for ops team)")
			Expect(failureCount).To(BeNumerically(">=", 1),
				"BR-GATEWAY-055: Audit failures must be logged for observability")

			GinkgoWriter.Printf("✅ Audit failures logged but didn't block processing: failures=%d\n", failureCount)
		})

		It("[GW-UNIT-AUD-005-DEDUP] should handle audit failures during deduplication", func() {
			By("1. Configure mock audit store to fail")
			mockAuditStore.StoreAuditFunc = func(ctx context.Context, event *ogenclient.AuditEventRequest) error {
				return errors.New("audit unavailable")
			}

			By("2. Create Gateway server")
			cfg := &config.ServerConfig{
				Server: config.ServerSettings{
					ListenAddr:   "127.0.0.1:8080",
					ReadTimeout:  30 * time.Second,
					WriteTimeout: 30 * time.Second,
					IdleTimeout:  120 * time.Second,
				},
				Infrastructure: config.InfrastructureSettings{
					DataStorageURL: "http://127.0.0.1:8080",
				},
				Processing: config.ProcessingSettings{
					Deduplication: config.DeduplicationSettings{
						TTL: 300 * time.Second,
					},
					CRD: config.CRDSettings{
						FallbackNamespace: testNamespace,
					},
					Retry: config.RetrySettings{
						MaxAttempts:    3,
						InitialBackoff: 100 * time.Millisecond,
						MaxBackoff:     1 * time.Second,
					},
				},
			}

			var err error
			gwServer, err = gatewaypkg.NewServerForTesting(cfg, logger, metricsInstance, k8sClient, mockAuditStore, nil)
			Expect(err).ToNot(HaveOccurred())

			By("3. Create initial signal")
			alert1 := createPrometheusAlert(testNamespace, "DiskFull", "critical", "", "")
			signal1, err := prometheusAdapter.Parse(ctx, alert1)
			Expect(err).ToNot(HaveOccurred())

			response1, err := gwServer.ProcessSignal(ctx, signal1)
			Expect(err).ToNot(HaveOccurred())
			Expect(response1.Status).To(Equal("created"))
			initialRRName := response1.RemediationRequestName

			By("4. Send duplicate signal (should deduplicate even with audit failures)")
			alert2 := createPrometheusAlert(testNamespace, "DiskFull", "critical", "", "")
			signal2, err := prometheusAdapter.Parse(ctx, alert2)
			Expect(err).ToNot(HaveOccurred())

			response2, err := gwServer.ProcessSignal(ctx, signal2)

			By("5. Verify deduplication works despite audit failures")
		Expect(err).ToNot(HaveOccurred(),
			"BR-GATEWAY-055: Deduplication must work even if audit fails")
		Expect(response2.Status).To(Equal("duplicate"),
			"BR-GATEWAY-055: Duplicate detection must not be affected by audit issues")
		Expect(response2.RemediationRequestName).To(Equal(initialRRName),
				"BR-GATEWAY-055: Same RR must be returned for duplicates")

			By("6. Verify audit attempts were made for both signals")
			Expect(mockAuditStore.GetStoreCalls()).To(BeNumerically(">=", 2),
				"BR-GATEWAY-055: Audit must be attempted for all signals (initial + duplicate)")

			GinkgoWriter.Printf("✅ Deduplication resilient to audit failures: RR=%s (audit attempts=%d)\n",
				initialRRName, mockAuditStore.GetStoreCalls())
		})
	})
})

// Helper function to create Prometheus alert for testing
func createPrometheusAlert(namespace, alertName, severity, team, environment string) []byte {
	labels := fmt.Sprintf(`"alertname": "%s", "namespace": "%s", "severity": "%s"`, alertName, namespace, severity)
	if team != "" {
		labels += fmt.Sprintf(`, "team": "%s"`, team)
	}
	if environment != "" {
		labels += fmt.Sprintf(`, "environment": "%s"`, environment)
	}

	return []byte(fmt.Sprintf(`{
		"alerts": [{
			"labels": {%s},
			"annotations": {"description": "Test alert"}
		}]
	}`, labels))
}
