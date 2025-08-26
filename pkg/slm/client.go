package slm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jordigilh/prometheus-alerts-slm/internal/config"
	"github.com/sirupsen/logrus"
)

type Client interface {
	AnalyzeAlert(ctx context.Context, alert Alert) (*ActionRecommendation, error)
	IsHealthy() bool
}

type client struct {
	config     config.SLMConfig
	httpClient *http.Client
	log        *logrus.Logger
}

type Alert struct {
	Name        string            `json:"name"`
	Status      string            `json:"status"`
	Severity    string            `json:"severity"`
	Description string            `json:"description"`
	Namespace   string            `json:"namespace"`
	Resource    string            `json:"resource"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	StartsAt    time.Time         `json:"starts_at"`
	EndsAt      *time.Time        `json:"ends_at,omitempty"`
}

type ActionRecommendation struct {
	Action     string                 `json:"action"`
	Parameters map[string]interface{} `json:"parameters"`
	Confidence float64                `json:"confidence,omitempty"`
	Reasoning  string                 `json:"reasoning,omitempty"`
}

// LocalAI Chat Completion API structures
type LocalAIRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float32   `json:"temperature,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Stream      bool      `json:"stream"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type LocalAIResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage,omitempty"`
}

type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

func NewClient(cfg config.SLMConfig, log *logrus.Logger) (Client, error) {
	if cfg.Provider != "localai" {
		return nil, fmt.Errorf("only LocalAI provider supported, got: %s", cfg.Provider)
	}

	c := &client{
		config: cfg,
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
		log: log,
	}

	log.WithFields(logrus.Fields{
		"provider": "LocalAI",
		"endpoint": cfg.Endpoint,
		"model":    cfg.Model,
	}).Info("SLM client initialized with LocalAI")

	return c, nil
}

func (c *client) AnalyzeAlert(ctx context.Context, alert Alert) (*ActionRecommendation, error) {
	prompt := c.generatePrompt(alert)
	
	c.log.WithFields(logrus.Fields{
		"alert":     alert.Name,
		"namespace": alert.Namespace,
		"severity":  alert.Severity,
		"provider":  c.config.Provider,
	}).Debug("Analyzing alert with SLM")

	var lastErr error
	for attempt := 0; attempt <= c.config.RetryCount; attempt++ {
		if attempt > 0 {
			c.log.WithFields(logrus.Fields{
				"attempt": attempt,
				"alert":   alert.Name,
			}).Warn("Retrying LocalAI request")
			
			// Exponential backoff
			backoff := time.Duration(attempt) * time.Second
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}

		recommendation, err := c.makeLocalAIRequest(ctx, prompt)
		if err != nil {
			lastErr = err
			continue
		}

		c.log.WithFields(logrus.Fields{
			"alert":  alert.Name,
			"action": recommendation.Action,
		}).Info("LocalAI analysis completed")

		return recommendation, nil
	}

	return nil, fmt.Errorf("failed to analyze alert after %d attempts: %w", c.config.RetryCount+1, lastErr)
}

func (c *client) makeLocalAIRequest(ctx context.Context, prompt string) (*ActionRecommendation, error) {
	reqBody := LocalAIRequest{
		Model: c.config.Model,
		Messages: []Message{
			{
				Role:    "user",
				Content: prompt,
			},
		},
		Temperature: c.config.Temperature,
		MaxTokens:   c.config.MaxTokens,
		Stream:      false,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal LocalAI request: %w", err)
	}

	// LocalAI chat completions endpoint
	endpoint := fmt.Sprintf("%s/v1/chat/completions", strings.TrimSuffix(c.config.Endpoint, "/"))
	
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create LocalAI request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	// LocalAI typically doesn't require authentication, but add if configured
	if c.config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	}

	c.log.WithFields(logrus.Fields{
		"endpoint": endpoint,
		"model":    c.config.Model,
	}).Debug("Making LocalAI request")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make LocalAI request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("LocalAI API returned status %d: %s", resp.StatusCode, string(body))
	}

	var localAIResp LocalAIResponse
	if err := json.NewDecoder(resp.Body).Decode(&localAIResp); err != nil {
		return nil, fmt.Errorf("failed to decode LocalAI response: %w", err)
	}

	if len(localAIResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in LocalAI response")
	}

	// Parse the action recommendation from the response content
	content := localAIResp.Choices[0].Message.Content
	recommendation, err := c.parseActionRecommendation(content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse action recommendation: %w", err)
	}

	// Log usage statistics if available
	if localAIResp.Usage.TotalTokens > 0 {
		c.log.WithFields(logrus.Fields{
			"prompt_tokens":     localAIResp.Usage.PromptTokens,
			"completion_tokens": localAIResp.Usage.CompletionTokens,
			"total_tokens":      localAIResp.Usage.TotalTokens,
		}).Debug("LocalAI token usage")
	}

	return recommendation, nil
}

func (c *client) generatePrompt(alert Alert) string {
	template := `<|system|>
You are a Kubernetes operations expert specialized in analyzing alerts and recommending automated remediation actions. Always respond with valid JSON only.
<|user|>
Analyze this Kubernetes alert and recommend an action:

Alert: %s
Status: %s
Severity: %s
Description: %s
Namespace: %s
Resource: %s
Labels: %v
Annotations: %v

Available actions:
- scale_deployment: Scale deployment replicas up or down
- restart_pod: Restart the affected pod(s)
- increase_resources: Increase CPU/memory limits
- notify_only: No automated action, notify operators only

Guidelines:
- For high memory/CPU usage: consider scale_deployment or increase_resources
- For pod crashes/failures: consider restart_pod
- For critical alerts in production: prefer notify_only unless certain
- Include confidence score (0.0-1.0) and reasoning

Respond with valid JSON in this exact format:
{
  "action": "one_of_the_available_actions",
  "parameters": {
    "replicas": 3,
    "cpu_limit": "500m",
    "memory_limit": "1Gi"
  },
  "confidence": 0.85,
  "reasoning": "Brief explanation of why this action was chosen"
}
<|assistant|>`

	return fmt.Sprintf(template,
		alert.Name,
		alert.Status,
		alert.Severity,
		alert.Description,
		alert.Namespace,
		alert.Resource,
		alert.Labels,
		alert.Annotations,
	)
}

func (c *client) parseActionRecommendation(content string) (*ActionRecommendation, error) {
	// Try to extract JSON from the content
	start := -1
	end := -1
	
	for i, char := range content {
		if char == '{' && start == -1 {
			start = i
		}
		if char == '}' {
			end = i + 1
		}
	}
	
	if start == -1 || end == -1 {
		return nil, fmt.Errorf("no JSON found in response content: %s", content)
	}
	
	jsonStr := content[start:end]
	
	var recommendation ActionRecommendation
	if err := json.Unmarshal([]byte(jsonStr), &recommendation); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %w", err)
	}
	
	// Validate action
	validActions := map[string]bool{
		"scale_deployment":   true,
		"restart_pod":        true,
		"increase_resources": true,
		"notify_only":        true,
	}
	
	if !validActions[recommendation.Action] {
		return nil, fmt.Errorf("invalid action: %s", recommendation.Action)
	}
	
	return &recommendation, nil
}

// Mock functionality removed - only LocalAI supported

func (c *client) IsHealthy() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Use Ollama's API tags endpoint for health check
	healthEndpoint := fmt.Sprintf("%s/api/tags", strings.TrimSuffix(c.config.Endpoint, "/"))
	
	req, err := http.NewRequestWithContext(ctx, "GET", healthEndpoint, nil)
	if err != nil {
		return false
	}

	// Ollama doesn't require authentication for API endpoints
	if c.config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}