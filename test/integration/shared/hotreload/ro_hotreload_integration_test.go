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

package hotreload

import (
	"context"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	controller "github.com/jordigilh/kubernaut/internal/controller/remediationorchestrator"
	rometrics "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
	"github.com/jordigilh/kubernaut/pkg/shared/hotreload"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
)

func setupScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(s)
	_ = remediationv1.AddToScheme(s)
	return s
}

var _ = Describe("RO Config Hot-Reload via FileWatcher (IT-RO-835)", func() {
	var (
		reconciler *controller.Reconciler
		tmpDir     string
		configPath string
	)

	BeforeEach(func() {
		scheme := setupScheme()
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		reconciler = controller.NewReconciler(
			fakeClient,
			fakeClient,
			scheme,
			nil,
			nil,
			rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
			controller.TimeoutConfig{
				Global:           1 * time.Hour,
				Processing:       5 * time.Minute,
				Analyzing:        10 * time.Minute,
				Executing:        30 * time.Minute,
				AwaitingApproval: 15 * time.Minute,
				Verifying:        30 * time.Minute,
			},
			nil, // routing engine (not needed for config reload)
			nil, // eaCreator
		)
		reconciler.SetRetentionPeriod(24 * time.Hour)
		reconciler.SetDryRun(false, 1*time.Hour)

		var err error
		tmpDir, err = os.MkdirTemp("", "ro-hotreload-*")
		Expect(err).NotTo(HaveOccurred())

		configPath = filepath.Join(tmpDir, "config.yaml")
	})

	AfterEach(func() {
		os.RemoveAll(tmpDir)
	})

	It("IT-RO-835-01: file modification triggers config update via FileWatcher", func() {
		initialConfig := `
controller:
  metricsAddr: ":9090"
  healthProbeAddr: ":8081"
timeouts:
  global: 1h
  processing: 5m
  analyzing: 10m
  executing: 30m
  awaitingApproval: 15m
  verifying: 30m
effectivenessAssessment:
  stabilizationWindow: 5m
asyncPropagation:
  gitOpsSyncDelay: 3m
  operatorReconcileDelay: 1m
  proactiveAlertDelay: 5m
retention:
  period: 24h
dryRun: false
routing:
  consecutiveFailureThreshold: 3
  consecutiveFailureCooldown: 1h
  recentlyRemediatedCooldown: 5m
  exponentialBackoffBase: 1m
  exponentialBackoffMax: 10m
  exponentialBackoffMaxExponent: 4
  scopeBackoffBase: 5s
  scopeBackoffMax: 5m
  ineffectiveChainThreshold: 3
  recurrenceCountThreshold: 5
  ineffectiveTimeWindow: 4h
datastorage:
  url: "http://localhost:8080"
  timeout: 30s
  buffer:
    bufferSize: 1000
    batchSize: 100
    flushInterval: 5s
    maxRetries: 3
`
		err := os.WriteFile(configPath, []byte(initialConfig), 0644)
		Expect(err).NotTo(HaveOccurred())

		callback := controller.NewReloadCallback(reconciler, ctrl.Log.WithName("test"))
		watcher, err := hotreload.NewFileWatcher(configPath, callback, ctrl.Log.WithName("test"))
		Expect(err).NotTo(HaveOccurred())

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		err = watcher.Start(ctx)
		Expect(err).NotTo(HaveOccurred())
		defer watcher.Stop()

		Expect(reconciler.IsDryRunExported()).To(BeFalse())
		Expect(reconciler.GetRetentionPeriodExported()).To(Equal(24 * time.Hour))

		updatedConfig := `
controller:
  metricsAddr: ":9090"
  healthProbeAddr: ":8081"
timeouts:
  global: 1h
  processing: 5m
  analyzing: 10m
  executing: 30m
  awaitingApproval: 15m
  verifying: 30m
effectivenessAssessment:
  stabilizationWindow: 5m
asyncPropagation:
  gitOpsSyncDelay: 7m
  operatorReconcileDelay: 3m
  proactiveAlertDelay: 10m
retention:
  period: 72h
dryRun: true
dryRunHoldPeriod: 4h
routing:
  consecutiveFailureThreshold: 3
  consecutiveFailureCooldown: 1h
  recentlyRemediatedCooldown: 5m
  exponentialBackoffBase: 1m
  exponentialBackoffMax: 10m
  exponentialBackoffMaxExponent: 4
  scopeBackoffBase: 5s
  scopeBackoffMax: 5m
  ineffectiveChainThreshold: 3
  recurrenceCountThreshold: 5
  ineffectiveTimeWindow: 4h
datastorage:
  url: "http://localhost:8080"
  timeout: 30s
  buffer:
    bufferSize: 1000
    batchSize: 100
    flushInterval: 5s
    maxRetries: 3
`
		err = os.WriteFile(configPath, []byte(updatedConfig), 0644)
		Expect(err).NotTo(HaveOccurred())

		Eventually(func() bool {
			return reconciler.IsDryRunExported()
		}, 3*time.Second, 100*time.Millisecond).Should(BeTrue(),
			"dryRun should be enabled after config file modification")

		Eventually(func() time.Duration {
			return reconciler.GetRetentionPeriodExported()
		}, 3*time.Second, 100*time.Millisecond).Should(Equal(72 * time.Hour),
			"retentionPeriod should update to 72h")

		Eventually(func() time.Duration {
			return reconciler.GetDryRunHoldPeriodExported()
		}, 3*time.Second, 100*time.Millisecond).Should(Equal(4 * time.Hour),
			"dryRunHoldPeriod should update to 4h")

		asyncCfg := reconciler.GetAsyncPropagationExported()
		Expect(asyncCfg.GitOpsSyncDelay).To(Equal(7 * time.Minute))
		Expect(asyncCfg.OperatorReconcileDelay).To(Equal(3 * time.Minute))
		Expect(asyncCfg.ProactiveAlertDelay).To(Equal(10 * time.Minute))
	})

	It("IT-RO-835-02: invalid config file modification is rejected (graceful degradation)", func() {
		validConfig := `
controller:
  metricsAddr: ":9090"
  healthProbeAddr: ":8081"
timeouts:
  global: 1h
  processing: 5m
  analyzing: 10m
  executing: 30m
  awaitingApproval: 15m
  verifying: 30m
effectivenessAssessment:
  stabilizationWindow: 5m
asyncPropagation:
  gitOpsSyncDelay: 3m
  operatorReconcileDelay: 1m
  proactiveAlertDelay: 5m
retention:
  period: 24h
routing:
  consecutiveFailureThreshold: 3
  consecutiveFailureCooldown: 1h
  recentlyRemediatedCooldown: 5m
  exponentialBackoffBase: 1m
  exponentialBackoffMax: 10m
  exponentialBackoffMaxExponent: 4
  scopeBackoffBase: 5s
  scopeBackoffMax: 5m
  ineffectiveChainThreshold: 3
  recurrenceCountThreshold: 5
  ineffectiveTimeWindow: 4h
datastorage:
  url: "http://localhost:8080"
  timeout: 30s
  buffer:
    bufferSize: 1000
    batchSize: 100
    flushInterval: 5s
    maxRetries: 3
`
		err := os.WriteFile(configPath, []byte(validConfig), 0644)
		Expect(err).NotTo(HaveOccurred())

		callback := controller.NewReloadCallback(reconciler, ctrl.Log.WithName("test"))
		watcher, err := hotreload.NewFileWatcher(configPath, callback, ctrl.Log.WithName("test"))
		Expect(err).NotTo(HaveOccurred())

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		err = watcher.Start(ctx)
		Expect(err).NotTo(HaveOccurred())
		defer watcher.Stop()

		Expect(reconciler.GetRetentionPeriodExported()).To(Equal(24 * time.Hour))

		invalidConfig := `
controller:
  metricsAddr: ":9090"
  healthProbeAddr: ":8081"
timeouts:
  global: 0s
  processing: 5m
  analyzing: 10m
  executing: 30m
  awaitingApproval: 15m
  verifying: 30m
retention:
  period: 1h
`
		err = os.WriteFile(configPath, []byte(invalidConfig), 0644)
		Expect(err).NotTo(HaveOccurred())

		// ✅ APPROVED EXCEPTION: intentional wait for fsnotify debounce (200ms) + processing
		time.Sleep(500 * time.Millisecond)

		Expect(reconciler.GetRetentionPeriodExported()).To(Equal(24 * time.Hour),
			"retention should remain unchanged after invalid config rejected")

		Expect(watcher.GetErrorCount()).To(BeNumerically(">=", int64(1)),
			"watcher should record at least one error for rejected config")
	})
})
