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

	// Helper: returns a valid base config with LLM fields satisfied.
	validBaseYAML := func(interactiveBlock string) []byte {
		return []byte(`
llm:
  endpoint: "http://localhost:11434/v1"
  model: "llama3"
` + interactiveBlock)
	}

	Describe("UT-KA-703-A01: Validate() rejects Interactive.Enabled=true with missing SessionTTL", func() {
		It("should return an error when session_ttl is zero", func() {
			yaml := validBaseYAML(`
interactive:
  enabled: true
  inactivity_timeout: 5m
  max_concurrent_sessions: 10
`)
			cfg, err := config.Load(yaml)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Interactive.Enabled).To(BeTrue())

			err = cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("session_ttl"))
		})
	})

	Describe("UT-KA-703-A02: Validate() rejects Interactive.Enabled=true with missing InactivityTimeout", func() {
		It("should return an error when inactivity_timeout is zero", func() {
			yaml := validBaseYAML(`
interactive:
  enabled: true
  session_ttl: 30m
  max_concurrent_sessions: 10
`)
			cfg, err := config.Load(yaml)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Interactive.Enabled).To(BeTrue())

			err = cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("inactivity_timeout"))
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
  session_ttl: 30m
  inactivity_timeout: 5m
  max_concurrent_sessions: 10
  max_analyzing_timeout: 45m
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
		It("should return an error when session_ttl exceeds maximum", func() {
			yaml := validBaseYAML(`
interactive:
  enabled: true
  session_ttl: 2h
  inactivity_timeout: 5m
  max_concurrent_sessions: 10
`)
			cfg, err := config.Load(yaml)
			Expect(err).NotTo(HaveOccurred())

			err = cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("session_ttl"))
			Expect(err.Error()).To(ContainSubstring("exceed"))
		})
	})

	Describe("UT-KA-703-A07: Validate() rejects InactivityTimeout exceeding 30m upper bound", func() {
		It("should return an error when inactivity_timeout exceeds maximum", func() {
			yaml := validBaseYAML(`
interactive:
  enabled: true
  session_ttl: 30m
  inactivity_timeout: 45m
  max_concurrent_sessions: 10
`)
			cfg, err := config.Load(yaml)
			Expect(err).NotTo(HaveOccurred())

			err = cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("inactivity_timeout"))
			Expect(err.Error()).To(ContainSubstring("exceed"))
		})
	})

	Describe("UT-KA-703-A08: Validate() rejects MaxConcurrentSessions exceeding 100", func() {
		It("should return an error when max_concurrent_sessions exceeds limit", func() {
			yaml := validBaseYAML(`
interactive:
  enabled: true
  session_ttl: 30m
  inactivity_timeout: 5m
  max_concurrent_sessions: 200
`)
			cfg, err := config.Load(yaml)
			Expect(err).NotTo(HaveOccurred())

			err = cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("max_concurrent_sessions"))
			Expect(err.Error()).To(ContainSubstring("exceed"))
		})
	})
})
