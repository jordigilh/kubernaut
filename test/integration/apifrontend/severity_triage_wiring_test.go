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

// wiringTestRuleGroups returns a Tier 2 setup: one inactive rule whose
// query label-matches the target resource, but with no live data behind it
// (podCorrelationPromClient.InstantQuery always returns an empty result).
// This forces the pipeline through the explicit rule-label path. DD-AF-016
// requires this source-owned severity to win without invoking an LLM.
func wiringTestRuleGroups() []prom.RuleGroup {
	return []prom.RuleGroup{
		{
			Name: "wiring-test",
			Rules: []prom.Rule{
				{
					Name:   "WiringTestRule",
					Query:  fmt.Sprintf(`rate(requests{namespace="%s"}[5m])`, defaultFixture),
					State:  "inactive",
					Labels: map[string]string{"severity": "high", "cluster": hubClusterID},
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

var _ = Describe("Severity Triage Source Wiring", func() {
	It("IT-AF-SEV-W01: explicit rule severity bypasses the LLM triager (DD-AF-016)", func() {
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
			ClusterID:   hubClusterID,
			Labels:      map[string]string{"namespace": defaultFixture, "kind": "Deployment", "name": "test-workload"},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(spy.callCount.Load()).To(Equal(int32(0)),
			"explicit rule severity must not invoke the LLM triager")
		Expect(result.Severity).To(Equal("high"),
			"severity must come unchanged from the explicit rule label")
		Expect(result.Source).To(Equal(severity.SourceRuleLabel))
	})

	It("IT-AF-SEV-W01b: noop triager cannot override explicit rule severity (control test)", func() {
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
			ClusterID:   hubClusterID,
			Labels:      map[string]string{"namespace": defaultFixture, "kind": "Deployment", "name": "test-workload"},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Severity).To(Equal("high"),
			"the explicit rule label must win over the noop triager")
		Expect(result.Source).To(Equal(severity.SourceRuleLabel))
	})
})
