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
// BR-SP-001: K8s Context Enrichment - validates enrichment config
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

var _ = Describe("Config Validation", func() {
	// Test 1: Valid config should pass validation
	// BR-SP-001, BR-SP-070, BR-SP-090
	It("should validate a complete valid configuration", func() {
		cfg := &config.Config{
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
		err := cfg.Validate()
		Expect(err).NotTo(HaveOccurred())
	})

	// Test 2: Enrichment timeout < 1s should fail
	// BR-SP-001: K8s enrichment must complete within timeout
	It("should reject enrichment timeout less than 1s", func() {
		cfg := &config.Config{
			Enrichment: config.EnrichmentConfig{
				CacheTTL: 5 * time.Minute,
				Timeout:  500 * time.Millisecond, // Invalid: < 1s
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
		err := cfg.Validate()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("timeout"))
	})

	// Test 3: Missing RegoConfigMapName should fail
	// BR-SP-070: Rego policies are required for classification
	It("should reject missing RegoConfigMapName", func() {
		cfg := &config.Config{
			Enrichment: config.EnrichmentConfig{
				CacheTTL: 5 * time.Minute,
				Timeout:  2 * time.Second,
			},
			Classifier: config.ClassifierConfig{
				RegoConfigMapName: "", // Missing required field
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
		err := cfg.Validate()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("rego_configmap_name"))
	})

	// Test 4: HotReloadInterval < 10s should fail
	// BR-SP-072: Rego hot-reload interval minimum 10s
	It("should reject HotReloadInterval less than 10s", func() {
		cfg := &config.Config{
			Enrichment: config.EnrichmentConfig{
				CacheTTL: 5 * time.Minute,
				Timeout:  2 * time.Second,
			},
			Classifier: config.ClassifierConfig{
				RegoConfigMapName: "signalprocessing-rego-policies",
				RegoConfigMapKey:  "policy.rego",
				HotReloadInterval: 5 * time.Second, // Invalid: < 10s
			},
			Audit: config.AuditConfig{
				DataStorageURL: "http://data-storage:8080",
				Timeout:        5 * time.Second,
				BufferSize:     1000,
				FlushInterval:  5 * time.Second,
			},
		}
		err := cfg.Validate()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("hot_reload_interval"))
	})

	// Test 5: BufferSize < 100 should fail
	// BR-SP-090: Audit buffer size minimum
	It("should reject BufferSize less than 100", func() {
		cfg := &config.Config{
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
				BufferSize:     50, // Invalid: < 100
				FlushInterval:  5 * time.Second,
			},
		}
		err := cfg.Validate()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("buffer_size"))
	})

	// Test 6: FlushInterval < 1s should fail
	It("should reject FlushInterval less than 1s", func() {
		cfg := &config.Config{
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
				FlushInterval:  500 * time.Millisecond, // Invalid: < 1s
			},
		}
		err := cfg.Validate()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("flush_interval"))
	})

	// Test 7: DefaultControllerConfig should return correct defaults
	It("should return correct default controller config", func() {
		defaults := config.DefaultControllerConfig()
		Expect(defaults.MetricsAddr).To(Equal(":9090"))
		Expect(defaults.HealthProbeAddr).To(Equal(":8081"))
		Expect(defaults.LeaderElection).To(BeTrue())
		Expect(defaults.LeaderElectionID).To(Equal("signalprocessing.kubernaut.ai"))
	})
})
