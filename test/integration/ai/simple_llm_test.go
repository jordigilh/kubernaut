/*
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

//go:build integration
// +build integration

package ai

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/ai/llm"
	"github.com/sirupsen/logrus"
)

// TestRealLLMConnectivity tests the real LLM service at the configured endpoint
func TestRealLLMConnectivity(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)

	// Configure for the real LLM service using environment variables
	endpoint := os.Getenv("LLM_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://192.168.1.169:8080" // Default to ramalama endpoint
	}

	model := os.Getenv("LLM_MODEL")
	if model == "" {
		model = "ggml-org/gpt-oss-20b-GGUF" // Default to 20B model
	}

	provider := os.Getenv("LLM_PROVIDER")
	if provider == "" {
		provider = "ramalama" // Default to ramalama provider
	}

	cfg := config.LLMConfig{
		Provider: provider,
		Model:    model,
		Endpoint: endpoint,
		Timeout:  30 * time.Second,
	}

	client, err := llm.NewClient(cfg, logger)
	if err != nil {
		t.Fatalf("Failed to create LLM client: %v", err)
	}

	// Test basic connectivity
	testPrompt := "Respond with 'CONNECTIVITY_OK' to confirm the LLM is working."
	response, err := client.GenerateResponse(testPrompt)
	if err != nil {
		t.Fatalf("Failed to get response from LLM: %v", err)
	}

	if response == "" {
		t.Fatal("Received empty response from LLM")
	}

	t.Logf("✅ Real LLM connectivity test successful!")
	t.Logf("📝 Prompt: %s", testPrompt)
	t.Logf("🤖 Response: %s", response)

	// Test with a more complex prompt
	alertPrompt := "Analyze this Kubernetes alert: HighMemoryUsage in production namespace. Recommend an action."
	alertResponse, err := client.GenerateResponse(alertPrompt)
	if err != nil {
		t.Fatalf("Failed to get alert analysis from LLM: %v", err)
	}

	if alertResponse == "" {
		t.Fatal("Received empty alert analysis from LLM")
	}

	t.Logf("✅ Real LLM alert analysis test successful!")
	t.Logf("📝 Alert Prompt: %s", alertPrompt)
	t.Logf("🤖 Alert Analysis: %s", alertResponse)

	// Test context-aware functionality
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := client.ReadinessCheck(ctx); err != nil {
		t.Fatalf("LLM readiness check failed: %v", err)
	}

	if err := client.LivenessCheck(ctx); err != nil {
		t.Fatalf("LLM liveness check failed: %v", err)
	}

	t.Logf("✅ Real LLM health checks successful!")
	t.Logf("🔧 Endpoint: %s", client.GetEndpoint())
	t.Logf("🧠 Model: %s", client.GetModel())
}
