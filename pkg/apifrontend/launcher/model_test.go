package launcher_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

var _ = Describe("Model Factory", func() {
	Describe("NewModelFromConfig", func() {
		It("UT-AF-1252-001: rejects unsupported provider", func() {
			cfg := types.LLMConfig{Provider: "unsupported", Model: "test"}
			_, err := launcher.NewModelFromConfig(context.Background(), cfg)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unsupported LLM provider"))
		})

		It("UT-AF-1252-002: constructs gemini model with API key and endpoint", func() {
			cfg := types.LLMConfig{
				Provider: types.LLMProviderGemini,
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
			cfg := types.LLMConfig{
				Provider: types.LLMProviderAnthropic,
				Model:    "claude-sonnet-4-20250514",
				APIKey:   "sk-ant-test-key",
			}
			m, err := launcher.NewModelFromConfig(context.Background(), cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(m).NotTo(BeNil())
		})

		It("UT-AF-1252-004: vertex_ai requires valid GCP project/location", func() {
			cfg := types.LLMConfig{
				Provider:       types.LLMProviderVertexAI,
				Model:          "claude-sonnet-4-20250514",
				VertexProject:  "test-project",
				VertexLocation: "us-central1",
			}
			_, err := launcher.NewModelFromConfig(context.Background(), cfg)
			if err != nil {
				Expect(err.Error()).To(ContainSubstring("GCP ADC unavailable"))
			}
		})

		// UT-AF-1254-010: factory dispatches to openai_compatible adapter
		It("UT-AF-1254-010: constructs openai_compatible model with endpoint", func() {
			cfg := types.LLMConfig{
				Provider: types.LLMProviderOpenAICompatible,
				Model:    "llama3.1",
				Endpoint: "http://llamastack:8080/v1",
			}
			m, err := launcher.NewModelFromConfig(context.Background(), cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(m).NotTo(BeNil())
			Expect(m.Name()).To(Equal("llama3.1"))
		})

		// UT-AF-1254-011: factory dispatches to openai adapter
		It("UT-AF-1254-011: constructs openai model with API key and endpoint", func() {
			cfg := types.LLMConfig{
				Provider: types.LLMProviderOpenAI,
				Model:    "gpt-4o",
				Endpoint: "https://api.openai.com/v1",
				APIKey:   "sk-test-key",
			}
			m, err := launcher.NewModelFromConfig(context.Background(), cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(m).NotTo(BeNil())
			Expect(m.Name()).To(Equal("gpt-4o"))
		})

		// UT-AF-1254-012: factory constructs openai_compatible without API key (keyless)
		It("UT-AF-1254-012: constructs openai_compatible without API key (keyless)", func() {
			cfg := types.LLMConfig{
				Provider: types.LLMProviderOpenAICompatible,
				Model:    "llama3.1",
				Endpoint: "http://llamastack:8080/v1",
				APIKey:   "",
			}
			m, err := launcher.NewModelFromConfig(context.Background(), cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(m).NotTo(BeNil())
		})

		// UT-AF-1254-013: factory still rejects truly unknown providers
		It("UT-AF-1254-013: still rejects unknown provider after adding openai", func() {
			cfg := types.LLMConfig{Provider: "totally_fake", Model: "test"}
			_, err := launcher.NewModelFromConfig(context.Background(), cfg)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unsupported LLM provider"))
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
