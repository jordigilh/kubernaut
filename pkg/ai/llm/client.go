package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/infrastructure/metrics"
	"github.com/jordigilh/kubernaut/pkg/infrastructure/types"
	sharedhttp "github.com/jordigilh/kubernaut/pkg/shared/http"
	"github.com/sirupsen/logrus"
)

const (
	// promptTemplate is the template for alert analysis
	promptTemplate = `<|system|>
You are a Kubernetes operations expert. Analyze alerts and recommend automated remediation actions. Always respond with valid JSON only.
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

## CRITICAL DECISION RULES (check in order):

### 1. TERMINATION SIGNALS (highest priority):
- **"OOMKilled"** in labels/annotations → **increase_resources** (NEVER scale_deployment)
- **"CrashLoopBackOff"** → restart_pod OR rollback_deployment
- **"ImagePullBackOff"** → collect_diagnostics
- **"Evicted"** → migrate_workload OR drain_node

### 2. ERROR MESSAGE PATTERNS:
- **"memory limit exceeded"** → increase_resources
- **"disk space"** + percentage → cleanup_storage OR expand_pvc
- **"connection refused/timeout"** → restart_network OR update_network_policy
- **"permission denied"** → audit_logs OR rotate_secrets

### 3. RESOURCE SCOPE (check resource field and impact):
- **Resource is POD name** → increase_resources, restart_pod
- **Resource is NODE name** → drain_node, collect_diagnostics, notify_only
- **Multiple pods affected** → scale_deployment, migrate_workload
- **Cluster-wide** → collect_diagnostics, notify_only

### 4. NODE-LEVEL INDICATORS (override pod actions):
- **"kubelet"** in description → drain_node, collect_diagnostics
- **"network_reachable: false"** → drain_node, collect_diagnostics
- **"node_level_action_required"** → drain_node, collect_diagnostics
- **Resource field contains "node"** → drain_node, collect_diagnostics

## AVAILABLE ACTIONS:
- **scale_deployment**: Scale deployment replicas up/down
- **restart_pod**: Restart affected pod(s)
- **increase_resources**: Increase CPU/memory limits
- **notify_only**: No action, notify operators
- **rollback_deployment**: Rollback to previous revision
- **expand_pvc**: Expand persistent volume claim
- **drain_node**: Safely drain node for maintenance
- **quarantine_pod**: Isolate pod with network policies
- **collect_diagnostics**: Gather diagnostic information
- **cleanup_storage**: Clean up old data/logs
- **backup_data**: Trigger emergency backups
- **compact_storage**: Trigger storage compaction
- **update_network_policy**: Modify network policies
- **restart_network**: Restart network components
- **rotate_secrets**: Rotate compromised credentials
- **audit_logs**: Trigger security audit collection

## RESPONSE FORMAT (JSON only):
{
  "action": "action_name",
  "parameters": {
    "cpu_limit": "500m",
    "memory_limit": "1Gi",
    "replicas": 3
  },
  "confidence": 0.85,
  "reasoning": "Brief explanation of why this action was chosen"
}
<|assistant|>`
)

type Client interface {
	AnalyzeAlert(ctx context.Context, alert types.Alert) (*types.ActionRecommendation, error)
	ChatCompletion(ctx context.Context, prompt string) (string, error)
	IsHealthy() bool
}

type client struct {
	config     config.LLMConfig
	httpClient *http.Client
	log        *logrus.Logger
}

