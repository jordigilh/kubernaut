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

var _ = Describe("Kubernaut Agent Configuration — #433", func() {

	Describe("UT-KA-433-001: Kubernaut Agent loads valid YAML configuration", func() {
		It("should parse all required fields from valid YAML", func() {
			yaml := []byte(`
llm:
  endpoint: "http://localhost:11434/v1"
  model: "llama3"
  api_key: "test-key"
server:
  address: "0.0.0.0"
  port: 8080
session:
  ttl: 30m
audit:
  enabled: true
  endpoint: "http://datastorage:8080"
investigator:
  max_turns: 15
`)
			cfg, err := config.Load(yaml)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg).NotTo(BeNil())
			Expect(cfg.LLM.Endpoint).To(Equal("http://localhost:11434/v1"))
			Expect(cfg.LLM.Model).To(Equal("llama3"))
			Expect(cfg.LLM.APIKey).To(Equal("test-key"))
			Expect(cfg.Server.Port).To(Equal(8080))
			Expect(cfg.Session.TTL).To(Equal(30 * time.Minute))
			Expect(cfg.Audit.Enabled).To(BeTrue())
			Expect(cfg.Investigator.MaxTurns).To(Equal(15))
		})
	})

	Describe("UT-KA-433-002: Kubernaut Agent applies correct defaults", func() {
		It("should fill defaults when optional fields are omitted", func() {
			yaml := []byte(`
llm:
  endpoint: "http://localhost:11434/v1"
  model: "llama3"
`)
			cfg, err := config.Load(yaml)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg).NotTo(BeNil())

			Expect(cfg.Server.Port).To(Equal(8080), "default port should be 8080")
			Expect(cfg.Session.TTL).To(Equal(30*time.Minute), "default TTL should be 30m")
			Expect(cfg.Investigator.MaxTurns).To(Equal(15), "default max turns should be 15")
			Expect(cfg.Anomaly.MaxToolCallsPerTool).To(Equal(5), "default per-tool limit should be 5")
			Expect(cfg.Anomaly.MaxTotalToolCalls).To(Equal(30), "default total tool calls should be 30")
			Expect(cfg.Anomaly.MaxRepeatedFailures).To(Equal(3), "default repeated failures should be 3")
		})
	})

	Describe("UT-KA-433-003: Kubernaut Agent rejects invalid config at startup", func() {
		It("should reject missing LLM endpoint", func() {
			yaml := []byte(`
llm:
  model: "llama3"
`)
			cfg, err := config.Load(yaml)
			if err == nil && cfg != nil {
				err = cfg.Validate()
			}
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("endpoint"))
		})

		It("should reject invalid max-turns (zero)", func() {
			yaml := []byte(`
llm:
  endpoint: "http://localhost:11434/v1"
  model: "llama3"
investigator:
  max_turns: 0
`)
			cfg, err := config.Load(yaml)
			if err == nil && cfg != nil {
				err = cfg.Validate()
			}
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("max_turns"))
		})

		It("should reject negative max-turns", func() {
			yaml := []byte(`
llm:
  endpoint: "http://localhost:11434/v1"
  model: "llama3"
investigator:
  max_turns: -5
`)
			cfg, err := config.Load(yaml)
			if err == nil && cfg != nil {
				err = cfg.Validate()
			}
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("max_turns"))
		})

		It("should reject missing LLM model", func() {
			yaml := []byte(`
llm:
  endpoint: "http://localhost:11434/v1"
`)
			cfg, err := config.Load(yaml)
			if err == nil && cfg != nil {
				err = cfg.Validate()
			}
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("llm.model"))
		})
	})
})
