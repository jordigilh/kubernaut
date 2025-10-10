<<<<<<< HEAD
=======
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

>>>>>>> crd_implementation
package hybrid

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/ai/llm"
	"github.com/jordigilh/kubernaut/pkg/testutil/mocks"
)

// CreateLLMClient creates the appropriate LLM client based on environment
// Returns real LLM if available, mock otherwise. Simple and clean.
func CreateLLMClient(logger *logrus.Logger) llm.Client {
	if logger == nil {
		logger = logrus.New()
		logger.SetLevel(logrus.WarnLevel)
	}

	// Check if we should use real LLM
	if shouldUseRealLLM() && isLLMAvailable() {
		if client := createRealLLMClient(logger); client != nil {
			logger.Info("Using real LLM client for testing")
			return client
		}
	}

	// Fallback to mock
	logger.Info("Using mock LLM client for testing")
	return mocks.NewMockLLMClient()
}

// shouldUseRealLLM determines if we should attempt real LLM
func shouldUseRealLLM() bool {
	// Check CI environment
	if isCIEnvironment() {
		return false
	}

	// Check explicit disable
	if os.Getenv("USE_REAL_LLM") == "false" {
		return false
	}

	return true
}

// isCIEnvironment detects CI/CD environments
func isCIEnvironment() bool {
	ciVars := []string{"CI", "GITHUB_ACTIONS", "GITLAB_CI", "JENKINS_URL", "BUILDKITE"}
	for _, envVar := range ciVars {
		if os.Getenv(envVar) != "" {
			return true
		}
	}
	return false
}

// isLLMAvailable checks if real LLM endpoint is reachable
func isLLMAvailable() bool {
	endpoint := os.Getenv("LLM_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://192.168.1.169:8080"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	client := &http.Client{Timeout: 2 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint+"/health", nil)
	if err != nil {
		return false
	}

	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			// Log error but don't fail the operation
			log.Printf("Failed to close response body: %v", err)
		}
	}()

	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

// createRealLLMClient attempts to create a real LLM client
func createRealLLMClient(logger *logrus.Logger) llm.Client {
	config := config.LLMConfig{
		Provider:    "ramalama",
		Model:       "ggml-org/gpt-oss-20b-GGUF",
		Temperature: 0.1,
		Timeout:     30 * time.Second,
	}

	client, err := llm.NewClient(config, logger)
	if err != nil {
		logger.WithError(err).Debug("Failed to create real LLM client")
		return nil
	}

	if !client.IsHealthy() {
		logger.Debug("Real LLM client failed health check")
		return nil
	}

	return client
}
