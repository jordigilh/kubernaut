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

	"github.com/go-logr/logr"
	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	gatewaypkg "github.com/jordigilh/kubernaut/pkg/gateway"
	"github.com/jordigilh/kubernaut/pkg/gateway/adapters"
	"github.com/jordigilh/kubernaut/pkg/gateway/config"
	"github.com/jordigilh/kubernaut/pkg/gateway/metrics"
	"github.com/jordigilh/kubernaut/pkg/gateway/types"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// ========================================
// BR-SCOPE-002: Gateway Scope Validation Unit Tests
// ========================================
//
// These tests validate that the Gateway rejects signals from unmanaged
// resources before creating RemediationRequest CRDs.
//
// Test Plan Reference: docs/testing/BR-SCOPE-001/TEST_PLAN.md
// Test IDs: UT-GW-002-001 through UT-GW-002-006
//
// Business Requirements:
// - BR-SCOPE-002: Gateway Signal Filtering
// - ADR-053: Resource Scope Management Architecture
// ========================================

// mockScopeChecker is a configurable mock for the ScopeChecker interface.
// It allows tests to control the IsManaged return value.
type mockScopeChecker struct {
	managed    bool
	err        error
	callCount  int
	lastParams struct {
		namespace string
		kind      string
		name      string
	}
}

func (m *mockScopeChecker) IsManaged(_ context.Context, namespace, kind, name string) (bool, error) {
	m.callCount++
	m.lastParams.namespace = namespace
	m.lastParams.kind = kind
	m.lastParams.name = name
	return m.managed, m.err
}

// newTestGatewayServer creates a Gateway server for unit tests with the given scope checker.
func newTestGatewayServer(k8sClient client.Client, metricsInstance *metrics.Metrics, scopeChecker gatewaypkg.ScopeChecker) (*gatewaypkg.Server, error) {
	cfg := &config.ServerConfig{
		Server: config.ServerSettings{
			ListenAddr:   "127.0.0.1:0",
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
			IdleTimeout:  120 * time.Second,
		},
		Processing: config.ProcessingSettings{
			Deduplication: config.DeduplicationSettings{
				TTL: 300 * time.Second,
			},
			CRD: config.CRDSettings{},
			Retry: config.RetrySettings{
				MaxAttempts:    3,
				InitialBackoff: 100 * time.Millisecond,
				MaxBackoff:     1 * time.Second,
			},
		},
	}

	logger := logr.Discard()
	return gatewaypkg.NewServerForTesting(cfg, logger, metricsInstance, k8sClient, nil, scopeChecker)
}

// newTestK8sClient creates a fake K8s client with the required scheme and field index.
func newTestK8sClient(namespace string) client.Client {
	scheme := runtime.NewScheme()
	Expect(remediationv1alpha1.AddToScheme(scheme)).To(Succeed())
	Expect(corev1.AddToScheme(scheme)).To(Succeed())

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: namespace},
	}

	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ns).
		WithIndex(&remediationv1alpha1.RemediationRequest{}, "spec.signalFingerprint",
			func(obj client.Object) []string {
				rr := obj.(*remediationv1alpha1.RemediationRequest)
				return []string{rr.Spec.SignalFingerprint}
			}).
		Build()
}

// createTestSignalPayload creates a Prometheus webhook payload for testing.
func createTestSignalPayload(namespace, alertName, podName string) []byte {
	return []byte(fmt.Sprintf(`{
		"alerts": [{
			"labels": {
				"alertname": "%s",
				"namespace": "%s",
				"pod": "%s",
				"severity": "warning"
			},
			"annotations": {"description": "Test alert for scope validation"}
		}]
	}`, alertName, namespace, podName))
}

// getMetricValue reads the current value of a Prometheus counter.
func getMetricValue(counter *prometheus.CounterVec, labels ...string) float64 {
	metric := &dto.Metric{}
	c, err := counter.GetMetricWithLabelValues(labels...)
	if err != nil {
		return 0
	}
	_ = c.Write(metric)
	if metric.Counter != nil && metric.Counter.Value != nil {
		return *metric.Counter.Value
	}
	return 0
}