// OpenAI-compatible API structures (used by ramalama, LocalAI)
type OpenAIRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float32   `json:"temperature"`
	MaxTokens   int       `json:"max_tokens"`
	Stream      bool      `json:"stream"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type OpenAIResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
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

// Ollama API structures
type OllamaRequest struct {
	Model       string         `json:"model"`
	Prompt      string         `json:"prompt"`
	Stream      bool           `json:"stream"`
	Temperature float32        `json:"temperature,omitempty"`
	Options     *OllamaOptions `json:"options,omitempty"`
}

type OllamaOptions struct {
	Temperature float32 `json:"temperature,omitempty"`
	NumPredict  int     `json:"num_predict,omitempty"` // Max tokens in Ollama
}

type OllamaResponse struct {
	Model              string `json:"model"`
	Response           string `json:"response"`
	Done               bool   `json:"done"`
	Context            []int  `json:"context,omitempty"`
	TotalDuration      int64  `json:"total_duration,omitempty"`
	LoadDuration       int64  `json:"load_duration,omitempty"`
	PromptEvalCount    int    `json:"prompt_eval_count,omitempty"`
	PromptEvalDuration int64  `json:"prompt_eval_duration,omitempty"`
	EvalCount          int    `json:"eval_count,omitempty"`
	EvalDuration       int64  `json:"eval_duration,omitempty"`
}

// APIType represents the type of API being used
type APIType int

const (
	APITypeOpenAI APIType = iota // OpenAI-compatible (ramalama, LocalAI)
	APITypeOllama                // Native Ollama API
)

// NewClient creates a new SLM client supporting multiple API types
func NewClient(cfg config.LLMConfig, log *logrus.Logger) (Client, error) {
	// Support multiple provider types
	supportedProviders := []string{"localai", "ramalama", "ollama"}
	validProvider := false
	for _, provider := range supportedProviders {
		if cfg.Provider == provider {
			validProvider = true
			break
		}
	}

	if !validProvider {
		return nil, fmt.Errorf("unsupported provider: %s, supported: %v", cfg.Provider, supportedProviders)
	}

	c := &client{
		config:     cfg,
		httpClient: sharedhttp.NewClient(sharedhttp.LLMClientConfig(cfg.Timeout)),
		log:        log,
	}

	log.WithFields(logrus.Fields{
		"provider": cfg.Provider,
		"endpoint": cfg.Endpoint,
		"model":    cfg.Model,
	}).Info("SLM client initialized")

	return c, nil
}

// getAPIType determines the API type based on provider configuration
func (c *client) getAPIType() APIType {
	switch c.config.Provider {
	case "ollama":
		return APITypeOllama
	case "localai", "ramalama":
		return APITypeOpenAI
	default:
		// Default to OpenAI for compatibility
		return APITypeOpenAI
	}
}

func (c *client) AnalyzeAlert(ctx context.Context, alert types.Alert) (*types.ActionRecommendation, error) {
	// Generate prompt for alert analysis
	prompt := c.generatePrompt(alert)

	// Record context size metrics
	metrics.RecordSLMContextSize(c.config.Provider, len(prompt))

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
				"error":   lastErr.Error(),
			}).Warn("Retrying SLM request")

			// Exponential backoff
			backoff := time.Duration(attempt) * time.Second
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
				// Continue with retry
			}
		}

		response, err := c.ChatCompletion(ctx, prompt)
		if err != nil {
			lastErr = err
			continue
		}

		recommendation, err := c.parseResponse(response)
		if err != nil {
			lastErr = err
			continue
		}

		// Success
		c.log.WithFields(logrus.Fields{
			"action":     recommendation.Action,
			"confidence": recommendation.Confidence,
		}).Info("Successfully analyzed alert")

		return recommendation, nil
	}

	return nil, fmt.Errorf("failed to analyze alert after %d attempts: %v", c.config.RetryCount+1, lastErr)
}

func (c *client) generatePrompt(alert types.Alert) string {
	return fmt.Sprintf(promptTemplate,
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

func (c *client) parseResponse(response string) (*types.ActionRecommendation, error) {
	var rawResponse struct {
		Action     string                 `json:"action"`
		Parameters map[string]interface{} `json:"parameters"`
		Confidence float64                `json:"confidence"`
		Reasoning  string                 `json:"reasoning"`
	}

	if err := json.Unmarshal([]byte(response), &rawResponse); err != nil {
		return nil, fmt.Errorf("failed to parse response JSON: %w", err)
	}

	// Validate required fields
	if rawResponse.Action == "" {
		return nil, fmt.Errorf("missing required field: action")
	}

	// Validate confidence is within acceptable range
	if rawResponse.Confidence < 0.0 || rawResponse.Confidence > 1.0 {
		c.log.WithField("confidence", rawResponse.Confidence).Warn("Invalid confidence value, clamping to [0,1]")
		if rawResponse.Confidence < 0.0 {
			rawResponse.Confidence = 0.0
		} else if rawResponse.Confidence > 1.0 {
			rawResponse.Confidence = 1.0
		}
	}

	recommendation := &types.ActionRecommendation{
		Action:     rawResponse.Action,
		Parameters: rawResponse.Parameters,
		Confidence: rawResponse.Confidence,
		Reasoning: &types.ReasoningDetails{
			Summary: rawResponse.Reasoning,
		},
	}

	return recommendation, nil
}

// ChatCompletion implements chat completion supporting multiple API types
func (c *client) ChatCompletion(ctx context.Context, prompt string) (string, error) {
	apiType := c.getAPIType()

	switch apiType {
	case APITypeOpenAI:
		return c.chatCompletionOpenAI(ctx, prompt)
	case APITypeOllama:
		return c.chatCompletionOllama(ctx, prompt)
	default:
		return "", fmt.Errorf("unsupported API type: %v", apiType)
	}
}

// chatCompletionOpenAI handles OpenAI-compatible API requests (ramalama, LocalAI)
func (c *client) chatCompletionOpenAI(ctx context.Context, prompt string) (string, error) {
	reqBody := OpenAIRequest{
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
		return "", fmt.Errorf("failed to marshal OpenAI request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.config.Endpoint+"/v1/chat/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create OpenAI request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to make OpenAI request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read OpenAI response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("openAI API returned non-200 status: %d, body: %s", resp.StatusCode, string(body))
	}

	var openAIResp OpenAIResponse
	if err := json.Unmarshal(body, &openAIResp); err != nil {
		return "", fmt.Errorf("failed to unmarshal OpenAI response: %w", err)
	}

	if len(openAIResp.Choices) == 0 {
		return "", fmt.Errorf("no choices in OpenAI response")
	}

	// Record token usage metrics
	if openAIResp.Usage.TotalTokens > 0 {
		tokenBytes := openAIResp.Usage.TotalTokens * 4
		metrics.RecordSLMContextSize(c.config.Provider, tokenBytes)
	}

	return openAIResp.Choices[0].Message.Content, nil
}

// chatCompletionOllama handles native Ollama API requests
func (c *client) chatCompletionOllama(ctx context.Context, prompt string) (string, error) {
	reqBody := OllamaRequest{
		Model:       c.config.Model,
		Prompt:      prompt,
		Stream:      false,
		Temperature: c.config.Temperature,
		Options: &OllamaOptions{
			Temperature: c.config.Temperature,
			NumPredict:  c.config.MaxTokens,
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal Ollama request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.config.Endpoint+"/api/generate", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create Ollama request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to make Ollama request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read Ollama response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Ollama API returned non-200 status: %d, body: %s", resp.StatusCode, string(body))
	}

	var ollamaResp OllamaResponse
	if err := json.Unmarshal(body, &ollamaResp); err != nil {
		return "", fmt.Errorf("failed to unmarshal Ollama response: %w", err)
	}

	if !ollamaResp.Done {
		return "", fmt.Errorf("ollama response not complete")
	}

	// Record token usage metrics (approximate from Ollama response)
	if ollamaResp.PromptEvalCount > 0 || ollamaResp.EvalCount > 0 {
		totalTokens := ollamaResp.PromptEvalCount + ollamaResp.EvalCount
		tokenBytes := totalTokens * 4
		metrics.RecordSLMContextSize(c.config.Provider, tokenBytes)
	}

	return ollamaResp.Response, nil
}

func (c *client) IsHealthy() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Simple health check by making a minimal request
	_, err := c.ChatCompletion(ctx, "Health check")
	return err == nil
}
