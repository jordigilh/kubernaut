package severity_test

import (
	"context"
	"sync"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	prom "github.com/jordigilh/kubernaut/pkg/apifrontend/prometheus"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/severity"
)

type triageAuditSpy struct {
	mu     sync.Mutex
	events []*audit.Event
}

func (s *triageAuditSpy) Emit(_ context.Context, event *audit.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
}

func (s *triageAuditSpy) eventsByType(t audit.EventType) []*audit.Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*audit.Event
	for _, e := range s.events {
		if e.Type == t {
			out = append(out, e)
		}
	}
	return out
}

var _ = Describe("Audit event emission – severity triage (PR2 wiring)", func() {
	var (
		spy *triageAuditSpy
		cfg severity.Config
	)

	BeforeEach(func() {
		spy = &triageAuditSpy{}
		cfg = severity.Config{
			Enabled:           true,
			MaxQueriesPerCall: 10,
			MaxRulesEvaluated: 100,
			CacheTTLSeconds:   30,
			LLMConfidence:     0.7,
		}
	})

	It("UT-AF-1156-055: emits severity_triage.completed on successful triage", func() {
		mockProm := &mockPromClient{
			alerts: []prom.Alert{
				{State: "firing", Labels: map[string]string{
					"alertname": "HighErrorRate",
					"namespace": "prod",
					"kind":      "Deployment",
					"name":      "web",
					"severity":  "critical",
				}},
			},
		}

		noopLLM := severity.NewNoopLLMTriager(logr.Discard())
		triager := severity.NewTriager(mockProm, noopLLM, cfg, logr.Discard(), severity.WithAuditor(spy))

		result, err := triager.Triage(context.Background(), severity.TriageInput{
			Namespace:   "prod",
			Kind:        "Deployment",
			Name:        "web",
			Description: "errors spiking",
			Labels:      map[string]string{"namespace": "prod", "kind": "Deployment", "name": "web"},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Severity).NotTo(BeEmpty())

		events := spy.eventsByType(audit.EventSeverityTriageCompleted)
		Expect(events).To(HaveLen(1), "expected exactly one severity_triage.completed event")
		Expect(events[0].Detail).To(HaveKeyWithValue("severity", "critical"))
		Expect(events[0].Detail).To(HaveKey("source"))
	})

	It("UT-AF-1156-056: emits severity_triage.failed on triage error", func() {
		failingProm := &mockPromClient{
			alertsErr: context.DeadlineExceeded,
			rulesErr:  context.DeadlineExceeded,
		}
		failingLLM := &mockLLM{
			ruleErr: context.DeadlineExceeded,
			pureErr: context.DeadlineExceeded,
		}

		triager := severity.NewTriager(failingProm, failingLLM, cfg, logr.Discard(), severity.WithAuditor(spy))

		_, err := triager.Triage(context.Background(), severity.TriageInput{
			Namespace:   "prod",
			Kind:        "Deployment",
			Name:        "web",
			Description: "errors",
			Labels:      map[string]string{"namespace": "prod"},
		})
		_ = err

		events := spy.eventsByType(audit.EventSeverityTriageFailed)
		Expect(events).To(HaveLen(1), "expected exactly one severity_triage.failed event")
		Expect(events[0].Detail).To(HaveKey("error"))
	})
})
