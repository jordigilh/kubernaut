package severity

import (
	"context"

	prom "github.com/jordigilh/kubernaut-apifrontend/internal/prometheus"
)

// llmTriager is the concrete implementation of LLMTriager.
type llmTriager struct{}

func (l *llmTriager) TriageWithRules(_ context.Context, _ []prom.Rule, _ TriageInput) (TriageResult, error) {
	return TriageResult{}, nil
}

func (l *llmTriager) TriagePure(_ context.Context, _ TriageInput) (TriageResult, error) {
	return TriageResult{}, nil
}
