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
	"fmt"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	gateway "github.com/jordigilh/kubernaut/pkg/gateway"
	"github.com/jordigilh/kubernaut/pkg/gateway/metrics"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
	"github.com/jordigilh/kubernaut/test/shared/helpers"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ========================================
// BR-SCOPE-002: Gateway Scope Filtering Integration Tests
// ========================================
//
// Test Plan Reference: docs/testing/BR-SCOPE-001/TEST_PLAN.md
// Test IDs: IT-GW-002-001 through IT-GW-002-010
//
// These integration tests validate the full Gateway processing flow
// with real scope.Manager backed by envtest K8s API:
// - IT-GW-002-001: No RR created for unmanaged signal
// - IT-GW-002-002: RR created for managed signal
// - IT-GW-002-003: Scope validation < 10ms latency
// - IT-GW-002-004: Namespace inheritance — Pod without label in managed NS
// - IT-GW-002-005: Resource opt-out overrides managed namespace
// - IT-GW-002-006: Resource opt-in overrides unmanaged namespace
// - IT-GW-002-007: Dynamic scope change — add label mid-test
// - IT-GW-002-008: Adapter-agnostic rejection (K8s Event signal)
// - IT-GW-002-009: Consecutive rejections — metric counter accuracy
// - IT-GW-002-010: Rejection response field verification
//
// Business Requirements:
// - BR-SCOPE-002: Gateway Signal Filtering
// - NFR-SCOPE-002: Scope validation latency
// ========================================

