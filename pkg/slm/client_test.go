package slm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jordigilh/prometheus-alerts-slm/internal/config"
	"github.com/jordigilh/prometheus-alerts-slm/pkg/types"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel) // Suppress logs during tests

	tests := []struct {
		name      string
		config    config.SLMConfig
		expectErr bool
		errString string
	}{
		{
			name: "valid localai config",
			config: config.SLMConfig{
				Provider: "localai",
				Endpoint: "http://localhost:8080",
				Model:    "test-model",
				Timeout:  30 * time.Second,
			},
			expectErr: false,
		},
		{
			name: "invalid provider",
			config: config.SLMConfig{
				Provider: "invalid",
				Endpoint: "http://localhost:8080",
				Model:    "test-model",
			},
			expectErr: true,
			errString: "only LocalAI provider supported, got: invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(tt.config, logger)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errString)
				assert.Nil(t, client)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, client)
				assert.Implements(t, (*Client)(nil), client)
			}
		})
	}
}

func TestClient_AnalyzeAlert(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	// Mock server that responds with valid LocalAI response
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/v1/chat/completions", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		// Validate request body
		var reqBody LocalAIRequest
		err := json.NewDecoder(r.Body).Decode(&reqBody)
		assert.NoError(t, err)
		assert.Equal(t, "test-model", reqBody.Model)
		assert.False(t, reqBody.Stream)
		assert.Contains(t, reqBody.Messages[0].Content, "TestAlert")

		// Return valid response
		response := LocalAIResponse{
			ID:      "test-id",
			Object:  "chat.completion",
			Created: time.Now().Unix(),
			Model:   "test-model",
			Choices: []Choice{
				{
					Index: 0,
					Message: Message{
						Role: "assistant",
						Content: `{
							"action": "scale_deployment",
							"parameters": {
								"replicas": 3
							},
							"confidence": 0.85,
							"reasoning": "High CPU usage indicates need for scaling"
						}`,
					},
					FinishReason: "stop",
				},
			},
			Usage: Usage{
				PromptTokens:     100,
				CompletionTokens: 50,
				TotalTokens:      150,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer mockServer.Close()

	cfg := config.SLMConfig{
		Provider:    "localai",
		Endpoint:    mockServer.URL,
		Model:       "test-model",
		Temperature: 0.3,
		MaxTokens:   500,
		Timeout:     10 * time.Second,
		RetryCount:  2,
	}

	client, err := NewClient(cfg, logger)
	require.NoError(t, err)

	alert := types.Alert{
		Name:        "TestAlert",
		Status:      "firing",
		Severity:    "critical",
		Description: "Test alert description",
		Namespace:   "test-namespace",
		Resource:    "test-resource",
		Labels: map[string]string{
			"app": "test-app",
		},
		Annotations: map[string]string{
			"runbook": "test-runbook",
		},
	}

	ctx := context.Background()
	recommendation, err := client.AnalyzeAlert(ctx, alert)

	assert.NoError(t, err)
	assert.NotNil(t, recommendation)
	assert.Equal(t, "scale_deployment", recommendation.Action)
	assert.Equal(t, float64(0.85), recommendation.Confidence)
	assert.Equal(t, "High CPU usage indicates need for scaling", recommendation.Reasoning)
	assert.Contains(t, recommendation.Parameters, "replicas")
}

func TestClient_AnalyzeAlert_ServerError(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	// Mock server that returns 500 error
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal Server Error"))
	}))
	defer mockServer.Close()

	cfg := config.SLMConfig{
		Provider:   "localai",
		Endpoint:   mockServer.URL,
		Model:      "test-model",
		Timeout:    1 * time.Second,
		RetryCount: 1,
	}

	client, err := NewClient(cfg, logger)
	require.NoError(t, err)

	alert := types.Alert{
		Name:      "TestAlert",
		Status:    "firing",
		Severity:  "critical",
		Namespace: "test",
	}

	ctx := context.Background()
	recommendation, err := client.AnalyzeAlert(ctx, alert)

	assert.Error(t, err)
	assert.Nil(t, recommendation)
	assert.Contains(t, err.Error(), "failed to analyze alert after")
}

