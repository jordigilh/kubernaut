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

package remediationorchestrator

import (
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	config "github.com/jordigilh/kubernaut/internal/config/remediationorchestrator"
)

// ========================================
// Issue #253: AsyncPropagation Config Tests
// ========================================
//
// Business Requirements:
// - BR-RO-103.4: Configurable propagation delays (gitOpsSyncDelay, operatorReconcileDelay)
//
// Design Document:
// - DD-EM-004 v2.0: RO configuration section

var _ = Describe("AsyncPropagation Config (#253, BR-RO-103.4)", Label("config", "async-propagation"), func() {

	// ========================================
	// UT-RO-253-001: Config defaults
	// ========================================
	Describe("UT-RO-253-001: Config defaults", Label("UT-RO-253-001"), func() {

		It("should provide sensible defaults for async propagation delays", func() {
			cfg := config.DefaultConfig()
			Expect(cfg.AsyncPropagation.GitOpsSyncDelay).To(Equal(3*time.Minute),
				"GitOpsSyncDelay default should be 3m per BR-RO-103.4")
			Expect(cfg.AsyncPropagation.OperatorReconcileDelay).To(Equal(1*time.Minute),
				"OperatorReconcileDelay default should be 1m per BR-RO-103.4")
		})

		It("should validate successfully with defaults", func() {
			cfg := config.DefaultConfig()
			Expect(cfg.Validate()).To(Succeed())
		})
	})

	// ========================================
	// UT-RO-253-002: Config rejects negative delays
	// ========================================
	Describe("UT-RO-253-002: Config rejects negative delays", Label("UT-RO-253-002"), func() {

		DescribeTable("negative duration rejection",
			func(field string, mutator func(*config.Config)) {
				cfg := config.DefaultConfig()
				mutator(cfg)
				err := cfg.Validate()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(field))
			},
			Entry("gitOpsSyncDelay = -1m",
				"gitOpsSyncDelay",
				func(c *config.Config) { c.AsyncPropagation.GitOpsSyncDelay = -1 * time.Minute },
			),
			Entry("operatorReconcileDelay = -30s",
				"operatorReconcileDelay",
				func(c *config.Config) { c.AsyncPropagation.OperatorReconcileDelay = -30 * time.Second },
			),
		)
	})

	// ========================================
	// UT-RO-253-003: Config accepts zero delay
	// ========================================
	Describe("UT-RO-253-003: Config accepts zero delay", Label("UT-RO-253-003"), func() {

		It("should accept gitOpsSyncDelay = 0", func() {
			cfg := config.DefaultConfig()
			cfg.AsyncPropagation.GitOpsSyncDelay = 0
			Expect(cfg.Validate()).To(Succeed(),
				"zero gitOpsSyncDelay disables that stage (instant sync environments)")
		})

		It("should accept operatorReconcileDelay = 0", func() {
			cfg := config.DefaultConfig()
			cfg.AsyncPropagation.OperatorReconcileDelay = 0
			Expect(cfg.Validate()).To(Succeed(),
				"zero operatorReconcileDelay disables that stage")
		})
	})

	// ========================================
	// UT-RO-253-008: Config loads explicit custom values
	// ========================================
	Describe("UT-RO-253-008: Config loads explicit custom values", Label("UT-RO-253-008"), func() {

		It("should parse custom asyncPropagation values from YAML", func() {
			tmpDir := GinkgoT().TempDir()
			yamlContent := `
controller:
  metricsAddr: ":9090"
  healthProbeAddr: ":8081"
timeouts:
  global: 1h
  processing: 5m
  analyzing: 10m
  executing: 30m
  awaitingApproval: 15m
effectivenessAssessment:
  stabilizationWindow: 5m
datastorage:
  url: "http://data-storage-service:8080"
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
asyncPropagation:
  gitOpsSyncDelay: 2m
  operatorReconcileDelay: 45s
`
			cfgPath := filepath.Join(tmpDir, "config.yaml")
			Expect(os.WriteFile(cfgPath, []byte(yamlContent), 0644)).To(Succeed())

			cfg, err := config.LoadFromFile(cfgPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.AsyncPropagation.GitOpsSyncDelay).To(Equal(2*time.Minute),
				"custom gitOpsSyncDelay should be 2m")
			Expect(cfg.AsyncPropagation.OperatorReconcileDelay).To(Equal(45*time.Second),
				"custom operatorReconcileDelay should be 45s")
		})
	})
})
