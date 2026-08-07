package launcher_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

// Claude/Vertex-Claude LLM call timeout wiring (#1955, BR-AI-1955).
//
// These drive the real, exported production entry point
// (launcher.NewModelFromConfig, the same call cmd/apifrontend makes) rather
// than the unexported wrapWithTimeout/timeoutModel directly — proving the
// decorator is actually applied at the two real construction sites
// (newAnthropicModel, newVertexAnthropicModel), not just that the
// decorator's own logic works in isolation (covered separately by
// model_timeout_1955_internal_test.go's white-box specs).
var _ = Describe("Claude/Vertex-Claude LLM call timeout wiring (#1955, BR-AI-1955)", func() {
	Describe("NewModelFromConfig", func() {
		It("IT-AF-1955-003: wraps a direct-API anthropic model with the timeout decorator", func() {
			cfg := types.LLMConfig{
				Provider:       types.LLMProviderAnthropic,
				Model:          "claude-sonnet-4-20250514",
				APIKey:         "sk-ant-test-key",
				TimeoutSeconds: 45,
			}
			m, err := launcher.NewModelFromConfig(context.Background(), cfg)
			Expect(err).NotTo(HaveOccurred())

			timeout, wrapped := launcher.IsTimeoutWrappedForTest(m)
			Expect(wrapped).To(BeTrue(), "newAnthropicModel must wrap its result with the #1955 timeout decorator")
			Expect(timeout).To(Equal(45 * time.Second))
		})

		// IT-AF-1955-004: vertex_ai + a claude-* model, which main dispatches
		// (via newVertexAIModel's types.IsAnthropicModel check, #1778/#1792)
		// to newVertexAnthropicModel specifically — the vertex_ai + gemini-*
		// path (newVertexGeminiModel) already has its own timeout via
		// httpClient.Timeout and is untouched by this fix. Construction
		// requires ambient GCP ADC, which this environment may or may not
		// have — soft-checked exactly like this package's pre-existing
		// IT-AF-1792-002 (model_test.go), since either outcome still proves
		// the fix as long as a successful construction is wrapped.
		It("IT-AF-1955-004: wraps a Vertex-hosted claude model with the timeout decorator when construction succeeds", func() {
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
			timeout, wrapped := launcher.IsTimeoutWrappedForTest(m)
			Expect(wrapped).To(BeTrue(), "newVertexAnthropicModel must wrap its result with the #1955 timeout decorator")
			Expect(timeout).To(Equal(time.Duration(types.DefaultLLMTimeoutSeconds) * time.Second))
		})
	})
})