func TestClient_AnalyzeAlert_ContextCancellation(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	// Mock server with delay
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer mockServer.Close()

	cfg := config.SLMConfig{
		Provider:   "localai",
		Endpoint:   mockServer.URL,
		Model:      "test-model",
		Timeout:    5 * time.Second,
		RetryCount: 0,
	}

	client, err := NewClient(cfg, logger)
	require.NoError(t, err)

	alert := types.Alert{
		Name:      "TestAlert",
		Status:    "firing",
		Namespace: "test",
	}

	// Create context that cancels quickly
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	recommendation, err := client.AnalyzeAlert(ctx, alert)

	assert.Error(t, err)
	assert.Nil(t, recommendation)
}

func TestClient_AnalyzeAlert_InvalidJSON(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	// Mock server that returns invalid JSON
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := LocalAIResponse{
			Choices: []Choice{
				{
					Message: Message{
						Role:    "assistant",
						Content: "This is not valid JSON response",
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer mockServer.Close()

	cfg := config.SLMConfig{
		Provider:   "localai",
		Endpoint:   mockServer.URL,
		Model:      "test-model",
		Timeout:    5 * time.Second,
		RetryCount: 0,
	}

	client, err := NewClient(cfg, logger)
	require.NoError(t, err)

	alert := types.Alert{
		Name:      "TestAlert",
		Status:    "firing",
		Namespace: "test",
	}

	ctx := context.Background()
	recommendation, err := client.AnalyzeAlert(ctx, alert)

	assert.Error(t, err)
	assert.Nil(t, recommendation)
	assert.Contains(t, err.Error(), "failed to parse action recommendation")
}

func TestClient_AnalyzeAlert_InvalidAction(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	// Mock server that returns invalid action
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := LocalAIResponse{
			Choices: []Choice{
				{
					Message: Message{
						Role: "assistant",
						Content: `{
							"action": "invalid_action",
							"confidence": 0.85,
							"reasoning": "This is invalid"
						}`,
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer mockServer.Close()

	cfg := config.SLMConfig{
		Provider:   "localai",
		Endpoint:   mockServer.URL,
		Model:      "test-model",
		Timeout:    5 * time.Second,
		RetryCount: 0,
	}

	client, err := NewClient(cfg, logger)
	require.NoError(t, err)

	alert := types.Alert{
		Name:      "TestAlert",
		Status:    "firing",
		Namespace: "test",
	}

	ctx := context.Background()
	recommendation, err := client.AnalyzeAlert(ctx, alert)

	assert.Error(t, err)
	assert.Nil(t, recommendation)
	assert.Contains(t, err.Error(), "invalid action: invalid_action")
}

func TestClient_AnalyzeAlert_WithAPIKey(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	// Mock server that checks for authorization header
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		assert.Equal(t, "Bearer test-api-key", authHeader)

		response := LocalAIResponse{
			Choices: []Choice{
				{
					Message: Message{
						Role: "assistant",
						Content: `{
							"action": "notify_only",
							"confidence": 0.5,
							"reasoning": "Requires manual investigation"
						}`,
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer mockServer.Close()

	cfg := config.SLMConfig{
		Provider:   "localai",
		Endpoint:   mockServer.URL,
		Model:      "test-model",
		APIKey:     "test-api-key",
		Timeout:    5 * time.Second,
		RetryCount: 0,
	}

	client, err := NewClient(cfg, logger)
	require.NoError(t, err)

	alert := types.Alert{
		Name:      "TestAlert",
		Status:    "firing",
		Namespace: "test",
	}

	ctx := context.Background()
	recommendation, err := client.AnalyzeAlert(ctx, alert)

	assert.NoError(t, err)
	assert.NotNil(t, recommendation)
	assert.Equal(t, "notify_only", recommendation.Action)
}

func TestClient_IsHealthy(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	tests := []struct {
		name           string
		serverResponse func(w http.ResponseWriter, r *http.Request)
		expectedHealth bool
	}{
		{
			name: "healthy server",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "GET", r.Method)
				assert.Equal(t, "/api/tags", r.URL.Path)
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"models": []}`))
			},
			expectedHealth: true,
		},
		{
			name: "unhealthy server",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			expectedHealth: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockServer := httptest.NewServer(http.HandlerFunc(tt.serverResponse))
			defer mockServer.Close()

			cfg := config.SLMConfig{
				Provider: "localai",
				Endpoint: mockServer.URL,
				Model:    "test-model",
				Timeout:  5 * time.Second,
			}

			client, err := NewClient(cfg, logger)
			require.NoError(t, err)

			healthy := client.IsHealthy()
			assert.Equal(t, tt.expectedHealth, healthy)
		})
	}
}

func TestClient_IsHealthy_NetworkError(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	cfg := config.SLMConfig{
		Provider: "localai",
		Endpoint: "http://nonexistent:9999",
		Model:    "test-model",
		Timeout:  1 * time.Second,
	}

	client, err := NewClient(cfg, logger)
	require.NoError(t, err)

	healthy := client.IsHealthy()
	assert.False(t, healthy)
}

// Note: generatePrompt is a private method, so we test it indirectly through AnalyzeAlert
// by checking that the request contains the alert data in the mock server

// Note: parseActionRecommendation is a private method, so we test it indirectly through AnalyzeAlert
// by providing different response content in the mock server and checking the parsed results

func TestClient_AnalyzeAlert_NoChoices(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	// Mock server that returns response with no choices
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := LocalAIResponse{
			ID:      "test-id",
			Object:  "chat.completion",
			Model:   "test-model",
			Choices: []Choice{}, // Empty choices
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer mockServer.Close()

	cfg := config.SLMConfig{
		Provider: "localai",
		Endpoint: mockServer.URL,
		Model:    "test-model",
		Timeout:  5 * time.Second,
	}

	client, err := NewClient(cfg, logger)
	require.NoError(t, err)

	alert := types.Alert{
		Name:      "TestAlert",
		Status:    "firing",
		Namespace: "test",
	}

	ctx := context.Background()
	recommendation, err := client.AnalyzeAlert(ctx, alert)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no choices in LocalAI response")
	assert.Nil(t, recommendation)
}

func TestClient_AnalyzeAlert_InvalidResponseJSON(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	// Mock server that returns invalid JSON
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"invalid": json}`))
	}))
	defer mockServer.Close()

	cfg := config.SLMConfig{
		Provider: "localai",
		Endpoint: mockServer.URL,
		Model:    "test-model",
		Timeout:  5 * time.Second,
	}

	client, err := NewClient(cfg, logger)
	require.NoError(t, err)

	alert := types.Alert{
		Name:      "TestAlert",
		Status:    "firing",
		Namespace: "test",
	}

	ctx := context.Background()
	recommendation, err := client.AnalyzeAlert(ctx, alert)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decode LocalAI response")
	assert.Nil(t, recommendation)
}

func TestClient_RetryLogic(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	attempts := 0
	// Mock server that fails first two attempts, succeeds on third
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		response := LocalAIResponse{
			Choices: []Choice{
				{
					Message: Message{
						Role: "assistant",
						Content: `{
							"action": "notify_only",
							"confidence": 0.5,
							"reasoning": "Finally succeeded"
						}`,
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer mockServer.Close()

	cfg := config.SLMConfig{
		Provider:   "localai",
		Endpoint:   mockServer.URL,
		Model:      "test-model",
		Timeout:    5 * time.Second,
		RetryCount: 3,
	}

	client, err := NewClient(cfg, logger)
	require.NoError(t, err)

	alert := types.Alert{
		Name:      "TestAlert",
		Status:    "firing",
		Namespace: "test",
	}

	ctx := context.Background()
	recommendation, err := client.AnalyzeAlert(ctx, alert)

	assert.NoError(t, err)
	assert.NotNil(t, recommendation)
	assert.Equal(t, "notify_only", recommendation.Action)
	assert.Equal(t, 3, attempts) // Should have made 3 attempts
}

func TestLocalAIStructures(t *testing.T) {
	// Test LocalAI request structure marshaling
	req := LocalAIRequest{
		Model: "test-model",
		Messages: []Message{
			{Role: "user", Content: "test message"},
		},
		Temperature: 0.7,
		MaxTokens:   100,
		Stream:      false,
	}

	data, err := json.Marshal(req)
	assert.NoError(t, err)
	assert.Contains(t, string(data), "test-model")
	assert.Contains(t, string(data), "test message")

	// Test LocalAI response structure unmarshaling
	responseJSON := `{
		"id": "test-id",
		"object": "chat.completion",
		"created": 1234567890,
		"model": "test-model",
		"choices": [
			{
				"index": 0,
				"message": {
					"role": "assistant",
					"content": "test response"
				},
				"finish_reason": "stop"
			}
		],
		"usage": {
			"prompt_tokens": 10,
			"completion_tokens": 5,
			"total_tokens": 15
		}
	}`

	var resp LocalAIResponse
	err = json.Unmarshal([]byte(responseJSON), &resp)
	assert.NoError(t, err)
	assert.Equal(t, "test-id", resp.ID)
	assert.Equal(t, "chat.completion", resp.Object)
	assert.Equal(t, "test-model", resp.Model)
	assert.Len(t, resp.Choices, 1)
	assert.Equal(t, "test response", resp.Choices[0].Message.Content)
	assert.Equal(t, 15, resp.Usage.TotalTokens)
}

// FakeSLMClient implements the Client interface for testing
type FakeSLMClient struct {
	healthy        bool
	recommendation *types.ActionRecommendation
	err            error
	callCount      int
}

func NewFakeSLMClient(healthy bool) *FakeSLMClient {
	return &FakeSLMClient{
		healthy: healthy,
		recommendation: &types.ActionRecommendation{
			Action:     "notify_only",
			Confidence: 0.5,
			Reasoning:  "Fake SLM response for testing",
			Parameters: map[string]interface{}{},
		},
	}
}

func (f *FakeSLMClient) AnalyzeAlert(ctx context.Context, alert types.Alert) (*types.ActionRecommendation, error) {
	f.callCount++
	if f.err != nil {
		return nil, f.err
	}
	return f.recommendation, nil
}

func (f *FakeSLMClient) IsHealthy() bool {
	return f.healthy
}

func (f *FakeSLMClient) SetError(err error) {
	f.err = err
}

func (f *FakeSLMClient) SetRecommendation(rec *types.ActionRecommendation) {
	f.recommendation = rec
}

func (f *FakeSLMClient) GetCallCount() int {
	return f.callCount
}

func TestFakeSLMClient(t *testing.T) {
	// Test fake client creation
	client := NewFakeSLMClient(true)
	assert.True(t, client.IsHealthy())
	assert.Equal(t, 0, client.GetCallCount())

	// Test analyze alert
	alert := types.Alert{
		Name:      "TestAlert",
		Status:    "firing",
		Namespace: "test",
	}

	ctx := context.Background()
	recommendation, err := client.AnalyzeAlert(ctx, alert)
	assert.NoError(t, err)
	assert.NotNil(t, recommendation)
	assert.Equal(t, "notify_only", recommendation.Action)
	assert.Equal(t, 1, client.GetCallCount())

	// Test error handling
	testErr := fmt.Errorf("fake error")
	client.SetError(testErr)
	recommendation, err = client.AnalyzeAlert(ctx, alert)
	assert.Error(t, err)
	assert.Equal(t, testErr, err)
	assert.Nil(t, recommendation)
	assert.Equal(t, 2, client.GetCallCount())

	// Test custom recommendation
	customRec := &types.ActionRecommendation{
		Action:     "scale_deployment",
		Confidence: 0.9,
		Reasoning:  "Custom recommendation",
		Parameters: map[string]interface{}{"replicas": 5},
	}
	client.SetError(nil)
	client.SetRecommendation(customRec)
	recommendation, err = client.AnalyzeAlert(ctx, alert)
	assert.NoError(t, err)
	assert.Equal(t, customRec, recommendation)
	assert.Equal(t, 3, client.GetCallCount())

	// Test unhealthy client
	client = NewFakeSLMClient(false)
	assert.False(t, client.IsHealthy())
}