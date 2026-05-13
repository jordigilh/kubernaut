//go:build llm_integration

package severity

import (
	"context"
	"os"
	"testing"
	"time"

	"google.golang.org/genai"
)

func TestLLMIntegration_RealClassification(t *testing.T) {
	project := os.Getenv("LLM_PROJECT")
	if project == "" {
		t.Skip("LLM_PROJECT not set — skipping integration test")
	}
	region := os.Getenv("LLM_REGION")
	if region == "" {
		region = "us-central1"
	}
	model := os.Getenv("LLM_MODEL")
	if model == "" {
		model = "gemini-2.0-flash"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		Project:  project,
		Location: region,
	})
	if err != nil {
		t.Fatalf("genai.NewClient: %v", err)
	}

	triager := NewGenAITriager(GenAITriagerConfig{Client: client, Model: model})

	input := TriageInput{
		Namespace:   "production",
		Kind:        "Pod",
		Name:        "web-frontend-abc123",
		Description: "CrashLoopBackOff: container exits with OOMKilled status every 30 seconds",
	}

	result, err := triager.TriagePure(ctx, input)
	if err != nil {
		t.Fatalf("TriagePure: %v", err)
	}

	validSeverities := map[string]bool{"critical": true, "high": true, "medium": true, "low": true}
	if !validSeverities[result.Severity] {
		t.Errorf("unexpected severity %q (not in valid set)", result.Severity)
	}
	if result.Confidence <= 0 || result.Confidence > 1.0 {
		t.Errorf("confidence %v out of [0,1] range", result.Confidence)
	}
	t.Logf("LLM classified as %q with confidence %.2f", result.Severity, result.Confidence)
}
