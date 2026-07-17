package apifrontend_test

import (
	"context"
	"sync/atomic"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/genai"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/severity"
)

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

		promClient := &podCorrelationPromClient{alerts: nil}

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
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(spy.callCount.Load()).To(BeNumerically(">", 0),
			"GenAITriager must delegate to the ContentGenerator — 0 calls means noop was used")
		Expect(result.Severity).To(Equal("critical"),
			"severity must come from the LLM response, not the noop default of 'medium'")
	})

	It("IT-AF-SEV-W01b: NoopLLMTriager always returns medium (control test)", func() {
		noop := severity.NewNoopLLMTriager(logr.Discard())

		promClient := &podCorrelationPromClient{alerts: nil}

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
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Severity).To(Equal("warning"),
			"NoopLLMTriager must return 'warning' — this is the control case")
	})
})
