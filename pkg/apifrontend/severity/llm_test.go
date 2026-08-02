package severity_test

import (
	"context"
	"errors"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/genai"

	prom "github.com/jordigilh/kubernaut/pkg/apifrontend/prometheus"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/severity"
)

var _ = Describe("LLM Triage", func() {
	var defaultInput severity.TriageInput

	BeforeEach(func() {
		defaultInput = severity.TriageInput{
			Namespace:   "prod",
			Kind:        "Deployment",
			Name:        "web-api",
			Description: "High error rate",
			Labels:      map[string]string{"namespace": "prod"},
		}
	})

	Describe("Tier 2.5: LLM with Rule Context", func() {
		It("UT-AF-T-047: prompt includes rule name, expression, annotations, severity", func() {
			capturedRules := []prom.Rule(nil)
			mock := &promptCaptureLLM{
				result: severity.TriageResult{Severity: "high", Source: severity.SourceLLMRuleInform},
				captureRules: func(rules []prom.Rule) {
					capturedRules = rules
				},
			}
			rules := []prom.Rule{
				{
					Name:        "HighCPU",
					Query:       `rate(cpu{namespace="prod"}[5m]) > 0.9`,
					Labels:      map[string]string{"severity": "critical"},
					Annotations: map[string]string{"summary": "CPU is too high"},
				},
			}
			result, err := mock.TriageWithRules(context.Background(), rules, defaultInput)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Severity).To(Equal("high"))
			Expect(capturedRules).To(HaveLen(1))
			Expect(capturedRules[0].Name).To(Equal("HighCPU"))
		})
	})

	Describe("Response Validation", func() {
		It("UT-AF-T-049: valid severity accepted (ADR-066: critical > high > warning > info)", func() {
			Expect(severity.ValidateSeverity("critical")).To(BeTrue())
			Expect(severity.ValidateSeverity("high")).To(BeTrue())
			Expect(severity.ValidateSeverity("warning")).To(BeTrue())
			Expect(severity.ValidateSeverity("info")).To(BeTrue())
		})

		It("UT-AF-T-049b: medium and low are no longer valid canonical severities (ADR-066)", func() {
			Expect(severity.ValidateSeverity("medium")).To(BeFalse())
			Expect(severity.ValidateSeverity("low")).To(BeFalse())
		})

		It("UT-AF-T-050: invalid severity string rejected", func() {
			Expect(severity.ValidateSeverity("CRITICAL")).To(BeFalse())
			Expect(severity.ValidateSeverity("urgent")).To(BeFalse())
			Expect(severity.ValidateSeverity("")).To(BeFalse())
			Expect(severity.ValidateSeverity("p1")).To(BeFalse())
		})

		It("UT-AF-T-082: empty LLM response defaults to warning", func() {
			normalized := severity.NormalizeSeverity("")
			Expect(normalized).To(Equal("warning"))
		})

		It("UT-AF-T-083: CRITICAL (wrong case) normalized to critical", func() {
			normalized := severity.NormalizeSeverity("CRITICAL")
			Expect(normalized).To(Equal("critical"))

			normalized = severity.NormalizeSeverity("High")
			Expect(normalized).To(Equal("high"))
		})
	})

	Describe("Error Handling", func() {
		It("UT-AF-T-052: LLM call error returns error", func() {
			mock := &mockLLM{
				ruleErr: errors.New("LLM unavailable"),
			}
			_, err := mock.TriageWithRules(context.Background(), nil, defaultInput)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("LLM unavailable"))
		})
	})

	Describe("NoopLLMTriager", func() {
		It("UT-AF-T-091: TriageWithRules always returns warning with full confidence", func() {
			noop := severity.NewNoopLLMTriager(logr.Discard())
			rules := []prom.Rule{{Name: "SomeRule", Query: "up == 0"}}
			result, err := noop.TriageWithRules(context.Background(), rules, defaultInput)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Severity).To(Equal("warning"))
			Expect(result.Confidence).To(Equal(1.0))
		})
	})

	Describe("GenAITriager Construction", func() {
		It("UT-AF-T-092: panics with nil client", func() {
			Expect(func() {
				severity.NewGenAITriager(severity.GenAITriagerConfig{Client: nil})
			}).To(Panic())
		})

		It("UT-AF-T-093: defaults Model when a Generator is supplied without one", func() {
			gen := &genAIStubGenerator{result: genaiTextResponse("warning")}
			triager := severity.NewGenAITriager(severity.GenAITriagerConfig{Generator: gen})

			result, err := triager.TriageWithRules(context.Background(), nil, defaultInput)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Severity).To(Equal("warning"))
		})

		It("UT-AF-T-094: wraps a bare Client in genaiModels when Generator is unset (construction-only, no network call)", func() {
			Expect(func() {
				severity.NewGenAITriager(severity.GenAITriagerConfig{Client: &genai.Client{Models: &genai.Models{}}})
			}).NotTo(Panic())
		})
	})

	Describe("extractText edge cases", func() {
		It("UT-AF-T-095: response with zero candidates yields empty text (rejected upstream by classify)", func() {
			gen := &genAIStubGenerator{result: &genai.GenerateContentResponse{Candidates: []*genai.Candidate{}}}
			triager := severity.NewGenAITriager(severity.GenAITriagerConfig{Generator: gen})

			_, err := triager.TriageWithRules(context.Background(), nil, defaultInput)
			Expect(err).To(MatchError(ContainSubstring("empty response")))
		})

		It("UT-AF-T-096: candidate with only blank-text parts yields empty text (rejected upstream by classify)", func() {
			gen := &genAIStubGenerator{result: &genai.GenerateContentResponse{
				Candidates: []*genai.Candidate{
					{Content: &genai.Content{Parts: []*genai.Part{{Text: ""}}}},
				},
			}}
			triager := severity.NewGenAITriager(severity.GenAITriagerConfig{Generator: gen})

			_, err := triager.TriageWithRules(context.Background(), nil, defaultInput)
			Expect(err).To(MatchError(ContainSubstring("empty response")))
		})
	})

	Describe("Prompt Safety", func() {
		It("UT-AF-T-055b: prompt includes rule summary annotation when present", func() {
			rules := []prom.Rule{
				{
					Name:        "HighCPU",
					Query:       `rate(cpu{namespace="prod"}[5m]) > 0.9`,
					Labels:      map[string]string{"severity": "critical"},
					Annotations: map[string]string{"summary": "CPU is too high"},
				},
			}
			prompt := severity.BuildTriagePrompt(defaultInput, rules)
			Expect(prompt).To(ContainSubstring("Summary: CPU is too high"))
		})

		It("UT-AF-T-055: LLM prompt does not contain secrets", func() {
			input := severity.TriageInput{
				Namespace:   "prod",
				Kind:        "Deployment",
				Name:        "web-api",
				Description: "error on service",
				Labels: map[string]string{
					"namespace": "prod",
					"password":  "should-not-appear",
				},
			}
			prompt := severity.BuildTriagePrompt(input, nil)
			Expect(prompt).NotTo(ContainSubstring("should-not-appear"))
			Expect(prompt).To(ContainSubstring("prod"))
			Expect(prompt).To(ContainSubstring("Deployment"))
		})
	})
})

// --- Test helpers ---

type promptCaptureLLM struct {
	result       severity.TriageResult
	err          error
	captureRules func([]prom.Rule)
	captureInput func(severity.TriageInput)
}

func (m *promptCaptureLLM) TriageWithRules(_ context.Context, rules []prom.Rule, input severity.TriageInput) (severity.TriageResult, error) {
	if m.captureRules != nil {
		m.captureRules(rules)
	}
	if m.captureInput != nil {
		m.captureInput(input)
	}
	return m.result, m.err
}

// genAIStubGenerator implements severity.ContentGenerator for GenAITriager
// tests, avoiding a live Vertex AI dependency.
type genAIStubGenerator struct {
	result *genai.GenerateContentResponse
	err    error
}

func (g *genAIStubGenerator) GenerateContent(_ context.Context, _ string, _ []*genai.Content, _ *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
	return g.result, g.err
}

func genaiTextResponse(text string) *genai.GenerateContentResponse {
	return &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{
			{Content: &genai.Content{Parts: []*genai.Part{{Text: text}}}},
		},
	}
}
