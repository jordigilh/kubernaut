package launcher_test

import (
	"context"
	"fmt"

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

		// IT-AF-1792: vertex_ai + a Gemini model previously unconditionally
		// constructed an adk-anthropic-go (Claude-typed) model regardless of
		// the configured model — the AF counterpart of #1778's KA bug.
		// Proven fixed here through the real production dispatch
		// (NewModelFromConfig -> newVertexAIModel), not a direct call to an
		// unexported constructor (CHECKPOINT W). Ambient GCP ADC may or may
		// not be present in the test environment (ADC-dependent, matching
		// UT-AF-1252-004's own soft-check pattern above): either outcome
		// proves the fix as long as the underlying model.LLM's concrete
		// type is Gemini-family, never Claude-family, whenever construction
		// succeeds.
		It("IT-AF-1792-001: dispatches vertex_ai + a gemini-* model to a Gemini-backed model.LLM, not Claude", func() {
			cfg := types.LLMConfig{
				Provider:       types.LLMProviderVertexAI,
				Model:          "gemini-2.5-pro",
				VertexProject:  "test-project",
				VertexLocation: "us-central1",
			}
			m, err := launcher.NewModelFromConfig(context.Background(), cfg)
			if err != nil {
				Expect(err.Error()).To(Or(ContainSubstring("credentials"), ContainSubstring("ADC")))
				return
			}
			Expect(m).NotTo(BeNil())
			typeName := fmt.Sprintf("%T", m)
			Expect(typeName).To(ContainSubstring("gemini"),
				"vertex_ai + a gemini-* model must construct a Gemini-backed model.LLM (#1792 root-cause fix)")
			Expect(typeName).NotTo(ContainSubstring("anthropic"))
		})

		// IT-AF-1792-002: vertex_ai + a Claude model — no regression. Must
		// remain routed to adk-anthropic-go exactly as before this fix.
		It("IT-AF-1792-002: still dispatches vertex_ai + a claude-* model to an Anthropic-backed model.LLM", func() {
			cfg := types.LLMConfig{
				Provider:       types.LLMProviderVertexAI,
				Model:          "claude-sonnet-4-20250514",
				VertexProject:  "test-project",
				VertexLocation: "us-central1",
			}
			m, err := launcher.NewModelFromConfig(context.Background(), cfg)
			if err != nil {
				Expect(err.Error()).To(ContainSubstring("GCP ADC unavailable"))
				return
			}
			Expect(m).NotTo(BeNil())
			// #1955 now wraps the Anthropic/Vertex-Anthropic construction
			// sites in a *launcher.timeoutModel decorator, so unwrap it
			// before asserting on the underlying provider's concrete type —
			// this spec's job is the dispatch (anthropic vs. gemini), not
			// the timeout wiring (covered separately by
			// model_timeout_1955_test.go's IT-AF-1955-004).
			Expect(fmt.Sprintf("%T", launcher.UnwrapTimeoutForTest(m))).To(ContainSubstring("anthropic"))
		})

		// IT-AF-1792-005: vertex_ai + an unrecognized model family. Found
		// during the post-merge GA readiness audit: before this fix, a
		// model that is neither claude-* nor gemini-* silently fell
		// through newVertexAIModel's implicit else-branch to
		// newVertexGeminiModel, failing later with a confusing
		// Gemini-SDK-level error instead of a clear one here.
		It("IT-AF-1792-005: vertex_ai with an unrecognized model family fails fast with a clear error", func() {
			cfg := types.LLMConfig{
				Provider:       types.LLMProviderVertexAI,
				Model:          "llama-3.1-70b",
				VertexProject:  "test-project",
				VertexLocation: "us-central1",
			}
			m, err := launcher.NewModelFromConfig(context.Background(), cfg)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unrecognized model family"))
			Expect(err.Error()).To(ContainSubstring("llama-3.1-70b"))
			Expect(m).To(BeNil())
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
