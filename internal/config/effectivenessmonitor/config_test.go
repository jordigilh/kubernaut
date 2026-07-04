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

package effectivenessmonitor_test

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	config "github.com/jordigilh/kubernaut/internal/config/effectivenessmonitor"
)

func TestEffectivenessMonitorConfig(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "EffectivenessMonitor Config Test Suite")
}

// Characterization tests written before the Wave E complexity remediation of
// Config.Validate (gocyclo finding), pinning current behavior since this
// function previously had zero test coverage.
var _ = Describe("Config.Validate", func() {
	It("accepts the default config unchanged", func() {
		cfg := config.DefaultConfig()
		Expect(cfg.Validate()).ToNot(HaveOccurred())
	})

	Describe("assessment window validation", func() {
		It("rejects a stabilizationWindow shorter than 30s", func() {
			cfg := config.DefaultConfig()
			cfg.Assessment.StabilizationWindow = 10 * time.Second
			err := cfg.Validate()
			Expect(err).To(MatchError(ContainSubstring("stabilizationWindow must be at least 30s")))
		})

		It("rejects a stabilizationWindow longer than 1h", func() {
			cfg := config.DefaultConfig()
			cfg.Assessment.StabilizationWindow = 2 * time.Hour
			err := cfg.Validate()
			Expect(err).To(MatchError(ContainSubstring("stabilizationWindow must not exceed 1h")))
		})

		It("rejects a validityWindow shorter than 30s", func() {
			cfg := config.DefaultConfig()
			cfg.Assessment.ValidityWindow = 10 * time.Second
			err := cfg.Validate()
			Expect(err).To(MatchError(ContainSubstring("validityWindow must be at least 30s")))
		})

		It("rejects a validityWindow longer than 24h", func() {
			cfg := config.DefaultConfig()
			cfg.Assessment.ValidityWindow = 25 * time.Hour
			err := cfg.Validate()
			Expect(err).To(MatchError(ContainSubstring("validityWindow must not exceed 24h")))
		})

		It("rejects a stabilizationWindow not shorter than the validityWindow", func() {
			cfg := config.DefaultConfig()
			cfg.Assessment.StabilizationWindow = 10 * time.Minute
			cfg.Assessment.ValidityWindow = 10 * time.Minute
			err := cfg.Validate()
			Expect(err).To(MatchError(ContainSubstring("must be shorter than validityWindow")))
		})
	})

	Describe("controller validation", func() {
		It("rejects an empty metricsAddr", func() {
			cfg := config.DefaultConfig()
			cfg.Controller.MetricsAddr = ""
			err := cfg.Validate()
			Expect(err).To(MatchError(ContainSubstring("controller.metricsAddr is required")))
		})

		It("rejects an empty healthProbeAddr", func() {
			cfg := config.DefaultConfig()
			cfg.Controller.HealthProbeAddr = ""
			err := cfg.Validate()
			Expect(err).To(MatchError(ContainSubstring("controller.healthProbeAddr is required")))
		})
	})

	Describe("external service validation", func() {
		It("rejects Prometheus enabled with an empty prometheusUrl", func() {
			cfg := config.DefaultConfig()
			cfg.External.PrometheusEnabled = true
			cfg.External.PrometheusURL = ""
			err := cfg.Validate()
			Expect(err).To(MatchError(ContainSubstring("external.prometheusUrl is required")))
		})

		It("rejects AlertManager enabled with an empty alertManagerUrl", func() {
			cfg := config.DefaultConfig()
			cfg.External.AlertManagerEnabled = true
			cfg.External.AlertManagerURL = ""
			err := cfg.Validate()
			Expect(err).To(MatchError(ContainSubstring("external.alertManagerUrl is required")))
		})

		It("rejects a non-positive connectionTimeout", func() {
			cfg := config.DefaultConfig()
			cfg.External.ConnectionTimeout = 0
			err := cfg.Validate()
			Expect(err).To(MatchError(ContainSubstring("external.connectionTimeout must be positive")))
		})

		It("rejects a prometheusLookback shorter than 1m", func() {
			cfg := config.DefaultConfig()
			cfg.External.PrometheusLookback = 30 * time.Second
			err := cfg.Validate()
			Expect(err).To(MatchError(ContainSubstring("external.prometheusLookback must be at least 1m")))
		})

		It("rejects a scrapeInterval shorter than 5s", func() {
			cfg := config.DefaultConfig()
			cfg.External.ScrapeInterval = 1 * time.Second
			err := cfg.Validate()
			Expect(err).To(MatchError(ContainSubstring("external.scrapeInterval must be at least 5s")))
		})
	})

	It("rejects a maxConcurrentReconciles below 1", func() {
		cfg := config.DefaultConfig()
		cfg.Assessment.MaxConcurrentReconciles = 0
		err := cfg.Validate()
		Expect(err).To(MatchError(ContainSubstring("assessment.maxConcurrentReconciles must be at least 1")))
	})

	It("propagates Fleet.Validate() failures", func() {
		cfg := config.DefaultConfig()
		cfg.Fleet.Enabled = true
		cfg.Fleet.Backend = "unsupported-backend"
		err := cfg.Validate()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unsupported backend"))
	})
})
