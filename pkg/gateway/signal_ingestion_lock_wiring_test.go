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

package gateway_test

import (
	"context"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	gatewaypkg "github.com/jordigilh/kubernaut/pkg/gateway"
	"github.com/jordigilh/kubernaut/pkg/gateway/metrics"
	"github.com/jordigilh/kubernaut/pkg/gateway/processing"
	"github.com/prometheus/client_golang/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ========================================
// Wave 6 RED phase: BR-GATEWAY-190 production wiring characterization tests
// ========================================
//
// Discovery: server_lock_retry_test.go (LOCK-RETRY-U-001..008) validates the
// exponential-backoff retry ALGORITHM in isolation against a hand-rolled mock
// loop. Its own comment says so explicitly: "In the actual implementation,
// this loop will be inside ProcessSignal()". Branch-check of the merged
// unit+integration coverage profile confirmed ProcessSignal's real lock
// contention/retry/dedup-recheck block (signal_ingestion.go:544-611) had 0%
// coverage: no test exercised the real production dispatch path.
//
// These tests close that wiring gap by driving the REAL *gatewaypkg.Server
// and REAL *processing.DistributedLockManager (backed by a fake K8s client)
// through ProcessSignal, proving:
//   - BR-GATEWAY-190: lock contention correctly triggers the dedup-recheck
//     path and returns the existing RR's duplicate response without creating
//     a second RemediationRequest (multi-replica safety).
//   - ADR-052 Addendum 001: exhausting all retries against a persistently
//     held lock surfaces a bounded timeout error rather than blocking forever.
var _ = Describe("BR-GATEWAY-190: ProcessSignal distributed-lock wiring (Wave 6 RED)", func() {
	var (
		ctx             context.Context
		k8sClient       client.Client
		metricsInstance *metrics.Metrics
		testNamespace   string
	)

	BeforeEach(func() {
		ctx = context.Background()
		testNamespace = "kubernaut-system"

		Expect(os.Setenv("POD_NAME", "gateway-pod-under-test")).To(Succeed())
		Expect(os.Setenv("POD_NAMESPACE", testNamespace)).To(Succeed())
		DeferCleanup(func() {
			Expect(os.Unsetenv("POD_NAME")).To(Succeed())
			Expect(os.Unsetenv("POD_NAMESPACE")).To(Succeed())
		})

		k8sClient = newTestK8sClient(testNamespace)
		metricsInstance = metrics.NewMetricsWithRegistry(prometheus.NewRegistry())
	})

	It("[IT-GW-190-001] returns the existing RR as a duplicate when lock contention resolves via dedup-recheck, instead of creating a second CRD", func() {
		By("1. Create Gateway server with a real (fake-client-backed) distributed lock manager")
		gwServer, err := newTestGatewayServer(k8sClient, metricsInstance, &mockScopeChecker{managed: true})
		Expect(err).ToNot(HaveOccurred())

		By("2. Parse a signal and pre-acquire its lock as if another pod holds it")
		signal, err := parsePrometheusSignal(ctx, testNamespace, "LockWiringDuplicate", "payment-api-lockwiring")
		Expect(err).ToNot(HaveOccurred())

		competitorLock := processing.NewDistributedLockManager(k8sClient, testNamespace, "competing-gateway-pod")
		acquired, err := competitorLock.AcquireLock(ctx, signal.Fingerprint)
		Expect(err).ToNot(HaveOccurred())
		Expect(acquired).To(BeTrue(), "competing pod must hold the lock so our ProcessSignal call experiences real contention")

		By("3. Pre-create the RemediationRequest that the 'competing pod' would have created while holding the lock")
		existingRR := &remediationv1alpha1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "existing-rr-lockwiring",
				Namespace: testNamespace,
			},
			Spec: remediationv1alpha1.RemediationRequestSpec{
				SignalFingerprint: signal.Fingerprint,
				SignalName:        signal.SignalName,
				Severity:          "warning",
				SignalType:        "alert",
				ReceivedTime:      metav1.Now(),
			},
			Status: remediationv1alpha1.RemediationRequestStatus{
				OverallPhase: "Processing",
			},
		}
		Expect(k8sClient.Create(ctx, existingRR)).To(Succeed())

		By("4. ProcessSignal must retry through the real backoff loop, discover the RR via dedup-recheck, and return early")
		response, err := gwServer.ProcessSignal(ctx, signal)

		Expect(err).ToNot(HaveOccurred(),
			"BR-GATEWAY-190: lock contention resolved by dedup-recheck must not surface as an error")
		Expect(response.Status).To(Equal(gatewaypkg.StatusDeduplicated),
			"BR-GATEWAY-190: signal must be reported as a duplicate of the RR created by the competing pod")
		Expect(response.RemediationRequestName).To(Equal(existingRR.Name))

		By("5. Verify no second RemediationRequest was created (the core multi-replica safety guarantee)")
		rrList := &remediationv1alpha1.RemediationRequestList{}
		Expect(k8sClient.List(ctx, rrList, client.InNamespace(testNamespace))).To(Succeed())
		Expect(rrList.Items).To(HaveLen(1),
			"BR-GATEWAY-190: exactly one RemediationRequest must exist despite lock contention across two pods")
	})

	It("[IT-GW-190-003] acquires the lock on the first attempt, creates the CRD, and releases the lock afterward", func() {
		// BR-GATEWAY-190: This is the common-case path (no contention). Prior to
		// Wave 6 it was entirely untested via ProcessSignal because no existing
		// test set POD_NAME, so s.lockManager was always nil and this whole branch
		// (including the defer-release at signal_ingestion.go:607-613) never ran.
		By("1. Create Gateway server with a real distributed lock manager and no competing pod")
		gwServer, err := newTestGatewayServer(k8sClient, metricsInstance, &mockScopeChecker{managed: true})
		Expect(err).ToNot(HaveOccurred())

		signal, err := parsePrometheusSignal(ctx, testNamespace, "LockWiringHappyPath", "payment-api-lockhappy")
		Expect(err).ToNot(HaveOccurred())

		By("2. ProcessSignal must acquire the uncontended lock, create the CRD, and release the lock")
		response, err := gwServer.ProcessSignal(ctx, signal)
		Expect(err).ToNot(HaveOccurred())
		Expect(response.Status).To(Equal(gatewaypkg.StatusCreated))

		By("3. Verify the lock was released (a new acquirer must succeed immediately, not see contention)")
		otherPodLock := processing.NewDistributedLockManager(k8sClient, testNamespace, "another-pod-checking-release")
		acquired, err := otherPodLock.AcquireLock(ctx, signal.Fingerprint)
		Expect(err).ToNot(HaveOccurred())
		Expect(acquired).To(BeTrue(),
			"BR-GATEWAY-190: defer must release the lock after CRD creation so it doesn't block subsequent occurrences")
	})

	It("[IT-GW-190-002] returns a bounded timeout error instead of blocking forever when the lock is never released", func() {
		By("1. Create Gateway server with a real distributed lock manager")
		gwServer, err := newTestGatewayServer(k8sClient, metricsInstance, &mockScopeChecker{managed: true})
		Expect(err).ToNot(HaveOccurred())

		By("2. Parse a signal and hold its lock for the entire test (never released, no competing RR ever appears)")
		signal, err := parsePrometheusSignal(ctx, testNamespace, "LockWiringTimeout", "payment-api-locktimeout")
		Expect(err).ToNot(HaveOccurred())

		competitorLock := processing.NewDistributedLockManager(k8sClient, testNamespace, "permanently-holding-pod")
		acquired, err := competitorLock.AcquireLock(ctx, signal.Fingerprint)
		Expect(err).ToNot(HaveOccurred())
		Expect(acquired).To(BeTrue())

		By("3. ProcessSignal must exhaust its bounded retry budget and return a clear timeout error")
		_, err = gwServer.ProcessSignal(ctx, signal)

		Expect(err).To(HaveOccurred(),
			"ADR-052 Addendum 001: persistent lock contention with no resolution must eventually fail, not block forever")
		Expect(err.Error()).To(ContainSubstring("lock acquisition timeout"),
			"error must clearly identify bounded-retry exhaustion for operability")
		Expect(err.Error()).To(ContainSubstring(signal.Fingerprint),
			"error must include the fingerprint to allow correlating which signal was blocked")
	})
})
