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

package main

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/config"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

// IT-AF-1599-001: AF has no live-swap mechanism for any LLM config field
// (unlike KA's SwappableClient), so it is restart-only by construction. This
// test proves that property holds for the CM-02 config-drift-detection
// callback specifically: a hot-reload attempt that changes LLM Provider/Model
// (or any other field) must validate successfully (for audit/drift-detection
// purposes) without ever mutating the live *config.Config passed to
// startConfigWatcher. See #1599.
var _ = Describe("configReloadCallback — restart-required LLM identity (#1599)", func() {
	It("validates a Provider/Model change but never mutates the live config", func() {
		cfg := &config.Config{}
		cfg.Agent.LLM.Provider = "openai"
		cfg.Agent.LLM.Model = "gpt-4o"
		cfg.SeverityTriage.LLM = &types.LLMConfig{Provider: "openai", Model: "gpt-4o-mini"}

		originalAgentProvider := cfg.Agent.LLM.Provider
		originalAgentModel := cfg.Agent.LLM.Model
		originalTriageProvider := cfg.SeverityTriage.LLM.Provider
		originalTriageModel := cfg.SeverityTriage.LLM.Model

		cb := configReloadCallback(cfg)

		keyFile := filepath.Join(GinkgoT().TempDir(), "anthropic-key")
		Expect(os.WriteFile(keyFile, []byte("test-key-value"), 0o600)).To(Succeed())

		// Start from a fully-valid default config and only change the fields
		// under test (Provider/Model, base and SeverityTriage), so the fixture
		// doesn't need to hand-duplicate every other required field/default.
		candidate := config.DefaultConfig()
		candidate.Agent.LLM.Provider = "anthropic"
		candidate.Agent.LLM.Model = "claude-3-5-sonnet"
		candidate.Agent.LLM.APIKeyFile = keyFile
		candidate.SeverityTriage.LLM = &types.LLMConfig{
			Provider:   "anthropic",
			Model:      "claude-haiku",
			APIKeyFile: keyFile,
		}
		newContent, marshalErr := yaml.Marshal(candidate)
		Expect(marshalErr).NotTo(HaveOccurred())

		err := cb(newContent)
		Expect(err).NotTo(HaveOccurred(), "candidate content must still pass audit/drift-detection validation")

		Expect(cfg.Agent.LLM.Provider).To(Equal(originalAgentProvider), "live config Provider must never be mutated by reload")
		Expect(cfg.Agent.LLM.Model).To(Equal(originalAgentModel), "live config Model must never be mutated by reload")
		Expect(cfg.SeverityTriage.LLM.Provider).To(Equal(originalTriageProvider), "live SeverityTriage Provider must never be mutated by reload")
		Expect(cfg.SeverityTriage.LLM.Model).To(Equal(originalTriageModel), "live SeverityTriage Model must never be mutated by reload")
	})

	It("rejects invalid content and never mutates the live config", func() {
		cfg := &config.Config{}
		cfg.Agent.LLM.Provider = "openai"
		cfg.Agent.LLM.Model = "gpt-4o"

		cb := configReloadCallback(cfg)

		err := cb([]byte("not: valid: yaml: [structure"))
		Expect(err).To(HaveOccurred())
		Expect(cfg.Agent.LLM.Provider).To(Equal("openai"))
		Expect(cfg.Agent.LLM.Model).To(Equal("gpt-4o"))
	})
})
