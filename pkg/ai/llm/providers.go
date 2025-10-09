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

package llm

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/jordigilh/kubernaut/internal/config"
	"github.com/sirupsen/logrus"
)

// OpenAIProvider implements the Provider interface for OpenAI
type OpenAIProvider struct {
	config   config.LLMConfig
	logger   *logrus.Logger
	apiKey   string
	endpoint string
}

func (p *OpenAIProvider) Initialize(config config.LLMConfig, logger *logrus.Logger) error {
	p.config = config
	p.logger = logger
	p.endpoint = "https://api.openai.com/v1"

	p.apiKey = os.Getenv("OPENAI_API_KEY")
	if p.apiKey == "" {
		return fmt.Errorf("OpenAI API key required for enterprise 20B+ model deployment")
	}

	logger.Info("Configured OpenAI provider for enterprise 20B+ model")
	return nil
}

func (p *OpenAIProvider) GetEndpoint() string          { return p.endpoint }
func (p *OpenAIProvider) GetAPIKey() string            { return p.apiKey }
func (p *OpenAIProvider) GetName() string              { return "openai" }
func (p *OpenAIProvider) GetDefaultModel() string      { return "gpt-4" }
func (p *OpenAIProvider) SupportsChatCompletion() bool { return true }
func (p *OpenAIProvider) GetMaxTokens() int            { return 131072 }

func (p *OpenAIProvider) GetHeaders() map[string]string {
	return map[string]string{
		"Authorization": "Bearer " + p.apiKey,
		"Content-Type":  "application/json",
	}
}

func (p *OpenAIProvider) ValidateConfig() error {
	if p.apiKey == "" {
		return fmt.Errorf("OpenAI API key is required")
	}
	return nil
}

func (p *OpenAIProvider) CreateHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 60 * time.Second,
	}
}

// HuggingFaceProvider implements the Provider interface for HuggingFace
type HuggingFaceProvider struct {
	config   config.LLMConfig
	logger   *logrus.Logger
	apiKey   string
	endpoint string
}

func (p *HuggingFaceProvider) Initialize(config config.LLMConfig, logger *logrus.Logger) error {
	p.config = config
	p.logger = logger
	p.endpoint = "https://api-inference.huggingface.co/models"

	p.apiKey = os.Getenv("HUGGINGFACE_API_KEY")
	if p.apiKey == "" {
		return fmt.Errorf("HuggingFace API key required for enterprise 20B+ model deployment")
	}

	logger.Info("Configured HuggingFace provider for enterprise 20B+ model")
	return nil
}

func (p *HuggingFaceProvider) GetEndpoint() string          { return p.endpoint }
func (p *HuggingFaceProvider) GetAPIKey() string            { return p.apiKey }
func (p *HuggingFaceProvider) GetName() string              { return "huggingface" }
func (p *HuggingFaceProvider) GetDefaultModel() string      { return "microsoft/DialoGPT-large" }
func (p *HuggingFaceProvider) SupportsChatCompletion() bool { return true }
func (p *HuggingFaceProvider) GetMaxTokens() int            { return 131072 }

func (p *HuggingFaceProvider) GetHeaders() map[string]string {
	return map[string]string{
		"Authorization": "Bearer " + p.apiKey,
		"Content-Type":  "application/json",
	}
}

func (p *HuggingFaceProvider) ValidateConfig() error {
	if p.apiKey == "" {
		return fmt.Errorf("HuggingFace API key is required")
	}
	return nil
}

func (p *HuggingFaceProvider) CreateHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 60 * time.Second,
	}
}

// OllamaProvider implements the Provider interface for Ollama
type OllamaProvider struct {
	config   config.LLMConfig
	logger   *logrus.Logger
	endpoint string
}

func (p *OllamaProvider) Initialize(config config.LLMConfig, logger *logrus.Logger) error {
	p.config = config
	p.logger = logger

	// Priority: 1) Config Endpoint, 2) LLM_ENDPOINT env var, 3) Default
	p.endpoint = config.Endpoint
	if p.endpoint == "" {
		p.endpoint = os.Getenv("LLM_ENDPOINT")
	}
	if p.endpoint == "" {
		p.endpoint = "http://localhost:11434"
	}

	logger.WithField("endpoint", p.endpoint).Info("Configured Ollama provider for enterprise 20B+ model")
	return nil
}

func (p *OllamaProvider) GetEndpoint() string          { return p.endpoint }
func (p *OllamaProvider) GetAPIKey() string            { return "" } // No API key needed
func (p *OllamaProvider) GetName() string              { return "ollama" }
func (p *OllamaProvider) GetDefaultModel() string      { return "ggml-org/gpt-oss-20b-GGUF" }
func (p *OllamaProvider) SupportsChatCompletion() bool { return true }
func (p *OllamaProvider) GetMaxTokens() int            { return 131072 }

func (p *OllamaProvider) GetHeaders() map[string]string {
	return map[string]string{
		"Content-Type": "application/json",
	}
}

func (p *OllamaProvider) ValidateConfig() error {
	if p.endpoint == "" {
		return fmt.Errorf("ollama endpoint is required")
	}
	return nil
}

func (p *OllamaProvider) CreateHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 60 * time.Second,
	}
}

// RamallamaProvider implements the Provider interface for Ramalama
type RamallamaProvider struct {
	config   config.LLMConfig
	logger   *logrus.Logger
	endpoint string
}

func (p *RamallamaProvider) Initialize(config config.LLMConfig, logger *logrus.Logger) error {
	p.config = config
	p.logger = logger

	// Priority: 1) Config Endpoint, 2) LLM_ENDPOINT env var, 3) Default ramalama endpoint
	p.endpoint = config.Endpoint
	if p.endpoint == "" {
		p.endpoint = os.Getenv("LLM_ENDPOINT")
	}
	if p.endpoint == "" {
		p.endpoint = "http://localhost:8080"
	}

	logger.WithField("endpoint", p.endpoint).Info("Configured Ramalama provider for enterprise 20B+ model")
	return nil
}

func (p *RamallamaProvider) GetEndpoint() string          { return p.endpoint }
func (p *RamallamaProvider) GetAPIKey() string            { return "" } // No API key needed
func (p *RamallamaProvider) GetName() string              { return "ramalama" }
func (p *RamallamaProvider) GetDefaultModel() string      { return "ggml-org/gpt-oss-20b-GGUF" }
func (p *RamallamaProvider) SupportsChatCompletion() bool { return true }
func (p *RamallamaProvider) GetMaxTokens() int            { return 131072 }

func (p *RamallamaProvider) GetHeaders() map[string]string {
	return map[string]string{
		"Content-Type": "application/json",
	}
}

func (p *RamallamaProvider) ValidateConfig() error {
	if p.endpoint == "" {
		return fmt.Errorf("ramalama endpoint is required")
	}
	return nil
}

func (p *RamallamaProvider) CreateHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 60 * time.Second,
	}
}

// MockProvider has been moved to pkg/ai/llm/testing/mock_provider.go
// This ensures clear separation between production business logic and testing infrastructure
