//go:build integration
// +build integration

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

package shared

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
)

func TestConfigLoading(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Configuration Loading Suite")
}

var _ = Describe("Provider Detection", func() {
	DescribeTable("should detect correct provider from endpoint",
		func(endpoint string, expectedProvider string) {
			result := detectProviderFromEndpoint(endpoint)
			Expect(result).To(Equal(expectedProvider))
		},
		Entry("localhost with port 11434", "http://localhost:11434", "ollama"),
		Entry("any host with port 11434", "http://kubernaut.io:11434", "ollama"),
		Entry("localhost with port 8080", "http://localhost:8080", "ramalama"),
		Entry("any host with port 8080", "http://kubernaut.io:8080", "ramalama"),
		Entry("custom port", "http://kubernaut.io:9999", "ramalama"),
	)
})

var _ = Describe("Configuration Loading", func() {
	var (
		envHelper *EnvironmentIsolationHelper
	)

	BeforeEach(func() {
		logger := logrus.New()
		logger.SetLevel(logrus.WarnLevel) // Reduce noise during tests

		envHelper = NewEnvironmentIsolationHelper(logger)

		// Capture current state of LLM environment variables
		envHelper.CaptureEnvironment("LLM_ENDPOINT", "LLM_MODEL", "LLM_PROVIDER", "USE_MOCK_LLM")
	})

	AfterEach(func() {
		// Restore original environment variables automatically
		envHelper.RestoreEnvironment()
	})

	Context("with custom environment variables", func() {
		BeforeEach(func() {
			envHelper.SetEnvironment(map[string]string{
				"LLM_ENDPOINT": "http://test:8080",
				"LLM_MODEL":    "test-model",
				"LLM_PROVIDER": "ramalama",
				"USE_MOCK_LLM": "false", // Disable mock mode for this test
			})
		})

		It("should load custom configuration values", func() {
			cfg := LoadConfig()

			Expect(cfg.LLMEndpoint).To(Equal("http://test:8080"))
			Expect(cfg.LLMModel).To(Equal("test-model"))
			Expect(cfg.LLMProvider).To(Equal("ramalama"))
		})
	})

	Context("with default environment variables", func() {
		BeforeEach(func() {
			envHelper.UnsetEnvironment("LLM_ENDPOINT", "LLM_MODEL", "LLM_PROVIDER")
			envHelper.SetEnvironment(map[string]string{
				"USE_MOCK_LLM": "false", // Disable mock mode for this test
			})
		})

		It("should load default configuration values", func() {
			cfg := LoadConfig()

			Expect(cfg.LLMEndpoint).To(Equal("http://localhost:8010"))
			Expect(cfg.LLMModel).To(Equal("ggml-org/gpt-oss-20b-GGUF"))
			// Provider should be auto-detected from endpoint
			Expect(cfg.LLMProvider).To(Equal("ramalama"))
		})
	})

	Context("with mock LLM enabled", func() {
		BeforeEach(func() {
			envHelper.SetEnvironment(map[string]string{
				"USE_MOCK_LLM": "true", // Enable mock mode for this test
			})
		})

		It("should load mock configuration values", func() {
			cfg := LoadConfig()

			Expect(cfg.LLMEndpoint).To(Equal("mock://localhost:8080"))
			Expect(cfg.LLMModel).To(Equal("mock-model"))
			Expect(cfg.LLMProvider).To(Equal("mock"))
			Expect(cfg.UseMockLLM).To(BeTrue())
		})
	})
})
