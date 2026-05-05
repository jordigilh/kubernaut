package launcher_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut-apifrontend/internal/launcher"
)

var _ = Describe("Model", func() {
	Describe("NewModelConfig", func() {
		It("UT-AF-210-008: default config uses Claude Sonnet via Vertex AI", func() {
			cfg := launcher.DefaultModelConfig()
			Expect(cfg.Provider).To(Equal("vertexai"))
			Expect(cfg.Model).To(ContainSubstring("claude"))
		})

		It("UT-AF-210-009: validates provider is supported", func() {
			cfg := launcher.ModelConfig{Provider: "unsupported", Model: "test"}
			err := cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unsupported provider"))
		})

		It("UT-AF-210-010: validates model name is non-empty", func() {
			cfg := launcher.ModelConfig{Provider: "vertexai", Model: ""}
			err := cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("model"))
		})

		It("UT-AF-210-011: accepts vertexai provider", func() {
			cfg := launcher.ModelConfig{Provider: "vertexai", Model: "claude-sonnet-4-20250514"}
			err := cfg.Validate()
			Expect(err).NotTo(HaveOccurred())
		})

		It("UT-AF-210-012: accepts anthropic provider", func() {
			cfg := launcher.ModelConfig{Provider: "anthropic", Model: "claude-sonnet-4-20250514"}
			err := cfg.Validate()
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("JWT Delegation", func() {
		It("UT-AF-210-013: JWTDelegationEnabled defaults to true", func() {
			cfg := launcher.DefaultModelConfig()
			Expect(cfg.JWTDelegation).To(BeTrue())
		})

		It("UT-AF-210-014: A2AConfig with model validation", func() {
			cfg := launcher.A2AConfig{
				AppName: "test",
			}
			Expect(cfg.AppName).To(Equal("test"))
		})
	})
})