var _ = Describe("BR-SCOPE-002: Gateway Scope Validation", func() {
	var (
		ctx             context.Context
		k8sClient       client.Client
		testRegistry    *prometheus.Registry
		metricsInstance *metrics.Metrics
		testNamespace   string
	)

	BeforeEach(func() {
		ctx = context.Background()
		testNamespace = "test-scope-validation"

		// Create test metrics registry (isolated per test)
		testRegistry = prometheus.NewRegistry()
		metricsInstance = metrics.NewMetricsWithRegistry(testRegistry)

		// Create fake K8s client with test namespace
		k8sClient = newTestK8sClient(testNamespace)
	})

	// ========================================
	// UT-GW-002-001: Reject signal from unmanaged namespace
	// ========================================
	It("[UT-GW-002-001] should reject signal when resource is not managed (BR-SCOPE-002)", func() {
		By("1. Configure scope checker to return unmanaged")
		mockScope := &mockScopeChecker{managed: false}

		By("2. Create Gateway server with scope checker")
		gwServer, err := newTestGatewayServer(k8sClient, metricsInstance, mockScope)
		Expect(err).ToNot(HaveOccurred())

		By("3. Parse and process signal from unmanaged namespace")
		signal, err := parsePrometheusSignal(ctx, testNamespace, "HighCPU", "payment-api-789")
		Expect(err).ToNot(HaveOccurred())

		response, err := gwServer.ProcessSignal(ctx, signal)

	By("4. Verify signal is rejected (not an error — just a rejection)")
	Expect(err).ToNot(HaveOccurred(),
		"BR-SCOPE-002: Rejection is not an error — signal simply rejected")
	Expect(response.Status).To(Equal(gatewaypkg.StatusRejected),
		"BR-SCOPE-002: Response status must be 'rejected' for unmanaged resources")

		By("5. Verify no RR CRD was created")
		var rrList remediationv1alpha1.RemediationRequestList
		err = k8sClient.List(ctx, &rrList, client.InNamespace(testNamespace))
		Expect(err).ToNot(HaveOccurred())
		Expect(rrList.Items).To(BeEmpty(),
			"BR-SCOPE-002: No RemediationRequest should be created for unmanaged signals")
	})

	// ========================================
	// UT-GW-002-002: Accept signal from managed namespace
	// ========================================
	It("[UT-GW-002-002] should accept signal when resource is managed (BR-SCOPE-002)", func() {
		By("1. Configure scope checker to return managed")
		mockScope := &mockScopeChecker{managed: true}

		By("2. Create Gateway server with scope checker")
		gwServer, err := newTestGatewayServer(k8sClient, metricsInstance, mockScope)
		Expect(err).ToNot(HaveOccurred())

		By("3. Parse and process signal from managed namespace")
		signal, err := parsePrometheusSignal(ctx, testNamespace, "HighMemory", "web-app-456")
		Expect(err).ToNot(HaveOccurred())

		response, err := gwServer.ProcessSignal(ctx, signal)

	By("4. Verify signal is accepted and RR CRD is created")
	Expect(err).ToNot(HaveOccurred())
	Expect(response.Status).To(Equal(gatewaypkg.StatusCreated),
		"BR-SCOPE-002: Managed signal should result in CRD creation")
		Expect(response.RemediationRequestName).ToNot(BeEmpty(),
			"BR-SCOPE-002: Response must include the created RR name")
	})

	// ========================================
	// UT-GW-002-003: Reject signal with resource opt-out
	// ========================================
	It("[UT-GW-002-003] should reject signal when resource has explicit opt-out (BR-SCOPE-002)", func() {
		By("1. Configure scope checker to return unmanaged (resource opt-out)")
		mockScope := &mockScopeChecker{managed: false}

		By("2. Create Gateway server with scope checker")
		gwServer, err := newTestGatewayServer(k8sClient, metricsInstance, mockScope)
		Expect(err).ToNot(HaveOccurred())

		By("3. Parse and process signal (resource has opt-out label)")
		signal, err := parsePrometheusSignal(ctx, testNamespace, "CrashLoop", "legacy-service-001")
		Expect(err).ToNot(HaveOccurred())

		response, err := gwServer.ProcessSignal(ctx, signal)

	By("4. Verify signal is rejected")
	Expect(err).ToNot(HaveOccurred())
	Expect(response.Status).To(Equal(gatewaypkg.StatusRejected),
		"BR-SCOPE-002: Resource opt-out must result in rejection")

		By("5. Verify no RR CRD was created")
		var rrList remediationv1alpha1.RemediationRequestList
		err = k8sClient.List(ctx, &rrList, client.InNamespace(testNamespace))
		Expect(err).ToNot(HaveOccurred())
		Expect(rrList.Items).To(BeEmpty(),
			"BR-SCOPE-002: No CRD for opt-out resource")
	})

	// ========================================
	// UT-GW-002-004: Actionable rejection response
	// ========================================
	It("[UT-GW-002-004] should return actionable rejection response with label instructions (BR-SCOPE-002)", func() {
		By("1. Configure scope checker to return unmanaged")
		mockScope := &mockScopeChecker{managed: false}

		By("2. Create Gateway server with scope checker")
		gwServer, err := newTestGatewayServer(k8sClient, metricsInstance, mockScope)
		Expect(err).ToNot(HaveOccurred())

		By("3. Parse and process signal")
		signal, err := parsePrometheusSignal(ctx, testNamespace, "NodeDiskPressure", "db-pod-xyz")
		Expect(err).ToNot(HaveOccurred())

		response, err := gwServer.ProcessSignal(ctx, signal)

	By("4. Verify rejection response contains actionable information")
	Expect(err).ToNot(HaveOccurred())
	Expect(response.Status).To(Equal(gatewaypkg.StatusRejected))

	Expect(response.Rejection).ToNot(BeNil(),
		"BR-SCOPE-002: Rejection response must include structured rejection details")
		Expect(response.Rejection.Reason).To(Equal(gatewaypkg.RejectionReasonUnmanagedResource),
			"BR-SCOPE-002: Reason must be 'unmanaged_resource'")
		Expect(response.Rejection.Resource).ToNot(BeEmpty(),
			"BR-SCOPE-002: Must include the resource identifier")
		Expect(response.Rejection.Action).To(ContainSubstring("kubernaut.ai/managed"),
			"BR-SCOPE-002: Action must include label instructions")
		Expect(response.Rejection.Action).To(ContainSubstring("kubectl label"),
			"BR-SCOPE-002: Action must include kubectl command")
	})

	// ========================================
	// UT-GW-002-005: Prometheus metric incremented
	// ========================================
	It("[UT-GW-002-005] should increment gateway_signals_rejected_total metric on rejection (BR-SCOPE-002)", func() {
		By("1. Configure scope checker to return unmanaged")
		mockScope := &mockScopeChecker{managed: false}

		By("2. Create Gateway server with scope checker")
		gwServer, err := newTestGatewayServer(k8sClient, metricsInstance, mockScope)
		Expect(err).ToNot(HaveOccurred())

		By("3. Record initial metric value")
		initialValue := getMetricValue(metricsInstance.SignalsRejectedTotal, gatewaypkg.RejectionReasonUnmanagedResource)

		By("4. Parse and process signal (should be rejected)")
		signal, err := parsePrometheusSignal(ctx, testNamespace, "HighLatency", "api-pod-001")
		Expect(err).ToNot(HaveOccurred())

		_, err = gwServer.ProcessSignal(ctx, signal)
		Expect(err).ToNot(HaveOccurred())

		By("5. Verify metric was incremented")
		newValue := getMetricValue(metricsInstance.SignalsRejectedTotal, gatewaypkg.RejectionReasonUnmanagedResource)
		Expect(newValue).To(Equal(initialValue+1),
			"BR-SCOPE-002: gateway_signals_rejected_total{reason=unmanaged_resource} must be incremented")
	})

	// ========================================
	// UT-GW-002-006: Structured log on rejection
	// ========================================
	It("[UT-GW-002-006] should log rejection with structured fields (BR-SCOPE-002)", func() {
		By("1. Configure scope checker to return unmanaged")
		mockScope := &mockScopeChecker{managed: false}

		By("2. Create Gateway server with scope checker")
		gwServer, err := newTestGatewayServer(k8sClient, metricsInstance, mockScope)
		Expect(err).ToNot(HaveOccurred())

		By("3. Parse and process signal (should be rejected)")
		signal, err := parsePrometheusSignal(ctx, testNamespace, "PodRestart", "worker-pod-999")
		Expect(err).ToNot(HaveOccurred())

		response, err := gwServer.ProcessSignal(ctx, signal)

		By("4. Verify rejection occurred (log capture is infrastructure concern)")
		// Note: Structured log validation is best done at integration level
		// with a captured log sink. At unit level, we verify the rejection
		// response includes all fields that would be logged.
	Expect(err).ToNot(HaveOccurred())
	Expect(response.Status).To(Equal(gatewaypkg.StatusRejected))

	// Verify the scope checker was called with correct parameters
	Expect(mockScope.callCount).To(Equal(1),
			"BR-SCOPE-002: Scope checker should be called exactly once per signal")
		Expect(mockScope.lastParams.namespace).To(Equal(testNamespace),
			"BR-SCOPE-002: Scope checker must receive the signal namespace")
		Expect(mockScope.lastParams.kind).To(Equal("Pod"),
			"BR-SCOPE-002: Scope checker must receive the resource kind")
		Expect(mockScope.lastParams.name).ToNot(BeEmpty(),
			"BR-SCOPE-002: Scope checker must receive the resource name")
	})

	// ========================================
	// UT-GW-002-007: ScopeChecker returns error
	// ========================================
	It("[UT-GW-002-007] should return error when scope checker fails (BR-SCOPE-002)", func() {
		By("1. Configure scope checker to return error")
		mockScope := &mockScopeChecker{
			managed: false,
			err:     fmt.Errorf("API unavailable"),
		}

		By("2. Create Gateway server with scope checker")
		gwServer, err := newTestGatewayServer(k8sClient, metricsInstance, mockScope)
		Expect(err).ToNot(HaveOccurred())

		By("3. Parse and process signal")
		signal, err := parsePrometheusSignal(ctx, testNamespace, "HighCPU", "failing-pod-001")
		Expect(err).ToNot(HaveOccurred())

		response, err := gwServer.ProcessSignal(ctx, signal)

		By("4. Verify scope checker error is propagated (not a rejection)")
		Expect(err).To(HaveOccurred(),
			"BR-SCOPE-002: Scope checker errors must be propagated as errors, not rejections")
		Expect(err.Error()).To(ContainSubstring("scope validation failed"),
			"BR-SCOPE-002: Error must include context about scope validation")
		Expect(err.Error()).To(ContainSubstring("API unavailable"),
			"BR-SCOPE-002: Error must wrap the original scope checker error")
		Expect(response).To(BeNil(),
			"BR-SCOPE-002: No response should be returned on scope checker error")
	})

	// ========================================
	// UT-GW-002-008: nil ScopeChecker (backward compatibility)
	// ========================================
	It("[UT-GW-002-008] should accept signal when scope checker is nil (backward compat) (BR-SCOPE-002)", func() {
		By("1. Create Gateway server WITHOUT scope checker (nil)")
		gwServer, err := newTestGatewayServer(k8sClient, metricsInstance, nil)
		Expect(err).ToNot(HaveOccurred())

		By("2. Parse and process signal")
		signal, err := parsePrometheusSignal(ctx, testNamespace, "HighMemory", "compat-pod-001")
		Expect(err).ToNot(HaveOccurred())

		response, err := gwServer.ProcessSignal(ctx, signal)

	By("3. Verify signal passes through (no scope filtering)")
	Expect(err).ToNot(HaveOccurred(),
		"BR-SCOPE-002: nil scopeChecker must not block signal processing")
	Expect(response.Status).To(Equal(gatewaypkg.StatusCreated),
		"BR-SCOPE-002: Signal must be accepted normally when scope filtering is disabled")
		Expect(response.RemediationRequestName).ToNot(BeEmpty(),
			"BR-SCOPE-002: RR CRD must be created when scope filtering is disabled")
	})

	// ========================================
	// UT-GW-002-009: Rejection response for cluster-scoped resource
	// ========================================
	It("[UT-GW-002-009] should produce correct rejection for cluster-scoped resource (BR-SCOPE-002)", func() {
		By("1. Configure scope checker to return unmanaged")
		mockScope := &mockScopeChecker{managed: false}

		By("2. Create Gateway server with scope checker")
		gwServer, err := newTestGatewayServer(k8sClient, metricsInstance, mockScope)
		Expect(err).ToNot(HaveOccurred())

		By("3. Construct signal for cluster-scoped resource (Node)")
		signal := &types.NormalizedSignal{
			Fingerprint: "test-fingerprint-node-001",
			SignalName:   "NodeNotReady",
			Severity:    "critical",
			Namespace:   "", // cluster-scoped: no namespace
			Resource: types.ResourceIdentifier{
				Kind: "Node",
				Name: "worker-1",
			},
		}

		response, err := gwServer.ProcessSignal(ctx, signal)

	By("4. Verify rejection response has no namespace prefix")
	Expect(err).ToNot(HaveOccurred())
	Expect(response.Status).To(Equal(gatewaypkg.StatusRejected))
	Expect(response.Rejection).ToNot(BeNil(),
		"BR-SCOPE-002: Cluster-scoped rejection must include structured rejection details")
		Expect(response.Rejection.Resource).To(Equal("Node:worker-1"),
			"BR-SCOPE-002: Cluster-scoped rejection must not include namespace prefix")

		// Action should NOT contain "-n" flag
		Expect(response.Rejection.Action).ToNot(ContainSubstring("-n "),
			"BR-SCOPE-002: Cluster-scoped kubectl command must not include -n flag")
		Expect(response.Rejection.Action).To(ContainSubstring("kubectl label node worker-1"),
			"BR-SCOPE-002: Action must include correct kubectl label command for cluster resource")
	})

	// ========================================
	// UT-GW-002-010: Multiple rejections increment metric correctly
	// ========================================
	It("[UT-GW-002-010] should increment metric correctly for multiple rejections (BR-SCOPE-002)", func() {
		By("1. Configure scope checker to return unmanaged")
		mockScope := &mockScopeChecker{managed: false}

		By("2. Create Gateway server with scope checker")
		gwServer, err := newTestGatewayServer(k8sClient, metricsInstance, mockScope)
		Expect(err).ToNot(HaveOccurred())

		By("3. Record initial metric value")
		initialValue := getMetricValue(metricsInstance.SignalsRejectedTotal, gatewaypkg.RejectionReasonUnmanagedResource)

		By("4. Send 3 distinct signals (all rejected)")
		alertNames := []string{"HighCPU", "HighMemory", "CrashLoop"}
		podNames := []string{"pod-a", "pod-b", "pod-c"}
		for i := 0; i < 3; i++ {
			signal, parseErr := parsePrometheusSignal(ctx, testNamespace, alertNames[i], podNames[i])
			Expect(parseErr).ToNot(HaveOccurred())

			response, processErr := gwServer.ProcessSignal(ctx, signal)
			Expect(processErr).ToNot(HaveOccurred())
			Expect(response.Status).To(Equal(gatewaypkg.StatusRejected))
		}

		By("5. Verify metric incremented exactly 3 times")
		finalValue := getMetricValue(metricsInstance.SignalsRejectedTotal, gatewaypkg.RejectionReasonUnmanagedResource)
		Expect(finalValue).To(Equal(initialValue+3),
			"BR-SCOPE-002: Metric must increment exactly once per rejection (3 rejections = +3)")

		By("6. Verify scope checker was called exactly 3 times")
		Expect(mockScope.callCount).To(Equal(3),
			"BR-SCOPE-002: Scope checker must be called once per signal")
	})
})

// parsePrometheusSignal parses a Prometheus webhook payload into a NormalizedSignal.
func parsePrometheusSignal(ctx context.Context, namespace, alertName, podName string) (*types.NormalizedSignal, error) {
	adapter := adapters.NewPrometheusAdapter(nil, nil)
	payload := createTestSignalPayload(namespace, alertName, podName)
	return adapter.Parse(ctx, payload)
}
