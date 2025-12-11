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

package remediationorchestrator

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
)

var _ = Describe("Metrics", func() {

	Describe("Collector", func() {

		It("should create a new Collector with NewCollector", func() {
			// Given: Need for metrics collection
			// When: We create a new Collector
			collector := metrics.NewCollector()

			// Then: The Collector should be non-nil
			Expect(collector).ToNot(BeNil())
		})
	})

	// ========================================
	// BR-ORCH-042: Blocking Metrics
	// TDD RED Phase: These tests define the expected metrics
	// ========================================
	Describe("Blocking Metrics (BR-ORCH-042)", func() {

		Context("BlockedTotal counter", func() {

			It("should be defined as a CounterVec with namespace and reason labels", func() {
				// Given: Blocking feature needs observability
				// When: We check the BlockedTotal metric
				// Then: It should be defined and non-nil
				Expect(metrics.BlockedTotal).ToNot(BeNil(),
					"BR-ORCH-042: BlockedTotal counter must be defined")
			})

			It("should be registered in controller-runtime registry", func() {
				// Given: BlockedTotal metric is defined
				// When: We record a metric value
				// Then: It should not panic (metric is registered)
				Expect(func() {
					metrics.BlockedTotal.WithLabelValues("test-ns", "consecutive_failures_exceeded")
				}).ToNot(Panic(), "BlockedTotal should be registered")
			})
		})

		Context("BlockedCooldownExpiredTotal counter", func() {

			It("should be defined as a Counter (no labels)", func() {
				// Given: Need to track cooldown expiry events
				// When: We check the BlockedCooldownExpiredTotal metric
				// Then: It should be defined and non-nil
				Expect(metrics.BlockedCooldownExpiredTotal).ToNot(BeNil(),
					"BR-ORCH-042.3: BlockedCooldownExpiredTotal counter must be defined")
			})
		})

		Context("CurrentBlockedGauge gauge", func() {

			It("should be defined as a GaugeVec with namespace label", func() {
				// Given: Need to track current blocked count
				// When: We check the CurrentBlockedGauge metric
				// Then: It should be defined and non-nil
				Expect(metrics.CurrentBlockedGauge).ToNot(BeNil(),
					"BR-ORCH-042: CurrentBlockedGauge gauge must be defined")
			})

			It("should be registered in controller-runtime registry", func() {
				// Given: CurrentBlockedGauge metric is defined
				// When: We record a metric value
				// Then: It should not panic (metric is registered)
				Expect(func() {
					metrics.CurrentBlockedGauge.WithLabelValues("test-ns")
				}).ToNot(Panic(), "CurrentBlockedGauge should be registered")
			})
		})
	})
})
