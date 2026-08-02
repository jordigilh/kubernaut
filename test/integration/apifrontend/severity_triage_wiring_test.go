package apifrontend_test

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/genai"

	prom "github.com/jordigilh/kubernaut/pkg/apifrontend/prometheus"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/severity"
)

// wiringTestRuleGroups returns a Tier 2.5 setup: one inactive rule whose
// query label-matches the target resource, but with no live data behind it
// (podCorrelationPromClient.InstantQuery always returns an empty result).
// This forces the pipeline through Tier 2.5 (LLM-with-rule-context) rather
// than Tier 1/1.5/2, which is what these tests exist to prove is wired to
// the LLM implementation under test (#1839 removed the old Tier 3 route to
// the same LLM.TriageWithRules/classify() code path these tests rely on).
func wiringTestRuleGroups() []prom.RuleGroup {
	return []prom.RuleGroup{
		{
			Name: "wiring-test",
			Rules: []prom.Rule{
				{
					Name:   "WiringTestRule",
					Query:  fmt.Sprintf(`rate(requests{namespace="%s"}[5m])`, defaultFixture),
					State:  "inactive",
					Labels: map[string]string{"severity": "high"},
				},
			},
		},
	}
}

type spyContentGenerator struct {
	callCount atomic.Int32
}

func (s *spyContentGenerator) GenerateContent(_ context.Context, _ string, _ []*genai.Content, _ *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
	s.callCount.Add(1)
	return &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{
			{
				Content: &genai.Content{
					Parts: []*genai.Part{{Text: "critical"}},
				},
			},
		},
	}, nil
}

var _ = Describe("Severity Triage LLM Wiring", func() {
	It("IT-AF-SEV-W01: GenAITriager routes triage calls to the LLM generator (not noop)", func() {
		spy := &spyContentGenerator{}
		triager := severity.NewGenAITriager(severity.GenAITriagerConfig{
			Generator: spy,
			Model:     "gemini-2.0-flash",
			Logger:    logr.Discard(),
		})

		promClient := &podCorrelationPromClient{alerts: nil, ruleGroups: wiringTestRuleGroups()}

		pipeline := severity.NewTriager(
			promClient,
			triager,
			severity.DefaultConfig(),
			logr.Discard(),
		)

		result, err := pipeline.Triage(context.Background(), severity.TriageInput{
			Kind:        "Deployment",
			Name:        "test-workload",
			Namespace:   defaultFixture,
			Description: "test workload failing",
			Labels:      map[string]string{"namespace": defaultFixture, "kind": "Deployment", "name": "test-workload"},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(spy.callCount.Load()).To(BeNumerically(">", 0),
			"GenAITriager must delegate to the ContentGenerator — 0 calls means noop was used")
		Expect(result.Severity).To(Equal("critical"),
			"severity must come from the LLM response, not the noop default of 'medium'")
	})

	It("IT-AF-SEV-W01b: NoopLLMTriager always returns medium (control test)", func() {
		noop := severity.NewNoopLLMTriager(logr.Discard())

		promClient := &podCorrelationPromClient{alerts: nil, ruleGroups: wiringTestRuleGroups()}

		pipeline := severity.NewTriager(
			promClient,
			noop,
			severity.DefaultConfig(),
			logr.Discard(),
		)

		result, err := pipeline.Triage(context.Background(), severity.TriageInput{
			Kind:        "Deployment",
			Name:        "test-workload",
			Namespace:   defaultFixture,
			Description: "test workload failing",
			Labels:      map[string]string{"namespace": defaultFixture, "kind": "Deployment", "name": "test-workload"},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Severity).To(Equal("warning"),
			"NoopLLMTriager must return 'warning' — this is the control case")
	})
})
