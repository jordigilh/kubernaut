package severity

import (
	"context"
	"errors"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"

	prom "github.com/jordigilh/kubernaut/pkg/apifrontend/prometheus"
)

type mockAnthropicMessager struct {
	resp *anthropic.Message
	err  error
}

func (m *mockAnthropicMessager) Create(ctx context.Context, params anthropic.MessageNewParams) (*anthropic.Message, error) {
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

// BR-AI-1404 / FedRAMP SI-4: Severity classification correctness
func TestAnthropicTriager_ClassifyHappyPath(t *testing.T) {
	mock := &mockAnthropicMessager{resp: makeAnthropicResponse("critical")}
	triager := NewAnthropicTriager(AnthropicTriagerConfig{
		Messager: mock,
		Model:    "claude-sonnet-4-6",
	})

	result, err := triager.TriagePure(context.Background(), TriageInput{Description: "HighCPU pod restart loop"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Severity != "critical" {
		t.Errorf("expected severity 'critical', got %q", result.Severity)
	}
	if result.Confidence != 1.0 {
		t.Errorf("expected confidence 1.0, got %v", result.Confidence)
	}
}

// BR-AI-1404 / FedRAMP SI-4: Error propagation for audit trail
func TestAnthropicTriager_ClassifyErrorPath(t *testing.T) {
	mock := &mockAnthropicMessager{err: errors.New("vertex auth failure")}
	triager := NewAnthropicTriager(AnthropicTriagerConfig{
		Messager: mock,
		Model:    "claude-sonnet-4-6",
	})

	_, err := triager.TriagePure(context.Background(), TriageInput{Description: "HighCPU"})
	if err == nil {
		t.Fatal("expected error from failed Anthropic call")
	}
	if got := err.Error(); got == "" {
		t.Error("expected non-empty error message")
	}
}

// BR-AI-1404 / FedRAMP SI-4: Degraded confidence for ambiguous LLM output
func TestAnthropicTriager_AmbiguousResponse(t *testing.T) {
	mock := &mockAnthropicMessager{resp: makeAnthropicResponse("I think it might be medium to high")}
	triager := NewAnthropicTriager(AnthropicTriagerConfig{
		Messager: mock,
		Model:    "claude-sonnet-4-6",
	})

	result, err := triager.TriagePure(context.Background(), TriageInput{Description: "Some alert"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Confidence >= 1.0 {
		t.Errorf("expected degraded confidence for ambiguous response, got %v", result.Confidence)
	}
}

// BR-AI-1404: TriageWithRules includes rule context in classification
func TestAnthropicTriager_WithRules(t *testing.T) {
	mock := &mockAnthropicMessager{resp: makeAnthropicResponse("high")}
	triager := NewAnthropicTriager(AnthropicTriagerConfig{
		Messager: mock,
		Model:    "claude-sonnet-4-6",
	})

	rules := []prom.Rule{{Name: "HighCPU", Query: `rate(cpu[5m]) > 0.9`}}
	result, err := triager.TriageWithRules(context.Background(), rules, TriageInput{Description: "CPU spike"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Severity != "high" {
		t.Errorf("expected severity 'high', got %q", result.Severity)
	}
}

// BR-AI-1404: Empty response handling
func TestAnthropicTriager_EmptyResponse(t *testing.T) {
	mock := &mockAnthropicMessager{resp: &anthropic.Message{Content: []anthropic.ContentBlockUnion{}}}
	triager := NewAnthropicTriager(AnthropicTriagerConfig{
		Messager: mock,
		Model:    "claude-sonnet-4-6",
	})

	_, err := triager.TriagePure(context.Background(), TriageInput{Description: "Something"})
	if err == nil {
		t.Fatal("expected error for empty response")
	}
}

// BR-AI-1404: Model family detection for factory routing
func TestIsAnthropicModel(t *testing.T) {
	tests := []struct {
		model string
		want  bool
	}{
		{"claude-sonnet-4-6", true},
		{"claude-3-5-sonnet-20241022", true},
		{"claude-3-haiku-20240307", true},
		{"gemini-2.0-flash", false},
		{"gemini-1.5-pro", false},
		{"gpt-4", false},
		{"", false},
	}
	for _, tc := range tests {
		if got := IsAnthropicModel(tc.model); got != tc.want {
			t.Errorf("IsAnthropicModel(%q) = %v, want %v", tc.model, got, tc.want)
		}
	}
}
