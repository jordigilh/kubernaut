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

package config_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/config"
)

var _ = Describe("InteractiveConfig Validation — #703", func() {

	// Helper: returns a valid base config with required fields satisfied.
	validBaseYAML := func(interactiveBlock string) []byte {
		return []byte(`
runtime:
  server:
    rateLimit:
      requestsPerSecond: 5
      burst: 10
ai:
  llm:
    provider: "openai"
  investigation:
    maxTurns: 40
` + interactiveBlock)
	}

	Describe("UT-KA-703-A01: Validate() rejects Interactive.Enabled=true with missing SessionTTL", func() {
		It("should return an error when sessionTTL is zero", func() {
			yaml := validBaseYAML(`
interactive:
  enabled: true
  inactivityTimeout: 5m
  maxConcurrentSessions: 10
`)
			cfg, err := config.Load(yaml)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Interactive.Enabled).To(BeTrue())

			err = cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("sessionTTL"))
		})
	})

	Describe("UT-KA-703-A02: Validate() rejects Interactive.Enabled=true with missing InactivityTimeout", func() {
		It("should return an error when inactivityTimeout is zero", func() {
			yaml := validBaseYAML(`
interactive:
  enabled: true
  sessionTTL: 30m
  maxConcurrentSessions: 10
`)
			cfg, err := config.Load(yaml)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Interactive.Enabled).To(BeTrue())

			err = cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("inactivityTimeout"))
		})
	})

	Describe("UT-KA-703-A03: Validate() accepts Interactive.Enabled=false with zero Interactive fields", func() {
		It("should not error when interactive is disabled regardless of other field values", func() {
			yaml := validBaseYAML(`
interactive:
  enabled: false
`)
			cfg, err := config.Load(yaml)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Interactive.Enabled).To(BeFalse())

			err = cfg.Validate()
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("UT-KA-703-A04: Validate() accepts complete Interactive config", func() {
		It("should pass validation with all required fields set", func() {
			yaml := validBaseYAML(`
interactive:
  enabled: true
  sessionTTL: 30m
  inactivityTimeout: 5m
  maxConcurrentSessions: 10
  maxAnalyzingTimeout: 45m
`)
			cfg, err := config.Load(yaml)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Interactive.Enabled).To(BeTrue())
			Expect(cfg.Interactive.SessionTTL).To(Equal(30 * time.Minute))
			Expect(cfg.Interactive.InactivityTimeout).To(Equal(5 * time.Minute))
			Expect(cfg.Interactive.MaxConcurrentSessions).To(Equal(10))
			Expect(cfg.Interactive.MaxAnalyzingTimeout).To(Equal(45 * time.Minute))

			err = cfg.Validate()
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("UT-KA-703-A05: DefaultConfig() returns Interactive.Enabled=false", func() {
		It("should have interactive mode disabled by default", func() {
			cfg := config.DefaultConfig()
			Expect(cfg.Interactive.Enabled).To(BeFalse())
			Expect(cfg.Interactive.SessionTTL).To(BeZero())
		})
	})

	Describe("UT-KA-703-A06: Validate() rejects SessionTTL exceeding 1h upper bound", func() {
		It("should return an error when sessionTTL exceeds maximum", func() {
			yaml := validBaseYAML(`
interactive:
  enabled: true
  sessionTTL: 2h
  inactivityTimeout: 5m
  maxConcurrentSessions: 10
`)
			cfg, err := config.Load(yaml)
			Expect(err).NotTo(HaveOccurred())

			err = cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("sessionTTL"))
			Expect(err.Error()).To(ContainSubstring("exceed"))
		})
	})

	Describe("UT-KA-703-A07: Validate() rejects InactivityTimeout exceeding 30m upper bound", func() {
		It("should return an error when inactivityTimeout exceeds maximum", func() {
			yaml := validBaseYAML(`
interactive:
  enabled: true
  sessionTTL: 30m
  inactivityTimeout: 45m
  maxConcurrentSessions: 10
`)
			cfg, err := config.Load(yaml)
			Expect(err).NotTo(HaveOccurred())

			err = cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("inactivityTimeout"))
			Expect(err.Error()).To(ContainSubstring("exceed"))
		})
	})

	Describe("UT-KA-703-A08: Validate() rejects MaxConcurrentSessions exceeding 100", func() {
		It("should return an error when maxConcurrentSessions exceeds limit", func() {
			yaml := validBaseYAML(`
interactive:
  enabled: true
  sessionTTL: 30m
  inactivityTimeout: 5m
  maxConcurrentSessions: 200
`)
			cfg, err := config.Load(yaml)
			Expect(err).NotTo(HaveOccurred())

			err = cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("maxConcurrentSessions"))
			Expect(err.Error()).To(ContainSubstring("exceed"))
		})
	})
})
