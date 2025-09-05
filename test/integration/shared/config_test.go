//go:build integration
// +build integration

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
		Entry("any host with port 11434", "http://example.com:11434", "ollama"),
		Entry("localhost with port 8080", "http://localhost:8080", "localai"),
		Entry("any host with port 8080", "http://example.com:8080", "localai"),
		Entry("custom port", "http://example.com:9999", "localai"),
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
		envHelper.CaptureEnvironment("LLM_ENDPOINT", "LLM_MODEL", "LLM_PROVIDER")
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
		})

		It("should load default configuration values", func() {
			cfg := LoadConfig()

			Expect(cfg.LLMEndpoint).To(Equal("http://localhost:11434"))
			Expect(cfg.LLMModel).To(Equal("granite3.1-dense:8b"))
			// Provider should be auto-detected from endpoint
			Expect(cfg.LLMProvider).To(Equal("ollama"))
		})
	})
})
