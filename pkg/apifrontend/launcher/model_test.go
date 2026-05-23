package launcher_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/config"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
)

var _ = Describe("Model Factory", func() {
	Describe("NewModelFromConfig", func() {
		It("UT-AF-1252-001: rejects unsupported provider", func() {
			cfg := config.LLMConfig{Provider: "unsupported", Model: "test"}
			_, err := launcher.NewModelFromConfig(context.Background(), cfg)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unsupported LLM provider"))
		})

		It("UT-AF-1252-002: constructs gemini model with API key and endpoint", func() {
			cfg := config.LLMConfig{
				Provider: config.LLMProviderGemini,
				Model:    "gemini-2.0-flash",
				Endpoint: "http://localhost:8888/v1",
				APIKey:   "test-key",
			}
			m, err := launcher.NewModelFromConfig(context.Background(), cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(m).NotTo(BeNil())
			Expect(m.Name()).To(ContainSubstring("gemini"))
		})

		It("UT-AF-1252-003: constructs anthropic model with API key", func() {
			cfg := config.LLMConfig{
				Provider: config.LLMProviderAnthropic,
				Model:    "claude-sonnet-4-20250514",
				APIKey:   "sk-ant-test-key",
			}
			m, err := launcher.NewModelFromConfig(context.Background(), cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(m).NotTo(BeNil())
		})

		It("UT-AF-1252-004: vertex_ai requires valid GCP project/location", func() {
			cfg := config.LLMConfig{
				Provider:       config.LLMProviderVertexAI,
				Model:          "claude-sonnet-4-20250514",
				VertexProject:  "test-project",
				VertexLocation: "us-central1",
			}
			// This will fail in CI without real GCP ADC, but should not panic
			_, err := launcher.NewModelFromConfig(context.Background(), cfg)
			// Accept both success (if ADC is available) and specific auth error
			if err != nil {
				Expect(err.Error()).To(SatisfyAny(
					ContainSubstring("credentials"),
					ContainSubstring("auth"),
					ContainSubstring("token"),
					ContainSubstring("transport"),
					ContainSubstring("could not find"),
				))
			}
		})
	})

	Describe("A2AConfig", func() {
		It("UT-AF-210-014: A2AConfig validation rejects nil Agent", func() {
			_, err := launcher.NewA2AHandler(launcher.A2AConfig{
				Agent:          nil,
				SessionService: nil,
				AppName:        "test",
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("agent"))
		})
	})
})
