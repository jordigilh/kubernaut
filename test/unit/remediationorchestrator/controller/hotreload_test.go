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

package controller

import (
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	roconfig "github.com/jordigilh/kubernaut/internal/config/remediationorchestrator"
	controller "github.com/jordigilh/kubernaut/internal/controller/remediationorchestrator"
	rometrics "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
)

var _ = Describe("RO Config Hot-Reload (#835, DD-INFRA-001)", func() {

	var reconciler *controller.Reconciler

	BeforeEach(func() {
		scheme := setupScheme()
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		reconciler = controller.NewReconciler(
			fakeClient,
			fakeClient,
			scheme,
			nil, // audit store
			nil, // recorder
			rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
			controller.TimeoutConfig{
				Global:           1 * time.Hour,
				Processing:       5 * time.Minute,
				Analyzing:        10 * time.Minute,
				Executing:        30 * time.Minute,
				AwaitingApproval: 15 * time.Minute,
				Verifying:        30 * time.Minute,
			},
			&MockRoutingEngine{},
			nil, // eaCreator
		)
	})

	Context("Thread-safety (UT-RO-835-TS)", func() {
		// BR-ORCH-835: Concurrent reads and writes to hot-reloadable config fields
		// must not race. Validates sync.RWMutex protection per DD-INFRA-001.

		It("UT-RO-835-TS01: concurrent SetDryRun and isDryRun do not race", func() {
			reconciler.SetDryRun(false, 1*time.Hour)

			var wg sync.WaitGroup
			const goroutines = 50

			for i := 0; i < goroutines; i++ {
				wg.Add(2)
				go func() {
					defer wg.Done()
					reconciler.SetDryRun(true, 2*time.Hour)
				}()
				go func() {
					defer wg.Done()
					_ = reconciler.IsDryRunExported()
				}()
			}

			wg.Wait()
		})

		It("UT-RO-835-TS02: concurrent SetRetentionPeriod and getRetentionPeriod do not race", func() {
			reconciler.SetRetentionPeriod(24 * time.Hour)

			var wg sync.WaitGroup
			const goroutines = 50

			for i := 0; i < goroutines; i++ {
				wg.Add(2)
				go func() {
					defer wg.Done()
					reconciler.SetRetentionPeriod(48 * time.Hour)
				}()
				go func() {
					defer wg.Done()
					_ = reconciler.GetRetentionPeriodExported()
				}()
			}

			wg.Wait()
		})

		It("UT-RO-835-TS03: concurrent SetAsyncPropagation and getAsyncPropagation do not race", func() {
			reconciler.SetAsyncPropagation(roconfig.AsyncPropagationConfig{
				GitOpsSyncDelay: 3 * time.Minute,
			})

			var wg sync.WaitGroup
			const goroutines = 50

			for i := 0; i < goroutines; i++ {
				wg.Add(2)
				go func() {
					defer wg.Done()
					reconciler.SetAsyncPropagation(roconfig.AsyncPropagationConfig{
						GitOpsSyncDelay:        5 * time.Minute,
						OperatorReconcileDelay: 2 * time.Minute,
					})
				}()
				go func() {
					defer wg.Done()
					_ = reconciler.GetAsyncPropagationExported()
				}()
			}

			wg.Wait()
		})
	})

	Context("ReloadCallback validation (UT-RO-835-CB)", func() {
		// BR-ORCH-835: ReloadCallback must reject invalid config (graceful degradation)
		// and apply valid config atomically.

		It("UT-RO-835-CB01: rejects invalid YAML and preserves previous config", func() {
			reconciler.SetDryRun(true, 2*time.Hour)
			reconciler.SetRetentionPeriod(48 * time.Hour)
			reconciler.SetAsyncPropagation(roconfig.AsyncPropagationConfig{
				GitOpsSyncDelay: 5 * time.Minute,
			})

			callback := controller.NewReloadCallback(reconciler, ctrl.Log.WithName("test"))

			err := callback("this is not valid YAML: [[[")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("parse"))

			Expect(reconciler.IsDryRunExported()).To(BeTrue())
			Expect(reconciler.GetRetentionPeriodExported()).To(Equal(48 * time.Hour))
			Expect(reconciler.GetAsyncPropagationExported().GitOpsSyncDelay).To(Equal(5 * time.Minute))
		})

		It("UT-RO-835-CB02: rejects config that fails Validate() and preserves previous", func() {
			reconciler.SetRetentionPeriod(24 * time.Hour)

			callback := controller.NewReloadCallback(reconciler, ctrl.Log.WithName("test"))

			invalidYAML := `
timeouts:
  global: 0s
  processing: 5m
  analyzing: 10m
  executing: 30m
  awaitingApproval: 15m
  verifying: 30m
`
			err := callback(invalidYAML)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid configuration"))

			Expect(reconciler.GetRetentionPeriodExported()).To(Equal(24 * time.Hour))
		})

		It("UT-RO-835-CB03: applies valid config and updates all hot-reloadable fields", func() {
			reconciler.SetDryRun(false, 1*time.Hour)
			reconciler.SetRetentionPeriod(24 * time.Hour)
			reconciler.SetAsyncPropagation(roconfig.AsyncPropagationConfig{
				GitOpsSyncDelay:        3 * time.Minute,
				OperatorReconcileDelay: 1 * time.Minute,
				ProactiveAlertDelay:    5 * time.Minute,
			})

			callback := controller.NewReloadCallback(reconciler, ctrl.Log.WithName("test"))

			validYAML := `
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
			err := callback(validYAML)
			Expect(err).NotTo(HaveOccurred())

			Expect(reconciler.IsDryRunExported()).To(BeTrue())
			Expect(reconciler.GetDryRunHoldPeriodExported()).To(Equal(4 * time.Hour))
			Expect(reconciler.GetRetentionPeriodExported()).To(Equal(72 * time.Hour))
			asyncCfg := reconciler.GetAsyncPropagationExported()
			Expect(asyncCfg.GitOpsSyncDelay).To(Equal(7 * time.Minute))
			Expect(asyncCfg.OperatorReconcileDelay).To(Equal(3 * time.Minute))
			Expect(asyncCfg.ProactiveAlertDelay).To(Equal(10 * time.Minute))
		})
	})
})
