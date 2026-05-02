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

	kacfg "github.com/jordigilh/kubernaut/internal/kubernautagent/config"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Kubernaut Agent config validation — BR-AI-952 / GAP-T2", func() {

	Describe("IT-CFG-001: DefaultConfig validates", func() {
		It("returns no error from Validate()", func() {
			Expect(kacfg.DefaultConfig().Validate()).To(Succeed())
		})
	})

	Describe("IT-CFG-002: port=0 fails validation", func() {
		It("rejects runtime.server.port below range", func() {
			cfg := kacfg.DefaultConfig()
			cfg.Runtime.Server.Port = 0
			Expect(cfg.Validate()).To(HaveOccurred())
		})
	})

	Describe("IT-CFG-003: port=70000 fails validation", func() {
		It("rejects runtime.server.port above 65535", func() {
			cfg := kacfg.DefaultConfig()
			cfg.Runtime.Server.Port = 70000
			Expect(cfg.Validate()).To(HaveOccurred())
		})
	})

	Describe("IT-CFG-004: negative session TTL fails", func() {
		It("rejects non-positive runtime.session.ttl", func() {
			cfg := kacfg.DefaultConfig()
			cfg.Runtime.Session.TTL = -1 * time.Minute
			Expect(cfg.Validate()).To(HaveOccurred())
		})
	})

	Describe("IT-CFG-005: audit enabled with bufferSize=0 fails", func() {
		It("requires positive buffer when audit enabled", func() {
			cfg := kacfg.DefaultConfig()
			cfg.Runtime.Audit.Enabled = true
			cfg.Runtime.Audit.BufferSize = 0
			Expect(cfg.Validate()).To(HaveOccurred())
		})
	})

	Describe("IT-CFG-006: alignmentCheck enabled with timeout=0 fails", func() {
		It("requires positive timeout when alignment enabled", func() {
			cfg := kacfg.DefaultConfig()
			cfg.AI.AlignmentCheck.Enabled = true
			cfg.AI.AlignmentCheck.Timeout = 0
			Expect(cfg.Validate()).To(HaveOccurred())
		})
	})

	Describe("IT-CFG-007: invalid logging level fails", func() {
		It("rejects unknown runtime.logging.level", func() {
			cfg := kacfg.DefaultConfig()
			cfg.Runtime.Logging.Level = "TRACE"
			Expect(cfg.Validate()).To(HaveOccurred())
		})
	})

	Describe("IT-CFG-008: invalid audit verbosity fails", func() {
		It("rejects verbosity outside full|standard|minimal", func() {
			cfg := kacfg.DefaultConfig()
			cfg.Runtime.Audit.Verbosity = "verbose-plus"
			Expect(cfg.Validate()).To(HaveOccurred())
		})
	})

	Describe("IT-CFG-009: Load + LoadLLMRuntime YAML round-trip", func() {
		It("parses representative YAML into valid structs", func() {
			mainYAML := `
runtime:
  server:
    port: 9443
  session:
    ttl: 45m
ai:
  llm:
    provider: openai
    circuitBreaker:
      enabled: true
      maxRequests: 2
      interval: 5s
      timeout: 10s
      failureThreshold: 3
      failureRatio: 0.4
`
			cfg, err := kacfg.Load([]byte(mainYAML))
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Validate()).To(Succeed())
			Expect(cfg.Runtime.Server.Port).To(Equal(9443))
			Expect(cfg.AI.LLM.Provider).To(Equal("openai"))

			llmYAML := `
model: gpt-test
endpoint: https://llm.example/v1
apiKey: secret
temperature: 0.2
maxRetries: 2
timeoutSeconds: 60
`
			rt, err := kacfg.LoadLLMRuntime([]byte(llmYAML))
			Expect(err).NotTo(HaveOccurred())
			Expect(rt.Validate("openai")).To(Succeed())
			Expect(rt.Model).To(Equal("gpt-test"))
			Expect(rt.Endpoint).To(Equal("https://llm.example/v1"))
		})
	})

	Describe("IT-CFG-010: LLMRuntimeConfig missing model fails", func() {
		It("Validate requires model", func() {
			rt := kacfg.DefaultLLMRuntime()
			rt.Model = ""
			rt.Endpoint = "https://x"
			Expect(rt.Validate("openai")).To(HaveOccurred())
		})
	})

	Describe("IT-CFG-011: LLMRuntimeConfig missing endpoint for custom provider", func() {
		It("Validate requires endpoint when provider is not a built-in remote", func() {
			rt := kacfg.DefaultLLMRuntime()
			rt.Model = "m"
			rt.Endpoint = ""
			Expect(rt.Validate("acme")).To(HaveOccurred())
		})
	})

	Describe("IT-CFG-012: CircuitBreakerCfg survives YAML via Load", func() {
		It("retains numeric duration and threshold fields", func() {
			yaml := `
ai:
  llm:
    circuitBreaker:
      enabled: true
      maxRequests: 7
      interval: 3s
      timeout: 8s
      failureThreshold: 9
      failureRatio: 0.55
`
			cfg, err := kacfg.Load([]byte(yaml))
			Expect(err).NotTo(HaveOccurred())
			cb := cfg.AI.LLM.CircuitBreaker
			Expect(cb.Enabled).To(BeTrue())
			Expect(cb.MaxRequests).To(Equal(uint32(7)))
			Expect(cb.Interval).To(Equal(3 * time.Second))
			Expect(cb.Timeout).To(Equal(8 * time.Second))
			Expect(cb.FailureThreshold).To(Equal(uint32(9)))
			Expect(cb.FailureRatio).To(Equal(0.55))
		})
	})
})
