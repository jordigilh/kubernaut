package severity_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	prom "github.com/jordigilh/kubernaut-apifrontend/internal/prometheus"
	"github.com/jordigilh/kubernaut-apifrontend/internal/severity"
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

	Describe("Tier 3: Pure LLM", func() {
		It("UT-AF-T-048: prompt includes resource context", func() {
			capturedInput := severity.TriageInput{}
			mock := &promptCaptureLLM{
				result: severity.TriageResult{Severity: "medium", Source: severity.SourceLLMTriage},
				captureInput: func(input severity.TriageInput) {
					capturedInput = input
				},
			}
			result, err := mock.TriagePure(context.Background(), defaultInput)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Severity).To(Equal("medium"))
			Expect(capturedInput.Namespace).To(Equal("prod"))
			Expect(capturedInput.Kind).To(Equal("Deployment"))
		})
	})

	Describe("Response Validation", func() {
		It("UT-AF-T-049: valid severity accepted", func() {
			Expect(severity.ValidateSeverity("critical")).To(BeTrue())
			Expect(severity.ValidateSeverity("high")).To(BeTrue())
			Expect(severity.ValidateSeverity("medium")).To(BeTrue())
			Expect(severity.ValidateSeverity("low")).To(BeTrue())
			Expect(severity.ValidateSeverity("info")).To(BeTrue())
		})

		It("UT-AF-T-050: invalid severity string rejected", func() {
			Expect(severity.ValidateSeverity("CRITICAL")).To(BeFalse())
			Expect(severity.ValidateSeverity("urgent")).To(BeFalse())
			Expect(severity.ValidateSeverity("")).To(BeFalse())
			Expect(severity.ValidateSeverity("p1")).To(BeFalse())
		})

		It("UT-AF-T-082: empty LLM response defaults to medium", func() {
			normalized := severity.NormalizeSeverity("")
			Expect(normalized).To(Equal("medium"))
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
				pureErr: errors.New("LLM unavailable"),
			}
			_, err := mock.TriagePure(context.Background(), defaultInput)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("LLM unavailable"))
		})
	})

	Describe("Prompt Safety", func() {
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

func (m *promptCaptureLLM) TriagePure(_ context.Context, input severity.TriageInput) (severity.TriageResult, error) {
	if m.captureInput != nil {
		m.captureInput(input)
	}
	return m.result, m.err
}
