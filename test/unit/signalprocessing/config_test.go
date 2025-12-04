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

// Package signalprocessing contains unit tests for Signal Processing controller.
// BR-SP-051: K8s Context Enrichment - validates enrichment config
// BR-SP-070: Priority Assignment (Rego) - validates classifier config
// BR-SP-090: Categorization Audit Trail - validates audit config
package signalprocessing

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/signalprocessing/config"
)

func TestConfig(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "SignalProcessing Config Suite")
}

var _ = Describe("BR-SP-051/070/090: Config Validation", func() {
	// Helper to create a valid base config
	validConfig := func() *config.Config {
		return &config.Config{
			Enrichment: config.EnrichmentConfig{
				CacheTTL: 5 * time.Minute,
				Timeout:  2 * time.Second,
			},
			Classifier: config.ClassifierConfig{
				RegoConfigMapName: "signalprocessing-rego-policies",
				RegoConfigMapKey:  "policy.rego",
				HotReloadInterval: 30 * time.Second,
			},
			Audit: config.AuditConfig{
				DataStorageURL: "http://data-storage:8080",
				Timeout:        5 * time.Second,
				BufferSize:     1000,
				FlushInterval:  5 * time.Second,
			},
		}
	}

	// ==================================================
	// VALID CONFIGURATION TESTS
	// ==================================================
	Context("Valid Configuration", func() {
		DescribeTable("should accept valid configurations",
			func(modifier func(*config.Config)) {
				cfg := validConfig()
				modifier(cfg)
				err := cfg.Validate()
				Expect(err).NotTo(HaveOccurred())
			},
			Entry("default valid config", func(cfg *config.Config) {}),
			Entry("minimum valid enrichment timeout (1s)",
				func(cfg *config.Config) { cfg.Enrichment.Timeout = 1 * time.Second }),
			Entry("maximum valid buffer size (10000)",
				func(cfg *config.Config) { cfg.Audit.BufferSize = 10000 }),
			Entry("minimum valid hot reload interval (10s)",
				func(cfg *config.Config) { cfg.Classifier.HotReloadInterval = 10 * time.Second }),
			Entry("maximum valid flush interval (30s)",
				func(cfg *config.Config) { cfg.Audit.FlushInterval = 30 * time.Second }),
			Entry("zero cache TTL (no caching)",
				func(cfg *config.Config) { cfg.Enrichment.CacheTTL = 0 }),
		)
	})

	// ==================================================
	// ENRICHMENT CONFIG VALIDATION TESTS (BR-SP-051)
	// ==================================================
	Context("BR-SP-051: Enrichment Config Validation", func() {
		DescribeTable("should reject invalid enrichment configuration",
			func(modifier func(*config.Config), expectedError string) {
				cfg := validConfig()
				modifier(cfg)
				err := cfg.Validate()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(expectedError))
			},
			Entry("timeout less than 1s (500ms)",
				func(cfg *config.Config) { cfg.Enrichment.Timeout = 500 * time.Millisecond },
				"timeout must be at least 1s"),
			Entry("zero timeout",
				func(cfg *config.Config) { cfg.Enrichment.Timeout = 0 },
				"timeout must be at least 1s"),
			Entry("negative timeout",
				func(cfg *config.Config) { cfg.Enrichment.Timeout = -1 * time.Second },
				"timeout must be at least 1s"),
			Entry("negative cache TTL",
				func(cfg *config.Config) { cfg.Enrichment.CacheTTL = -1 * time.Minute },
				"cache_ttl cannot be negative"),
		)
	})

	// ==================================================
	// CLASSIFIER CONFIG VALIDATION TESTS (BR-SP-070)
	// ==================================================
	Context("BR-SP-070: Classifier Config Validation", func() {
		DescribeTable("should reject invalid classifier configuration",
			func(modifier func(*config.Config), expectedError string) {
				cfg := validConfig()
				modifier(cfg)
				err := cfg.Validate()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(expectedError))
			},
			Entry("missing RegoConfigMapName",
				func(cfg *config.Config) { cfg.Classifier.RegoConfigMapName = "" },
				"rego_configmap_name must be specified"),
			Entry("missing RegoConfigMapKey",
				func(cfg *config.Config) { cfg.Classifier.RegoConfigMapKey = "" },
				"rego_configmap_key must be specified"),
			Entry("hot reload interval less than 10s (5s)",
				func(cfg *config.Config) { cfg.Classifier.HotReloadInterval = 5 * time.Second },
				"hot_reload_interval must be at least 10s"),
			Entry("hot reload interval zero",
				func(cfg *config.Config) { cfg.Classifier.HotReloadInterval = 0 },
				"hot_reload_interval must be at least 10s"),
		)
	})

	// ==================================================
	// AUDIT CONFIG VALIDATION TESTS (BR-SP-090)
	// ==================================================
	Context("BR-SP-090: Audit Config Validation", func() {
		DescribeTable("should reject invalid audit configuration",
			func(modifier func(*config.Config), expectedError string) {
				cfg := validConfig()
				modifier(cfg)
				err := cfg.Validate()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(expectedError))
			},
			Entry("missing DataStorageURL",
				func(cfg *config.Config) { cfg.Audit.DataStorageURL = "" },
				"data_storage_url must be specified"),
			Entry("invalid DataStorageURL (not a URL)",
				func(cfg *config.Config) { cfg.Audit.DataStorageURL = "not-a-valid-url" },
				"data_storage_url is not a valid URL"),
			Entry("audit timeout less than 1s",
				func(cfg *config.Config) { cfg.Audit.Timeout = 500 * time.Millisecond },
				"timeout must be at least 1s"),
			Entry("audit timeout zero",
				func(cfg *config.Config) { cfg.Audit.Timeout = 0 },
				"timeout must be at least 1s"),
			Entry("buffer size less than 100 (50)",
				func(cfg *config.Config) { cfg.Audit.BufferSize = 50 },
				"buffer_size must be between 100 and 10000"),
			Entry("buffer size greater than 10000 (15000)",
				func(cfg *config.Config) { cfg.Audit.BufferSize = 15000 },
				"buffer_size must be between 100 and 10000"),
			Entry("flush interval less than 1s",
				func(cfg *config.Config) { cfg.Audit.FlushInterval = 500 * time.Millisecond },
				"flush_interval must be between 1s and 30s"),
			Entry("flush interval greater than 30s",
				func(cfg *config.Config) { cfg.Audit.FlushInterval = 60 * time.Second },
				"flush_interval must be between 1s and 30s"),
		)
	})

	// ==================================================
	// CONTROLLER CONFIG DEFAULTS TEST
	// ==================================================
	Context("Controller Config Defaults", func() {
		It("should return correct default controller config values", func() {
			defaults := config.DefaultControllerConfig()

			// Verify all expected default values
			Expect(defaults.MetricsAddr).To(Equal(":9090"))
			Expect(defaults.HealthProbeAddr).To(Equal(":8081"))
			Expect(defaults.LeaderElection).To(BeTrue())
			Expect(defaults.LeaderElectionID).To(Equal("signalprocessing.kubernaut.ai"))
		})
	})
})
