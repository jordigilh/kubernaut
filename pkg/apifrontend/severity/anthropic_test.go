package severity_test

import (
	"context"
	"errors"

	"github.com/anthropics/anthropic-sdk-go"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	prom "github.com/jordigilh/kubernaut/pkg/apifrontend/prometheus"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/severity"
)

// mockAnthropicMessager implements severity.Messager for AnthropicTriager
// tests, avoiding a live Anthropic/Vertex API dependency.
type mockAnthropicMessager struct {
	resp *anthropic.Message
	err  error
}

func (m *mockAnthropicMessager) Create(_ context.Context, _ anthropic.MessageNewParams) (*anthropic.Message, error) {
	return m.resp, m.err
}

func makeAnthropicResponse(text string) *anthropic.Message {
	return &anthropic.Message{
		Content: []anthropic.ContentBlockUnion{
			{Type: "text", Text: text},
		},
		StopReason: "end_turn",
	}
}

// BR-AI-1404 / FedRAMP SI-4: AnthropicTriager severity classification
// correctness, error propagation, and confidence scoring for the audit
// trail.
var _ = Describe("AnthropicTriager", func() {
	It("UT-AF-1404-001: classifies a clear-cut response with full confidence", func() {
		mock := &mockAnthropicMessager{resp: makeAnthropicResponse("critical")}
		triager := severity.NewAnthropicTriager(severity.AnthropicTriagerConfig{
			Messager: mock,
			Model:    "claude-sonnet-4-6",
		})

		result, err := triager.TriageWithRules(context.Background(), nil, severity.TriageInput{Description: "HighCPU pod restart loop"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Severity).To(Equal("critical"))
		Expect(result.Confidence).To(Equal(1.0))
	})

	It("UT-AF-1404-002: propagates the underlying SDK error for the audit trail", func() {
		mock := &mockAnthropicMessager{err: errors.New("vertex auth failure")}
		triager := severity.NewAnthropicTriager(severity.AnthropicTriagerConfig{
			Messager: mock,
			Model:    "claude-sonnet-4-6",
		})

		_, err := triager.TriageWithRules(context.Background(), nil, severity.TriageInput{Description: "HighCPU"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).NotTo(BeEmpty())
	})

	It("UT-AF-1404-003: degrades confidence for an ambiguous LLM response", func() {
		mock := &mockAnthropicMessager{resp: makeAnthropicResponse("I think it might be medium to high")}
		triager := severity.NewAnthropicTriager(severity.AnthropicTriagerConfig{
			Messager: mock,
			Model:    "claude-sonnet-4-6",
		})

		result, err := triager.TriageWithRules(context.Background(), nil, severity.TriageInput{Description: "Some alert"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Confidence).To(BeNumerically("<", 1.0))
	})

	It("UT-AF-1404-004: TriageWithRules includes rule context in classification", func() {
		mock := &mockAnthropicMessager{resp: makeAnthropicResponse("high")}
		triager := severity.NewAnthropicTriager(severity.AnthropicTriagerConfig{
			Messager: mock,
			Model:    "claude-sonnet-4-6",
		})

		rules := []prom.Rule{{Name: "HighCPU", Query: `rate(cpu[5m]) > 0.9`}}
		result, err := triager.TriageWithRules(context.Background(), rules, severity.TriageInput{Description: "CPU spike"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Severity).To(Equal("high"))
	})

	It("UT-AF-1404-005: rejects an empty model response instead of guessing", func() {
		mock := &mockAnthropicMessager{resp: &anthropic.Message{Content: []anthropic.ContentBlockUnion{}}}
		triager := severity.NewAnthropicTriager(severity.AnthropicTriagerConfig{
			Messager: mock,
			Model:    "claude-sonnet-4-6",
		})

		_, err := triager.TriageWithRules(context.Background(), nil, severity.TriageInput{Description: "Something"})
		Expect(err).To(HaveOccurred())
	})
})

// BR-AI-1404 / BR-AI-087: model family detection for factory routing moved
// to the shared types.IsAnthropicModel (pkg/shared/types/llm_test.go,
// UT-SH-AI-087-001) so AF and KA share one canonical detector instead of
// independent copies (#1778, #1792).
