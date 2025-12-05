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
// Unit tests validate implementation correctness, not business value delivery.
// See docs/development/business-requirements/TESTING_GUIDELINES.md
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

// Unit Test: Config.Validate implementation correctness
var _ = Describe("Config.Validate", func() {
	// Test 1: Function behavior for valid input
	It("should return nil for valid configuration with all required fields", func() {
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
})
