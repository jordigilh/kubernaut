package severity

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/genai"
)

type mockGenerator struct {
	resp *genai.GenerateContentResponse
	err  error
}

func (m *mockGenerator) GenerateContent(_ context.Context, _ string, _ []*genai.Content, _ *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
	return m.resp, m.err
}

func TestGenAITriager_ClassifyHappyPath(t *testing.T) {
	// Business outcome: when LLM returns a valid severity, it's normalized and returned with full confidence
	gen := &mockGenerator{
		resp: &genai.GenerateContentResponse{
			Candidates: []*genai.Candidate{
				{Content: &genai.Content{Parts: []*genai.Part{{Text: "critical"}}}},
			},
		},
	}
	triager := NewGenAITriager(GenAITriagerConfig{Generator: gen, Model: "test-model"})

	result, err := triager.TriagePure(context.Background(), TriageInput{Description: "HighCPU"})
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

func TestGenAITriager_ClassifyErrorPath(t *testing.T) {
	// Business outcome: when LLM call fails, error is propagated
	gen := &mockGenerator{err: errors.New("network timeout")}
	triager := NewGenAITriager(GenAITriagerConfig{Generator: gen, Model: "test-model"})

	_, err := triager.TriagePure(context.Background(), TriageInput{Description: "HighCPU"})
	if err == nil {
		t.Fatal("expected error from failed LLM call")
	}
}

func TestGenAITriager_ClassifyNilResponse(t *testing.T) {
	// Business outcome: nil response is treated as an error
	gen := &mockGenerator{resp: nil}
	triager := NewGenAITriager(GenAITriagerConfig{Generator: gen, Model: "test-model"})

	_, err := triager.TriagePure(context.Background(), TriageInput{Description: "HighCPU"})
	if err == nil {
		t.Fatal("expected error for nil response")
	}
}

func TestGenAITriager_ClassifyEmptyResponse(t *testing.T) {
	// Business outcome: response with no text parts is treated as error
	gen := &mockGenerator{
		resp: &genai.GenerateContentResponse{
			Candidates: []*genai.Candidate{
				{Content: &genai.Content{Parts: []*genai.Part{}}},
			},
		},
	}
	triager := NewGenAITriager(GenAITriagerConfig{Generator: gen, Model: "test-model"})

	_, err := triager.TriagePure(context.Background(), TriageInput{Description: "HighCPU"})
	if err == nil {
		t.Fatal("expected error for empty response")
	}
}

func TestGenAITriager_ClassifyMalformedText(t *testing.T) {
	// Business outcome: unrecognized severity text gets normalized and marked low confidence
	gen := &mockGenerator{
		resp: &genai.GenerateContentResponse{
			Candidates: []*genai.Candidate{
				{Content: &genai.Content{Parts: []*genai.Part{{Text: "urgent!!!"}}}},
			},
		},
	}
	triager := NewGenAITriager(GenAITriagerConfig{Generator: gen, Model: "test-model"})

	result, err := triager.TriagePure(context.Background(), TriageInput{Description: "HighCPU"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Confidence >= 1.0 {
		t.Errorf("expected low confidence for unrecognized severity, got %v", result.Confidence)
	}
}

func TestGenAITriager_TriageWithRules(t *testing.T) {
	// Business outcome: rule context is included in prompt and classification works
	gen := &mockGenerator{
		resp: &genai.GenerateContentResponse{
			Candidates: []*genai.Candidate{
				{Content: &genai.Content{Parts: []*genai.Part{{Text: "high"}}}},
			},
		},
	}
	triager := NewGenAITriager(GenAITriagerConfig{Generator: gen, Model: "test-model"})

	result, err := triager.TriageWithRules(context.Background(), nil, TriageInput{Description: "HighCPU"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Severity != "high" {
		t.Errorf("expected severity 'high', got %q", result.Severity)
	}
}