// getIntegrationMetricValue reads the current value of a Prometheus counter in integration tests.
func getIntegrationMetricValue(counter *prometheus.CounterVec, labels ...string) float64 {
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

var _ = Describe("BR-SCOPE-002: Gateway Scope Filtering (Integration)", Ordered, Label("scope", "integration"), func() {
	var (
		testLogger      logr.Logger
		gwServer        *gateway.Server
		scopeMgr        *scope.Manager
		metricsInstance *metrics.Metrics
		managedNS       string
		unmanagedNS     string
	)

	BeforeAll(func() {
		testLogger = logger.WithValues("test", "scope-filtering-integration")

		testLogger.Info("Setting up scope filtering integration tests")

		// Create managed namespace (with kubernaut.ai/managed=true)
		managedNS = helpers.CreateTestNamespace(ctx, k8sClient, "scope-managed-int")
		testLogger.Info("Created managed namespace", "namespace", managedNS)

		// Create unmanaged namespace (without kubernaut.ai/managed label)
		unmanagedNS = helpers.CreateTestNamespace(ctx, k8sClient, "scope-unmanaged-int",
			helpers.WithoutManagedLabel())
		testLogger.Info("Created unmanaged namespace", "namespace", unmanagedNS)

		// Create test Pods — some inherit from namespace, some have explicit labels
		testPods := []corev1.Pod{
			{
				// Pod in managed NS without own label (inherits from NS)
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod-managed",
					Namespace: managedNS,
				},
				Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "test", Image: "busybox:latest"}}},
			},
			{
				// Pod in managed NS with explicit opt-out label
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod-optout",
					Namespace: managedNS,
					Labels:    map[string]string{scope.ManagedLabelKey: scope.ManagedLabelValueFalse},
				},
				Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "test", Image: "busybox:latest"}}},
			},
			{
				// Pod in unmanaged NS without own label (inherits unmanaged from NS)
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod-unmanaged",
					Namespace: unmanagedNS,
				},
				Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "test", Image: "busybox:latest"}}},
			},
			{
				// Pod in unmanaged NS with explicit opt-in label
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod-optin",
					Namespace: unmanagedNS,
					Labels:    map[string]string{scope.ManagedLabelKey: scope.ManagedLabelValueTrue},
				},
				Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "test", Image: "busybox:latest"}}},
			},
		}
		for i := range testPods {
			Expect(k8sClient.Create(ctx, &testPods[i])).To(Succeed())
		}

		// Create real scope.Manager backed by envtest K8s client
		scopeMgr = scope.NewManager(k8sClient)

		// Create Gateway server with scope validation and exposed metrics
		gwConfig := createGatewayConfig(fmt.Sprintf("http://127.0.0.1:%d", gatewayDataStoragePort))
		testRegistry := prometheus.NewRegistry()
		metricsInstance = metrics.NewMetricsWithRegistry(testRegistry)
		var err error
		gwServer, err = gateway.NewServerForTesting(gwConfig, testLogger, metricsInstance, k8sClient, sharedAuditStore, scopeMgr)
		Expect(err).ToNot(HaveOccurred())
		testLogger.Info("Gateway server with scope validation initialized")
	})

	AfterAll(func() {
		testLogger.Info("Cleaning up scope filtering integration tests")
		helpers.DeleteTestNamespace(ctx, k8sClient, managedNS)
		helpers.DeleteTestNamespace(ctx, k8sClient, unmanagedNS)
	})

	// IT-GW-002-001: No RR created for unmanaged signal
	It("[IT-GW-002-001] should NOT create RR for signal from unmanaged namespace (BR-SCOPE-002)", func() {
		By("1. Send signal referencing pod in unmanaged namespace")
		signal := createNormalizedSignal(SignalBuilder{
			AlertName:    "HighCPU",
			Namespace:    unmanagedNS,
			ResourceKind: "Pod",
			ResourceName: "test-pod-unmanaged",
			Severity:     "warning",
		})

		response, err := gwServer.ProcessSignal(ctx, signal)

		By("2. Verify signal is rejected (not an error)")
		Expect(err).ToNot(HaveOccurred(),
			"IT-GW-002-001: Scope rejection is not an error")
		Expect(response).ToNot(BeNil())
		Expect(response.Status).To(Equal(gateway.StatusRejected),
			"IT-GW-002-001: Unmanaged signal must be rejected")

		By("3. Verify no RemediationRequest CRD was created")
		var rrList remediationv1alpha1.RemediationRequestList
		Expect(k8sClient.List(ctx, &rrList, client.InNamespace(unmanagedNS))).To(Succeed())
		Expect(rrList.Items).To(BeEmpty(),
			"IT-GW-002-001: No RR should exist in unmanaged namespace")
	})

	// IT-GW-002-002: RR created for managed signal
	It("[IT-GW-002-002] should create RR for signal from managed namespace (BR-SCOPE-002)", func() {
		By("1. Send signal referencing pod in managed namespace")
		signal := createNormalizedSignal(SignalBuilder{
			AlertName:    "HighMemory",
			Namespace:    managedNS,
			ResourceKind: "Pod",
			ResourceName: "test-pod-managed",
			Severity:     "critical",
		})

		response, err := gwServer.ProcessSignal(ctx, signal)

		By("2. Verify signal is accepted and RR created")
		Expect(err).ToNot(HaveOccurred())
		Expect(response).ToNot(BeNil())
		Expect(response.Status).To(Equal(gateway.StatusCreated),
			"IT-GW-002-002: Managed signal must result in CRD creation")
		Expect(response.RemediationRequestName).ToNot(BeEmpty())

		By("3. Verify RemediationRequest CRD exists in K8s")
		var rr remediationv1alpha1.RemediationRequest
		rrKey := client.ObjectKey{
			Name:      response.RemediationRequestName,
			Namespace: managedNS,
		}
		Expect(k8sClient.Get(ctx, rrKey, &rr)).To(Succeed(),
			"IT-GW-002-002: RR CRD must exist in managed namespace")
	})

	// IT-GW-002-003: Scope validation latency < 10ms
	It("[IT-GW-002-003] should validate scope within 10ms (NFR-SCOPE-002)", func() {
		By("1. Warm the cache with namespace metadata")
		// The previous tests already queried the namespaces, so cache is warm

		By("2. Time 100 IsManaged() calls")
		const iterations = 100
		start := time.Now()
		for i := 0; i < iterations; i++ {
			_, err := scopeMgr.IsManaged(ctx, managedNS, "Pod", "test-pod-managed")
			Expect(err).ToNot(HaveOccurred())
		}
		totalDuration := time.Since(start)
		avgLatency := totalDuration / time.Duration(iterations)

		By("3. Verify P95 latency < 10ms")
		GinkgoWriter.Printf("Scope validation: %d calls in %v (avg: %v)\n",
			iterations, totalDuration, avgLatency)
		// envtest is in-process, so latency should be well under 10ms
		Expect(avgLatency).To(BeNumerically("<", 10*time.Millisecond),
			"NFR-SCOPE-002: Average scope validation latency must be < 10ms")
	})

	// IT-GW-002-004: Namespace inheritance — Pod without label in managed namespace
	It("[IT-GW-002-004] should accept signal when pod inherits managed from namespace (BR-SCOPE-002)", func() {
		By("1. Send signal for pod WITHOUT own managed label in managed namespace")
		signal := createNormalizedSignal(SignalBuilder{
			AlertName:    "PodInheritManaged",
			Namespace:    managedNS,
			ResourceKind: "Pod",
			ResourceName: "test-pod-managed", // no kubernaut.ai/managed label — inherits from NS
			Severity:     "warning",
		})

		response, err := gwServer.ProcessSignal(ctx, signal)

		By("2. Verify signal is accepted via namespace inheritance")
		Expect(err).ToNot(HaveOccurred())
		Expect(response).ToNot(BeNil())
		Expect(response.Status).To(Equal(gateway.StatusCreated),
			"IT-GW-002-004: Pod without label must inherit managed from namespace")

		By("3. Verify RR CRD was created")
		Expect(response.RemediationRequestName).ToNot(BeEmpty())
		var rr remediationv1alpha1.RemediationRequest
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Name:      response.RemediationRequestName,
			Namespace: managedNS,
		}, &rr)).To(Succeed(),
			"IT-GW-002-004: RR must be created for inherited-managed pod")
	})

	// IT-GW-002-005: Resource opt-out overrides managed namespace
	It("[IT-GW-002-005] should reject signal when pod has opt-out label in managed NS (BR-SCOPE-002)", func() {
		By("1. Send signal for pod WITH managed=false in managed namespace")
		signal := createNormalizedSignal(SignalBuilder{
			AlertName:    "OptOutPodInManagedNS",
			Namespace:    managedNS,
			ResourceKind: "Pod",
			ResourceName: "test-pod-optout", // has kubernaut.ai/managed=false
			Severity:     "critical",
		})

		response, err := gwServer.ProcessSignal(ctx, signal)

		By("2. Verify signal is rejected (resource opt-out overrides NS)")
		Expect(err).ToNot(HaveOccurred())
		Expect(response).ToNot(BeNil())
		Expect(response.Status).To(Equal(gateway.StatusRejected),
			"IT-GW-002-005: Resource opt-out must override managed namespace")

		By("3. Verify no RR was created for this signal")
		var rrList remediationv1alpha1.RemediationRequestList
		Expect(k8sClient.List(ctx, &rrList,
			client.InNamespace(managedNS),
			client.MatchingFields{"spec.signalFingerprint": signal.Fingerprint},
		)).To(Succeed())
		Expect(rrList.Items).To(BeEmpty(),
			"IT-GW-002-005: No RR for opt-out resource in managed NS")
	})

	// IT-GW-002-006: Resource opt-in overrides unmanaged namespace
	It("[IT-GW-002-006] should accept signal when pod has opt-in label in unmanaged NS (BR-SCOPE-002)", func() {
		By("1. Send signal for pod WITH managed=true in unmanaged namespace")
		signal := createNormalizedSignal(SignalBuilder{
			AlertName:    "OptInPodInUnmanagedNS",
			Namespace:    unmanagedNS,
			ResourceKind: "Pod",
			ResourceName: "test-pod-optin", // has kubernaut.ai/managed=true
			Severity:     "warning",
		})

		response, err := gwServer.ProcessSignal(ctx, signal)

		By("2. Verify signal is accepted (resource opt-in overrides unmanaged NS)")
		Expect(err).ToNot(HaveOccurred())
		Expect(response).ToNot(BeNil())
		Expect(response.Status).To(Equal(gateway.StatusCreated),
			"IT-GW-002-006: Resource opt-in must override unmanaged namespace")

		By("3. Verify RR CRD was created in unmanaged namespace")
		Expect(response.RemediationRequestName).ToNot(BeEmpty())
		var rr remediationv1alpha1.RemediationRequest
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Name:      response.RemediationRequestName,
			Namespace: unmanagedNS,
		}, &rr)).To(Succeed(),
			"IT-GW-002-006: RR must be created for opt-in resource in unmanaged NS")
	})

	// IT-GW-002-007: Dynamic scope change — add label mid-test
	It("[IT-GW-002-007] should reflect dynamic namespace label change (BR-SCOPE-002)", func() {
		By("1. Create namespace without managed label")
		dynamicNS := helpers.CreateTestNamespace(ctx, k8sClient, "scope-dynamic-int",
			helpers.WithoutManagedLabel())
		defer helpers.DeleteTestNamespace(ctx, k8sClient, dynamicNS)

		// Create a pod in it
		dynamicPod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dynamic-test-pod",
				Namespace: dynamicNS,
			},
			Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "test", Image: "busybox:latest"}}},
		}
		Expect(k8sClient.Create(ctx, dynamicPod)).To(Succeed())

		By("2. Send signal — should be rejected (namespace not managed)")
		signal1 := createNormalizedSignal(SignalBuilder{
			AlertName:    "DynamicScopeBefore",
			Namespace:    dynamicNS,
			ResourceKind: "Pod",
			ResourceName: "dynamic-test-pod",
			Severity:     "warning",
		})

		response1, err := gwServer.ProcessSignal(ctx, signal1)
		Expect(err).ToNot(HaveOccurred())
		Expect(response1.Status).To(Equal(gateway.StatusRejected),
			"IT-GW-002-007: Signal must be rejected before label is added")

		By("3. Add kubernaut.ai/managed=true label to namespace")
		var ns corev1.Namespace
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: dynamicNS}, &ns)).To(Succeed())
		if ns.Labels == nil {
			ns.Labels = make(map[string]string)
		}
		ns.Labels[scope.ManagedLabelKey] = scope.ManagedLabelValueTrue
		Expect(k8sClient.Update(ctx, &ns)).To(Succeed())

		By("4. Send different signal — should be accepted (namespace now managed)")
		signal2 := createNormalizedSignal(SignalBuilder{
			AlertName:    "DynamicScopeAfter",
			Namespace:    dynamicNS,
			ResourceKind: "Pod",
			ResourceName: "dynamic-test-pod",
			Severity:     "warning",
		})

		response2, err := gwServer.ProcessSignal(ctx, signal2)
		Expect(err).ToNot(HaveOccurred())
		Expect(response2.Status).To(Equal(gateway.StatusCreated),
			"IT-GW-002-007: Signal must be accepted after managed label is added")
	})

	// IT-GW-002-008: Adapter-agnostic rejection (K8s Event signal)
	It("[IT-GW-002-008] should reject K8s Event-style signal from unmanaged NS (BR-SCOPE-002)", func() {
		By("1. Create signal mimicking K8s Event adapter output")
		signal := createNormalizedSignal(SignalBuilder{
			AlertName:    "FailedScheduling",
			Namespace:    unmanagedNS,
			ResourceKind: "Pod",
			ResourceName: "test-pod-unmanaged",
			Severity:     "warning",
			Source:       "kubernetes-events", // K8s Event adapter source
		})

		response, err := gwServer.ProcessSignal(ctx, signal)

		By("2. Verify rejection is adapter-agnostic")
		Expect(err).ToNot(HaveOccurred())
		Expect(response).ToNot(BeNil())
		Expect(response.Status).To(Equal(gateway.StatusRejected),
			"IT-GW-002-008: Scope filtering must be adapter-agnostic (K8s Event)")
	})

	// IT-GW-002-009: Consecutive rejections — metric counter accuracy
	It("[IT-GW-002-009] should increment rejection metric accurately for consecutive rejections (BR-SCOPE-002)", func() {
		By("1. Record initial metric value")
		initialValue := getIntegrationMetricValue(metricsInstance.SignalsRejectedTotal, gateway.RejectionReasonUnmanagedResource)

		By("2. Send 3 distinct signals to unmanaged namespace")
		alertNames := []string{"MetricTest1", "MetricTest2", "MetricTest3"}
		for _, alertName := range alertNames {
			signal := createNormalizedSignal(SignalBuilder{
				AlertName:    alertName,
				Namespace:    unmanagedNS,
				ResourceKind: "Pod",
				ResourceName: "test-pod-unmanaged",
				Severity:     "warning",
			})

			response, err := gwServer.ProcessSignal(ctx, signal)
			Expect(err).ToNot(HaveOccurred())
			Expect(response.Status).To(Equal(gateway.StatusRejected))
		}

		By("3. Verify metric incremented exactly 3 times")
		finalValue := getIntegrationMetricValue(metricsInstance.SignalsRejectedTotal, gateway.RejectionReasonUnmanagedResource)
		Expect(finalValue).To(Equal(initialValue+3),
			"IT-GW-002-009: Metric must increment by exactly 3 for 3 rejections")
	})

	// IT-GW-002-010: Rejection response field verification at integration level
	It("[IT-GW-002-010] should return correctly populated rejection response fields (BR-SCOPE-002)", func() {
		By("1. Send signal to unmanaged namespace")
		signal := createNormalizedSignal(SignalBuilder{
			AlertName:    "RejectionFieldVerify",
			Namespace:    unmanagedNS,
			ResourceKind: "Pod",
			ResourceName: "test-pod-unmanaged",
			Severity:     "warning",
		})

		response, err := gwServer.ProcessSignal(ctx, signal)

		By("2. Verify rejection response structure")
		Expect(err).ToNot(HaveOccurred())
		Expect(response).ToNot(BeNil())
		Expect(response.Status).To(Equal(gateway.StatusRejected))

		By("3. Verify rejection details with real namespace names")
		Expect(response.Rejection).ToNot(BeNil(),
			"IT-GW-002-010: Rejection struct must be populated")
		Expect(response.Rejection.Reason).To(Equal(gateway.RejectionReasonUnmanagedResource),
			"IT-GW-002-010: Reason must be 'unmanaged_resource'")
		Expect(response.Rejection.Resource).To(ContainSubstring(unmanagedNS),
			"IT-GW-002-010: Resource field must contain the real namespace name")
		Expect(response.Rejection.Resource).To(ContainSubstring("Pod"),
			"IT-GW-002-010: Resource field must contain the resource kind")
		Expect(response.Rejection.Action).To(ContainSubstring("kubectl label"),
			"IT-GW-002-010: Action must include kubectl command")
		Expect(response.Rejection.Action).To(ContainSubstring(scope.ManagedLabelKey),
			"IT-GW-002-010: Action must reference the managed label key")
		Expect(response.Rejection.Action).To(ContainSubstring(unmanagedNS),
			"IT-GW-002-010: Action must reference the real namespace for -n flag")
	})
})
