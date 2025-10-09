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

package testing

import (
	"net/http"
	"time"

	"github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/ai/llm"
	"github.com/sirupsen/logrus"
)

// MockProvider implements the llm.Provider interface for testing purposes
// This provider is specifically designed for testing and should never be used in production
type MockProvider struct {
	config   config.LLMConfig
	logger   *logrus.Logger
	endpoint string
}

// NewMockProvider creates a new MockProvider instance for testing
func NewMockProvider() llm.Provider {
	return &MockProvider{}
}

func (p *MockProvider) Initialize(config config.LLMConfig, logger *logrus.Logger) error {
	p.config = config
	p.logger = logger
	p.endpoint = "http://localhost:8080" // Mock endpoint

	if logger != nil {
		logger.Info("Configured Mock provider for testing")
	}
	return nil
}

func (p *MockProvider) GetEndpoint() string {
	return p.endpoint
}

func (p *MockProvider) GetAPIKey() string {
	return "mock-api-key"
}

func (p *MockProvider) GetName() string {
	return "mock"
}

func (p *MockProvider) GetDefaultModel() string {
	return "mock-model"
}

func (p *MockProvider) SupportsChatCompletion() bool {
	return true
}

func (p *MockProvider) GetMaxTokens() int {
	return 131072
}

func (p *MockProvider) GetHeaders() map[string]string {
	return map[string]string{
		"Content-Type":    "application/json",
		"X-Mock-Provider": "true",
	}
}

func (p *MockProvider) ValidateConfig() error {
	return nil // Mock provider always validates successfully
}

func (p *MockProvider) CreateHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 60 * time.Second,
	}
}

