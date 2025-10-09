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

package shared

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jordigilh/kubernaut/pkg/ai/llm"
	"github.com/jordigilh/kubernaut/pkg/infrastructure/metrics"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

// TestSLMClient provides a comprehensive test SLM client for integration testing
type TestSLMClient struct {
	responses     map[string]*types.ActionRecommendation
	chatResponses map[string]string
	healthStatus  bool
	callHistory   []TestSLMCall
	latency       time.Duration
	simulateError bool
	errorMessage  string
	callCount     int
	maxCalls      int // For testing rate limiting

	// Enhanced capabilities for realistic behavior
	confidencePatterns map[string]float64 // Alert type -> confidence mapping
	responseVariation  float64            // 0.0-1.0, adds randomness to responses

	// Enhanced decision making
	decisionEngine       *SophisticatedDecisionEngine
	useEnhancedDecisions bool
	memoryEnabled        bool                 // Remember previous interactions
	previousAlerts       map[string]time.Time // Alert fingerprint -> last seen time
	rateLimitWindow      time.Duration        // Rate limiting window
	rateLimitCounts      map[time.Time]int    // Track calls per time window
	networkSimulation    NetworkSimulation    // Simulate network conditions
	modelAvailability    map[string]bool      // Simulate different model availability
	complexityDelay      bool                 // Add delay based on alert complexity

	// Error injection capabilities
	errorInjection       ErrorInjectionConfig
	circuitBreaker       CircuitBreakerState
	circuitBreakerConfig CircuitBreakerConfig
	failureCount         int
	lastFailureTime      time.Time
	successCount         int
	activeScenarios      map[string]bool
}

// REMOVED: FakeSLMClient alias - Use TestSLMClient directly for cleaner code

type TestSLMCall struct {
	Method     string
	Alert      *types.Alert
	Prompt     string
	Timestamp  time.Time
	Duration   time.Duration
	Success    bool
	ErrorType  string
	Confidence float64
	ModelUsed  string
}

// NetworkSimulation represents network conditions for realistic testing
type NetworkSimulation struct {
	Enabled     bool
	LatencyMin  time.Duration
	LatencyMax  time.Duration
	FailureRate float64 // 0.0-1.0, probability of request failure
	TimeoutRate float64 // 0.0-1.0, probability of timeout
	RetryAfter  time.Duration
}

// RealisticScenario defines a testing scenario with specific behaviors
type RealisticScenario struct {
	Name              string
	ConfidenceRange   [2]float64 // [min, max]
	LatencyMultiplier float64
	ErrorRate         float64
	ConsistencyLevel  float64 // How consistent responses should be
}

// ErrorInjectionConfig controls error injection behavior
type ErrorInjectionConfig struct {
	Enabled             bool
	ErrorRate           float64                   // 0.0-1.0 probability of injecting errors
	ErrorTypes          []ErrorCategory           // Which error types to simulate
	TriggerAfter        int                       // Trigger after N successful operations
	RecoveryAfter       time.Duration             // Automatic recovery time
	ErrorDistribution   map[ErrorCategory]float64 // Weight distribution of error types
	MaxConcurrentErrors int                       // Maximum concurrent error scenarios
}

// CircuitBreakerState represents the state of the circuit breaker
type CircuitBreakerState string

const (
	CircuitClosed   CircuitBreakerState = "closed"    // Normal operation
	CircuitOpen     CircuitBreakerState = "open"      // Failing fast
	CircuitHalfOpen CircuitBreakerState = "half_open" // Testing recovery
)

// CircuitBreakerConfig configures circuit breaker behavior
type CircuitBreakerConfig struct {
	Enabled          bool
	FailureThreshold int           // Failures before opening circuit
	RecoveryTimeout  time.Duration // Time before attempting half-open
	SuccessThreshold int           // Successful calls needed to close circuit
	HalfOpenMaxCalls int           // Maximum calls allowed in half-open state
}

// NewSLMClient creates a new test SLM client with realistic behavior for integration testing
func NewSLMClient() *TestSLMClient {
	client := &TestSLMClient{
		responses:            make(map[string]*types.ActionRecommendation),
		chatResponses:        make(map[string]string),
		healthStatus:         true,
		callHistory:          make([]TestSLMCall, 0),
		latency:              10 * time.Millisecond, // Default fast response
		maxCalls:             1000,                  // Default high limit
		confidencePatterns:   make(map[string]float64),
		responseVariation:    0.1,                              // 10% variation by default
		useEnhancedDecisions: true,                             // Enable enhanced decisions by default
		decisionEngine:       NewSophisticatedDecisionEngine(), // Initialize decision engine
		memoryEnabled:        true,                             // Enable memory by default
		previousAlerts:       make(map[string]time.Time),
		rateLimitWindow:      time.Minute,
		rateLimitCounts:      make(map[time.Time]int),
		networkSimulation: NetworkSimulation{
			Enabled:     false, // Disabled by default for fast tests
			LatencyMin:  50 * time.Millisecond,
			LatencyMax:  200 * time.Millisecond,
			FailureRate: 0.02, // 2% failure rate
			TimeoutRate: 0.01, // 1% timeout rate
		},
		modelAvailability: map[string]bool{
			"granite3.1-dense:8b": true,
			"llama2:7b":           true,
			"fake-test-model":     true,
		},
		complexityDelay: true,

		// Initialize error injection
		errorInjection: ErrorInjectionConfig{
			Enabled:             false, // Disabled by default
			ErrorRate:           0.0,
			ErrorTypes:          []ErrorCategory{},
			ErrorDistribution:   make(map[ErrorCategory]float64),
			MaxConcurrentErrors: 1,
		},
		circuitBreaker: CircuitClosed,
		circuitBreakerConfig: CircuitBreakerConfig{
			Enabled:          false,
			FailureThreshold: 5,
			RecoveryTimeout:  30 * time.Second,
			SuccessThreshold: 3,
			HalfOpenMaxCalls: 3,
		},
		activeScenarios: make(map[string]bool),
	}

	// Pre-configure realistic responses and patterns
	client.setupDefaultResponses()
	client.setupConfidencePatterns()
	return client
}

// NewTestSLMClient creates a new TestSLMClient (replaces deprecated NewFakeSLMClient)
func NewTestSLMClient() *TestSLMClient {
	return NewSLMClient()
}

// NewSLMClientWithScenario creates a test client configured for a specific testing scenario
func NewSLMClientWithScenario(scenario RealisticScenario) *TestSLMClient {
	client := NewSLMClient()
	// Note: ConfigureForScenario method not found, may need to be implemented if needed
	// client.ConfigureForScenario(scenario)
	return client
}

// setupDefaultResponses configures realistic responses for common scenarios
func (f *TestSLMClient) setupDefaultResponses() {
	// OOM alerts
	f.AddResponse("OOMKilled", &types.ActionRecommendation{
		Action:     "increase_resources",
		Confidence: 0.85,
		Reasoning: &types.ReasoningDetails{
			Summary: "Pod was killed due to out-of-memory condition. Increasing resource limits is the most effective remedy.",
		},
	})

	// Memory pressure alerts
	f.AddResponse("HighMemoryUsage", &types.ActionRecommendation{
		Action:     "collect_diagnostics",
		Confidence: 0.72,
		Reasoning: &types.ReasoningDetails{
			Summary: "High memory usage detected. Collecting diagnostics to determine if this is a leak or expected load.",
		},
	})

	// Pod crash looping
	f.AddResponse("PodCrashLooping", &types.ActionRecommendation{
		Action:     "restart_pod",
		Confidence: 0.68,
		Reasoning: &types.ReasoningDetails{
			Summary: "Pod is crash looping. Attempting restart to recover from transient issues.",
		},
	})

	// Security threats
	f.AddResponse("SecurityThreat", &types.ActionRecommendation{
		Action:     "isolate_workload",
		Confidence: 0.92,
		Reasoning: &types.ReasoningDetails{
			Summary: "Security threat detected. Immediate isolation required to prevent lateral movement.",
		},
	})

	// Network connectivity issues
	f.AddResponse("NetworkConnectivityIssue", &types.ActionRecommendation{
		Action:     "check_network_policies",
		Confidence: 0.64,
		Reasoning: &types.ReasoningDetails{
			Summary: "Network connectivity problems detected. Checking network policies and service configurations.",
		},
	})

	// Default chat completion responses
	f.AddChatResponse("analyze memory patterns", `{
		"action": "increase_resources",
		"confidence": 0.83,
		"reasoning": "Memory usage shows consistent growth pattern indicative of a memory leak"
	}`)

	f.AddChatResponse("security incident response", `{
		"action": "isolate_workload",
		"confidence": 0.91,
		"reasoning": "Security incident requires immediate containment to prevent spread"
	}`)
}

// Configurable behavior methods
func (f *TestSLMClient) SetHealthStatus(healthy bool) {
	f.healthStatus = healthy
}

func (f *TestSLMClient) SetLatency(latency time.Duration) {
	f.latency = latency
}

func (f *TestSLMClient) SimulateError(message string) {
	f.simulateError = true
	f.errorMessage = message
}

func (f *TestSLMClient) ClearError() {
	f.simulateError = false
	f.errorMessage = ""
}

func (f *TestSLMClient) SetMaxCalls(maxCalls int) {
	f.maxCalls = maxCalls
}

func (f *TestSLMClient) AddResponse(alertName string, response *types.ActionRecommendation) {
	f.responses[alertName] = response
}

func (f *TestSLMClient) AddChatResponse(prompt string, response string) {
	f.chatResponses[prompt] = response
}

// Interface implementation - Updated to match llm.Client interface
func (f *TestSLMClient) AnalyzeAlert(ctx context.Context, alert interface{}) (*llm.AnalyzeAlertResponse, error) {
	// Convert interface{} to types.Alert for internal processing
	var typedAlert types.Alert
	if a, ok := alert.(types.Alert); ok {
		typedAlert = a
	} else {
		return nil, fmt.Errorf("expected types.Alert, got %T", alert)
	}
	startTime := time.Now()
	var success bool
	defer func() {
		// Update circuit breaker state after call completion
		f.updateCircuitBreakerState(success)

		// Record call with complete information
		duration := time.Since(startTime)
		f.callHistory[len(f.callHistory)-1].Duration = duration
		f.callHistory[len(f.callHistory)-1].Success = success
	}()

	// Record call (will be updated in defer)
	f.callHistory = append(f.callHistory, TestSLMCall{
		Method:    "AnalyzeAlert",
		Alert:     &typedAlert,
		Timestamp: startTime,
	})

	f.callCount++

	// Check circuit breaker state first
	if f.isCircuitBreakerOpen() {
		success = false
		return nil, errors.New("circuit breaker is open - SLM service unavailable")
	}

	// Check rate limiting
	if f.callCount > f.maxCalls {
		success = false
		return nil, errors.New("rate limit exceeded")
	}

	// Check for error injection
	if shouldInject, errorCategory := f.shouldInjectError(); shouldInject {
		success = false
		switch errorCategory {
		case NetworkError:
			return nil, errors.New("network connection failed")
		case TimeoutError:
			return nil, context.DeadlineExceeded
		case RateLimitError:
			return nil, errors.New("rate limit exceeded")
		case SLMError:
			return nil, errors.New("SLM model inference failed")
		default:
			return nil, errors.New("injected error for testing")
		}
	}

	// Simulate network conditions if enabled
	if f.networkSimulation.Enabled {
		// Check for network failure
		if f.networkSimulation.FailureRate > 0 && f.callCount%int(1.0/f.networkSimulation.FailureRate) == 0 {
			success = false
			return nil, errors.New("connection refused: network failure simulation")
		}

		// Check for timeout
		if f.networkSimulation.TimeoutRate > 0 && f.callCount%int(1.0/f.networkSimulation.TimeoutRate) == 0 {
			success = false
			return nil, context.DeadlineExceeded
		}

		// Add variable latency
		if f.networkSimulation.LatencyMax > f.networkSimulation.LatencyMin {
			variableLatency := f.networkSimulation.LatencyMin +
				time.Duration(f.callCount%int(f.networkSimulation.LatencyMax-f.networkSimulation.LatencyMin))
			f.latency = variableLatency
		}
	}

	// Simulate latency
	select {
	case <-time.After(f.latency):
	case <-ctx.Done():
		success = false
		return nil, ctx.Err()
	}

	// Simulate error if configured (legacy support)
	if f.simulateError {
		success = false
		return nil, errors.New(f.errorMessage)
	}

	// If we get here, the call was successful
	success = true

	// BR-3B: Emit context size metrics for monitoring during SLM analysis
	contextSize := f.calculateContextSize(typedAlert)
	provider := f.getProviderName()
	metrics.RecordSLMContextSize(provider, contextSize)

	// Return configured response or default
	if response, exists := f.responses[typedAlert.Name]; exists {
		// Convert ActionRecommendation to AnalyzeAlertResponse
		return &llm.AnalyzeAlertResponse{
			Action:     response.Action,
			Confidence: response.Confidence,
			Reasoning:  response.Reasoning,
			Parameters: response.Parameters,
		}, nil
	}

	// Generate default response based on alert characteristics
	if f.useEnhancedDecisions && f.decisionEngine != nil {
		enhanced := f.generateEnhancedResponse(typedAlert)
		return &llm.AnalyzeAlertResponse{
			Action:     enhanced.Action,
			Confidence: enhanced.Confidence,
			Reasoning:  enhanced.Reasoning,
			Parameters: enhanced.Parameters,
		}, nil
	}

	defaultResp := f.generateDefaultResponse(typedAlert)
	return &llm.AnalyzeAlertResponse{
		Action:     defaultResp.Action,
		Confidence: defaultResp.Confidence,
		Reasoning:  defaultResp.Reasoning,
		Parameters: defaultResp.Parameters,
	}, nil
}

func (f *TestSLMClient) ChatCompletion(ctx context.Context, prompt string) (string, error) {
	startTime := time.Now()
	var success bool
	defer func() {
		// Update circuit breaker state after call completion
		f.updateCircuitBreakerState(success)

		// Record call with complete information
		duration := time.Since(startTime)
		f.callHistory[len(f.callHistory)-1].Duration = duration
		f.callHistory[len(f.callHistory)-1].Success = success
	}()

	// Record call (will be updated in defer)
	f.callHistory = append(f.callHistory, TestSLMCall{
		Method:    "ChatCompletion",
		Prompt:    prompt,
		Timestamp: startTime,
	})

	f.callCount++

	// Check circuit breaker state first
	if f.isCircuitBreakerOpen() {
		success = false
		return "", errors.New("circuit breaker is open - SLM service unavailable")
	}

	// Check rate limiting
	if f.callCount > f.maxCalls {
		success = false
		return "", errors.New("rate limit exceeded")
	}

	// Check for error injection
	if shouldInject, errorCategory := f.shouldInjectError(); shouldInject {
		success = false
		switch errorCategory {
		case NetworkError:
			return "", errors.New("network connection failed")
		case TimeoutError:
			return "", context.DeadlineExceeded
		case RateLimitError:
			return "", errors.New("rate limit exceeded")
		case SLMError:
			return "", errors.New("SLM model inference failed")
		default:
			return "", errors.New("injected error for testing")
		}
	}

	// Simulate network conditions if enabled
	if f.networkSimulation.Enabled {
		// Check for network failure
		if f.networkSimulation.FailureRate > 0 && f.callCount%int(1.0/f.networkSimulation.FailureRate) == 0 {
			success = false
			return "", errors.New("connection refused: network failure simulation")
		}

		// Check for timeout
		if f.networkSimulation.TimeoutRate > 0 && f.callCount%int(1.0/f.networkSimulation.TimeoutRate) == 0 {
			success = false
			return "", context.DeadlineExceeded
		}
	}

	// Simulate latency
	select {
	case <-time.After(f.latency):
	case <-ctx.Done():
		success = false
		return "", ctx.Err()
	}

	// Simulate error if configured (legacy support)
	if f.simulateError {
		success = false
		return "", errors.New(f.errorMessage)
	}

	// If we get here, the call was successful
	success = true

	// Return configured response or default
	if response, exists := f.chatResponses[prompt]; exists {
		return response, nil
	}

	return `{"action": "notify_only", "confidence": 0.7, "reasoning": "Fake chat completion response"}`, nil
}

func (f *TestSLMClient) GenerateResponse(prompt string) (string, error) {
	// Delegate to ChatCompletion for consistency
	ctx := context.Background()
	return f.ChatCompletion(ctx, prompt)
}

func (f *TestSLMClient) IsHealthy() bool {
	return f.healthStatus
}

func (f *TestSLMClient) GenerateWorkflow(ctx context.Context, objective *llm.WorkflowObjective) (*llm.WorkflowGenerationResult, error) {
	// Simulate latency
	select {
	case <-time.After(f.latency):
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// Check for errors
	if f.simulateError {
		return nil, errors.New(f.errorMessage)
	}

	// Return a basic mock workflow
	return &llm.WorkflowGenerationResult{
		WorkflowID:  fmt.Sprintf("fake-workflow-%s", objective.ID),
		Name:        fmt.Sprintf("Fake Workflow for %s", objective.Type),
		Description: fmt.Sprintf("Mock workflow generated for %s objective", objective.Description),
		Steps: []*llm.AIGeneratedStep{
			{
				ID:   "step-1",
				Name: "Mock Step",
				Type: "action",
				Action: &llm.AIStepAction{
					Type:       "notify_only",
					Parameters: map[string]interface{}{"message": "Mock workflow execution"},
				},
				Timeout: "5m",
			},
		},
		Variables: map[string]interface{}{
			"test": "fake-value",
		},
		Timeouts: &llm.WorkflowTimeouts{
			Execution: "30m",
			Step:      "5m",
			Condition: "30s",
		},
		Confidence: 0.8,
		Reasoning:  "Mock workflow generated by fake client",
	}, nil
}

// Helper methods for testing
func (f *TestSLMClient) GetCallHistory() []TestSLMCall {
	return f.callHistory
}

func (f *TestSLMClient) GetCallCount() int {
	return f.callCount
}

func (f *TestSLMClient) ResetCallHistory() {
	f.callHistory = make([]TestSLMCall, 0)
	f.callCount = 0
}

func (f *TestSLMClient) generateDefaultResponse(alert types.Alert) *types.ActionRecommendation {
	// Generate realistic responses based on alert characteristics
	var action string
	var confidence float64
	var reasoning string

	// First check confidence patterns for test scenarios
	if f.confidencePatterns != nil {
		// Check for direct alert name match
		if pattern, exists := f.confidencePatterns[alert.Name]; exists {
			confidence = pattern
		}
		// Check for test scenario patterns (like "obvious_storage_expansion")
		for pattern, patternConfidence := range f.confidencePatterns {
			if pattern != "default" && (alert.Name == pattern || alert.Description == pattern ||
				// Check if alert description contains the pattern name
				(len(pattern) > 5 && alert.Description != "" && strings.Contains(strings.ToLower(alert.Description), strings.ToLower(pattern))) ||
				// Check if alert resource contains pattern
				(alert.Resource != "" && strings.Contains(strings.ToLower(alert.Resource), strings.ToLower(pattern))) ||
				// Check alert labels for matching patterns
				(alert.Labels != nil && alert.Labels["scenario"] == pattern)) {
				confidence = patternConfidence
				break
			}
		}
		// Use default if no pattern matched
		if confidence == 0 {
			if defaultConf, exists := f.confidencePatterns["default"]; exists {
				confidence = defaultConf
			}
		}
	}

	// Determine action based on alert characteristics (check specific alerts before generic severity)
	switch {
	case alert.Name == "ActiveSecurityThreat" || strings.Contains(alert.Description, "malware"):
		action = "quarantine_pod"
		if confidence == 0 {
			confidence = 0.9
		}
		reasoning = "Security threat detected, quarantining affected pod"

	case alert.Name == "PVCNearFull" || alert.Description == "obvious_storage_expansion":
		action = "expand_pvc"
		if confidence == 0 {
			confidence = 0.9
		}
		reasoning = "Storage space is critically low, expanding PVC"

	case alert.Name == "OOMKilled" || alert.Labels["reason"] == "OOMKilled":
		action = "increase_resources"
		if confidence == 0 {
			confidence = 0.85
		}
		reasoning = "Pod was killed due to out-of-memory, increasing resource limits"

	case alert.Name == "PodCrashLooping":
		action = "restart_pod"
		if confidence == 0 {
			confidence = 0.75
		}
		reasoning = "Pod is crash looping, attempting restart"

	case alert.Name == "DeploymentFailed" || strings.Contains(alert.Description, "completely failed"):
		action = "rollback_deployment"
		if confidence == 0 {
			confidence = 0.8
		}
		reasoning = "Deployment failure detected, rolling back to last stable version"

	case alert.Name == "SecurityIncident" || alert.Labels["threat_type"] != "" || strings.Contains(alert.Description, "Security breach"):
		action = "quarantine_pod"
		if confidence == 0 {
			confidence = 0.85
		}
		reasoning = "Security incident detected, quarantining affected resources"

	case alert.Name == "MemoryExhaustion" || alert.Name == "DiskSpaceExhaustion" || alert.Name == "FileDescriptorExhaustion" || strings.Contains(alert.Description, "exhaustion"):
		action = "scale_and_increase_resources"
		if confidence == 0 {
			confidence = 0.8
		}
		reasoning = "Resource exhaustion detected, scaling and increasing resources"

	case alert.Name == "ResourceExhaustion" || alert.Name == "CPUExhaustion" || strings.Contains(alert.Description, "resource exhaustion"):
		action = "scale_and_increase_resources"
		if confidence == 0 {
			confidence = 0.8
		}
		reasoning = "Resource exhaustion detected, scaling and increasing resources"

	case alert.Severity == "critical":
		action = "escalate"
		if confidence == 0 {
			confidence = 0.9
		}
		reasoning = fmt.Sprintf("Critical alert for %s requires immediate escalation", alert.Resource)

	case alert.Severity == "warning":
		action = "collect_diagnostics"
		if confidence == 0 {
			confidence = 0.6
		}
		reasoning = fmt.Sprintf("Warning level alert for %s, collecting diagnostics", alert.Resource)

	default:
		// Check for escalation scenarios (system instability, critical alerts, multiple failures)
		if alert.Name == "SystemInstability" || alert.Severity == "critical" || alert.Description == "escalate" || strings.Contains(alert.Name, "Instability") {
			action = "escalate"
			if confidence == 0 {
				confidence = 0.7
			}
			reasoning = "System instability or critical alert detected, escalating for manual intervention"
		} else {
			action = "notify_only"
			if confidence == 0 {
				confidence = 0.5
			}
			reasoning = "Standard alert monitoring, no immediate action required"
		}
	}

	return &types.ActionRecommendation{
		Action:     action,
		Confidence: confidence,
		Reasoning: &types.ReasoningDetails{
			Summary: reasoning,
		},
	}
}

// generateEnhancedResponse uses the sophisticated decision engine
func (f *TestSLMClient) generateEnhancedResponse(alert types.Alert) *types.ActionRecommendation {
	// Use enhanced decision engine
	ctx := f.decisionEngine.AnalyzeContext(alert)
	action, confidence, reasoning := f.decisionEngine.MakeDecision(ctx)

	return &types.ActionRecommendation{
		Action:     action,
		Confidence: confidence,
		Reasoning: &types.ReasoningDetails{
			Summary: reasoning,
		},
	}
}

// SetUseEnhancedDecisions enables or disables enhanced decision making
func (f *TestSLMClient) SetUseEnhancedDecisions(use bool) {
	f.useEnhancedDecisions = use
}

// GetDecisionEngine returns the decision engine for advanced configuration
func (f *TestSLMClient) GetDecisionEngine() *SophisticatedDecisionEngine {
	return f.decisionEngine
}

// Enhanced Error Injection Methods as specified in TODO requirements

// ConfigureErrorInjection configures comprehensive error injection behavior
func (f *TestSLMClient) ConfigureErrorInjection(config ErrorInjectionConfig) {
	f.errorInjection = config

	// Reset circuit breaker state if error injection is being reconfigured
	if config.Enabled {
		f.circuitBreaker = CircuitClosed
		f.failureCount = 0
		f.successCount = 0
	}
}

// TriggerErrorScenario triggers a specific error scenario for testing
func (f *TestSLMClient) TriggerErrorScenario(scenario ErrorScenario) error {
	if !f.errorInjection.Enabled {
		return fmt.Errorf("error injection is not enabled")
	}

	// Check if we can trigger more concurrent scenarios
	activeCount := 0
	for _, active := range f.activeScenarios {
		if active {
			activeCount++
		}
	}

	if activeCount >= f.errorInjection.MaxConcurrentErrors {
		return fmt.Errorf("maximum concurrent error scenarios reached (%d)", f.errorInjection.MaxConcurrentErrors)
	}

	// Activate the scenario
	f.activeScenarios[scenario.Name] = true

	// Configure error injection based on scenario
	switch scenario.Category {
	case NetworkError:
		f.networkSimulation.Enabled = true
		f.networkSimulation.FailureRate = 0.8 // High failure rate for scenario
		f.networkSimulation.TimeoutRate = 0.3

	case TimeoutError:
		f.latency = scenario.Duration
		f.networkSimulation.TimeoutRate = 1.0 // Always timeout

	case SLMError:
		f.simulateError = true
		f.errorMessage = fmt.Sprintf("SLM service degradation: %s", scenario.Description)

	case RateLimitError:
		f.maxCalls = 1 // Trigger rate limiting immediately

	default:
		f.simulateError = true
		f.errorMessage = fmt.Sprintf("Error scenario: %s", scenario.Name)
	}

	// Schedule automatic recovery if specified
	if scenario.RecoveryTime > 0 {
		go func() {
			time.Sleep(scenario.RecoveryTime)
			f.recoverFromScenario(scenario.Name)
		}()
	}

	return nil
}

// GetCircuitBreakerState returns the current circuit breaker state
func (f *TestSLMClient) GetCircuitBreakerState() CircuitBreakerState {
	return f.circuitBreaker
}

// SetErrorInjectionEnabled enables or disables error injection
func (f *TestSLMClient) SetErrorInjectionEnabled(enabled bool) {
	f.errorInjection.Enabled = enabled
}

// calculateContextSize estimates the context size in bytes for metrics collection (BR-3B)
func (f *TestSLMClient) calculateContextSize(alert types.Alert) int {
	// Calculate rough context size based on alert data
	contextSize := 0

	// Basic alert fields
	contextSize += len(alert.Name)
	contextSize += len(alert.Description)
	contextSize += len(alert.Namespace)
	contextSize += len(alert.Resource)
	contextSize += len(alert.Severity)
	contextSize += len(alert.Status)

	// Labels and annotations
	for k, v := range alert.Labels {
		contextSize += len(k) + len(v)
	}
	for k, v := range alert.Annotations {
		contextSize += len(k) + len(v)
	}

	// Add baseline prompt overhead (typical SLM prompt structure)
	baselinePrompt := 1200 // Estimated base prompt size in bytes
	contextSize += baselinePrompt

	// Simulate varying context sizes based on alert complexity
	if len(alert.Description) > 100 {
		contextSize += len(alert.Description) * 2 // Historical context for complex alerts
	}

	return contextSize
}

// getProviderName returns the provider name for metrics labeling (BR-3B)
func (f *TestSLMClient) getProviderName() string {
	// Return a consistent provider name for test metrics
	return "fake_test_client"
}

// ResetErrorState resets all error injection and circuit breaker state
func (f *TestSLMClient) ResetErrorState() {
	f.errorInjection.Enabled = false
	f.errorInjection.ErrorRate = 0.0
	f.circuitBreaker = CircuitClosed
	f.failureCount = 0
	f.successCount = 0
	f.lastFailureTime = time.Time{}
	f.simulateError = false
	f.errorMessage = ""

	// Reset network simulation
	f.networkSimulation.Enabled = false
	f.networkSimulation.FailureRate = 0.0
	f.networkSimulation.TimeoutRate = 0.0
	f.latency = 10 * time.Millisecond

	// Reset rate limiting
	f.maxCalls = 1000
	f.callCount = 0

	// Clear active scenarios
	f.activeScenarios = make(map[string]bool)
}

// EnableCircuitBreaker enables circuit breaker functionality with given config
func (f *TestSLMClient) EnableCircuitBreaker(config CircuitBreakerConfig) {
	f.circuitBreakerConfig = config
	f.circuitBreakerConfig.Enabled = true
	f.circuitBreaker = CircuitClosed
}

// DisableCircuitBreaker disables circuit breaker functionality
func (f *TestSLMClient) DisableCircuitBreaker() {
	f.circuitBreakerConfig.Enabled = false
	f.circuitBreaker = CircuitClosed
}

// Internal helper methods for error injection

// recoverFromScenario recovers from a specific error scenario
func (f *TestSLMClient) recoverFromScenario(scenarioName string) {
	delete(f.activeScenarios, scenarioName)

	// If no active scenarios remain, reset to normal operation
	if len(f.activeScenarios) == 0 {
		f.networkSimulation.Enabled = false
		f.networkSimulation.FailureRate = 0.02 // Back to normal 2%
		f.networkSimulation.TimeoutRate = 0.01 // Back to normal 1%
		f.latency = 10 * time.Millisecond      // Back to fast response
		f.simulateError = false
		f.errorMessage = ""
		f.maxCalls = 1000 // Reset rate limiting
	}
}

// shouldInjectError determines if an error should be injected based on configuration
func (f *TestSLMClient) shouldInjectError() (bool, ErrorCategory) {
	if !f.errorInjection.Enabled {
		return false, ""
	}

	// Check trigger conditions
	if f.errorInjection.TriggerAfter > 0 && f.callCount <= f.errorInjection.TriggerAfter {
		return false, ""
	}

	// Check error rate probability
	if f.errorInjection.ErrorRate == 0 {
		return false, ""
	}

	// Simple probability check (in real implementation would use proper random)
	if f.callCount%int(1.0/f.errorInjection.ErrorRate) == 0 {
		// Select error type based on distribution
		if len(f.errorInjection.ErrorTypes) > 0 {
			// Use first error type for simplicity (real implementation would use weighted selection)
			return true, f.errorInjection.ErrorTypes[0]
		}
		return true, NetworkError // Default error type
	}

	return false, ""
}

// updateCircuitBreakerState updates circuit breaker state based on call outcome
func (f *TestSLMClient) updateCircuitBreakerState(success bool) {
	if !f.circuitBreakerConfig.Enabled {
		return
	}

	switch f.circuitBreaker {
	case CircuitClosed:
		if success {
			f.failureCount = 0 // Reset failure count on success
		} else {
			f.failureCount++
			f.lastFailureTime = time.Now()
			if f.failureCount >= f.circuitBreakerConfig.FailureThreshold {
				f.circuitBreaker = CircuitOpen
			}
		}

	case CircuitOpen:
		// Check if recovery timeout has passed
		if time.Since(f.lastFailureTime) >= f.circuitBreakerConfig.RecoveryTimeout {
			f.circuitBreaker = CircuitHalfOpen
			f.successCount = 0
		}

	case CircuitHalfOpen:
		if success {
			f.successCount++
			if f.successCount >= f.circuitBreakerConfig.SuccessThreshold {
				f.circuitBreaker = CircuitClosed
				f.failureCount = 0
			}
		} else {
			f.circuitBreaker = CircuitOpen
			f.failureCount++
			f.lastFailureTime = time.Now()
		}
	}
}

// isCircuitBreakerOpen checks if circuit breaker should prevent calls
func (f *TestSLMClient) isCircuitBreakerOpen() bool {
	return f.circuitBreakerConfig.Enabled && f.circuitBreaker == CircuitOpen
}

// Additional health monitoring methods required for llm.Client interface

// LivenessCheck performs a liveness check simulation
func (f *TestSLMClient) LivenessCheck(ctx context.Context) error {
	if f.simulateError {
		return errors.New(f.errorMessage)
	}
	if !f.healthStatus {
		return errors.New("test SLM client is not healthy")
	}

	// Simulate network latency for realistic testing
	time.Sleep(f.latency)
	return nil
}

// ReadinessCheck performs a readiness check simulation
func (f *TestSLMClient) ReadinessCheck(ctx context.Context) error {
	if f.simulateError {
		return errors.New(f.errorMessage)
	}
	if !f.healthStatus {
		return errors.New("test SLM client is not ready")
	}

	// Simulate more complex readiness check with model availability
	for model, available := range f.modelAvailability {
		if !available {
			return fmt.Errorf("model %s is not available", model)
		}
	}

	// Simulate network latency for realistic testing
	time.Sleep(f.latency)
	return nil
}

// GetEndpoint returns the simulated endpoint for the test client
func (f *TestSLMClient) GetEndpoint() string {
	return "http://test-slm-client:8080"
}

// GetModel returns the simulated model name for the test client
func (f *TestSLMClient) GetModel() string {
	return "granite3.1-dense:8b" // Default test model
}

// GetMinParameterCount returns the minimum parameter count for testing
func (f *TestSLMClient) GetMinParameterCount() int64 {
	return 8000000000 // 8B parameters for test model
}

// AnalyzePatterns discovers patterns in execution data using ML
// BR-ML-001: MUST provide machine learning analytics for pattern discovery
func (f *TestSLMClient) AnalyzePatterns(ctx context.Context, executionData []interface{}) (interface{}, error) {
	if f.simulateError {
		return nil, errors.New(f.errorMessage)
	}

	// Simulate pattern analysis
	return map[string]interface{}{
		"patterns":   "Test pattern analysis results",
		"confidence": 0.85,
		"data_size":  len(executionData),
		"clusters":   []string{"pattern1", "pattern2", "pattern3"},
	}, nil
}

// PredictEffectiveness predicts workflow success probability
// BR-ML-001: MUST predict workflow effectiveness using ML
func (f *TestSLMClient) PredictEffectiveness(ctx context.Context, workflow interface{}) (float64, error) {
	if f.simulateError {
		return 0.0, errors.New(f.errorMessage)
	}
	return 0.85, nil // Default test effectiveness score
}

// ClusterWorkflows groups similar workflows using AI clustering
// BR-CLUSTER-001: MUST support workflow clustering and similarity analysis
func (f *TestSLMClient) ClusterWorkflows(ctx context.Context, executionData []interface{}, config map[string]interface{}) (interface{}, error) {
	if f.simulateError {
		return nil, errors.New(f.errorMessage)
	}

	return map[string]interface{}{
		"clusters":      "Test clustering results",
		"cluster_count": 3,
		"confidence":    0.80,
	}, nil
}

// AnalyzeTrends identifies trends in time series data
// BR-TIMESERIES-001: MUST provide time series analysis capabilities
func (f *TestSLMClient) AnalyzeTrends(ctx context.Context, executionData []interface{}, timeRange interface{}) (interface{}, error) {
	if f.simulateError {
		return nil, errors.New(f.errorMessage)
	}

	return map[string]interface{}{
		"trends":     "Test trend analysis results",
		"direction":  "improving",
		"confidence": 0.75,
	}, nil
}

// DetectAnomalies identifies unusual patterns in execution data
// BR-TIMESERIES-001: MUST detect anomalies in execution patterns
func (f *TestSLMClient) DetectAnomalies(ctx context.Context, executionData []interface{}) (interface{}, error) {
	if f.simulateError {
		return nil, errors.New(f.errorMessage)
	}

	return map[string]interface{}{
		"anomalies":  "Test anomaly detection results",
		"count":      2,
		"confidence": 0.70,
	}, nil
}

// BuildPrompt creates optimized prompts from templates
// BR-PROMPT-001: MUST support dynamic prompt building and template optimization
func (f *TestSLMClient) BuildPrompt(ctx context.Context, template string, context map[string]interface{}) (string, error) {
	if f.simulateError {
		return "", errors.New(f.errorMessage)
	}

	// Simple template substitution for testing
	prompt := template
	for key, value := range context {
		placeholder := fmt.Sprintf("{{%s}}", key)
		prompt = strings.ReplaceAll(prompt, placeholder, fmt.Sprintf("%v", value))
	}

	return prompt + "_test_enhanced", nil
}

// LearnFromExecution updates AI models based on execution feedback
// BR-PROMPT-001: MUST learn from execution results to improve future prompts
func (f *TestSLMClient) LearnFromExecution(ctx context.Context, execution interface{}) error {
	if f.simulateError {
		return errors.New(f.errorMessage)
	}
	return nil // Test implementation - no actual learning
}

// GetOptimizedTemplate retrieves pre-optimized templates
// BR-PROMPT-001: MUST provide optimized prompt templates
func (f *TestSLMClient) GetOptimizedTemplate(ctx context.Context, templateID string) (string, error) {
	if f.simulateError {
		return "", errors.New(f.errorMessage)
	}
	return templateID + "_test_optimized", nil
}

// CollectMetrics collects AI metrics from execution
// BR-AI-017, BR-AI-025: MUST provide comprehensive AI metrics collection and analysis
func (f *TestSLMClient) CollectMetrics(ctx context.Context, execution interface{}) (map[string]float64, error) {
	if f.simulateError {
		return nil, errors.New(f.errorMessage)
	}

	return map[string]float64{
		"test_metric_1":    0.85,
		"test_metric_2":    0.92,
		"execution_time":   1.5,
		"confidence_score": 0.78,
	}, nil
}

// GetAggregatedMetrics retrieves aggregated metrics for a workflow
// BR-AI-017, BR-AI-025: MUST provide comprehensive AI metrics collection and analysis
func (f *TestSLMClient) GetAggregatedMetrics(ctx context.Context, workflowID string, timeRange interface{}) (map[string]float64, error) {
	if f.simulateError {
		return nil, errors.New(f.errorMessage)
	}

	return map[string]float64{
		"avg_execution_time": 2.1,
		"success_rate":       0.95,
		"avg_confidence":     0.82,
		"total_executions":   150.0,
	}, nil
}

// RecordAIRequest records AI request for metrics tracking
// BR-AI-017, BR-AI-025: MUST provide comprehensive AI metrics collection and analysis
func (f *TestSLMClient) RecordAIRequest(ctx context.Context, requestID string, prompt string, response string) error {
	if f.simulateError {
		return errors.New(f.errorMessage)
	}
	// Test implementation - just record the call
	return nil
}

// RegisterPromptVersion registers a new prompt version
// BR-AI-022, BR-ORCH-002, BR-ORCH-003: MUST support prompt optimization and A/B testing
func (f *TestSLMClient) RegisterPromptVersion(ctx context.Context, version interface{}) error {
	if f.simulateError {
		return errors.New(f.errorMessage)
	}
	return nil
}

// GetOptimalPrompt retrieves the optimal prompt for an objective
// BR-AI-022, BR-ORCH-002, BR-ORCH-003: MUST support prompt optimization and A/B testing
func (f *TestSLMClient) GetOptimalPrompt(ctx context.Context, objective interface{}) (interface{}, error) {
	if f.simulateError {
		return nil, errors.New(f.errorMessage)
	}
	return "test_optimal_prompt", nil
}

// StartABTest starts an A/B test experiment
// BR-AI-022, BR-ORCH-002, BR-ORCH-003: MUST support prompt optimization and A/B testing
func (f *TestSLMClient) StartABTest(ctx context.Context, experiment interface{}) error {
	if f.simulateError {
		return errors.New(f.errorMessage)
	}
	return nil
}

// OptimizeWorkflow optimizes a workflow using AI
// BR-ORCH-003: MUST provide workflow optimization and improvement suggestions
func (f *TestSLMClient) OptimizeWorkflow(ctx context.Context, workflow interface{}, executionHistory interface{}) (interface{}, error) {
	if f.simulateError {
		return nil, errors.New(f.errorMessage)
	}
	return workflow, nil // Return the same workflow for testing
}

// SuggestOptimizations suggests workflow optimizations
// BR-ORCH-003: MUST provide workflow optimization and improvement suggestions
func (f *TestSLMClient) SuggestOptimizations(ctx context.Context, workflow interface{}) (interface{}, error) {
	if f.simulateError {
		return nil, errors.New(f.errorMessage)
	}
	return []string{"test_optimization_1", "test_optimization_2"}, nil
}

// EvaluateCondition evaluates a condition using AI
// BR-COND-001: MUST support intelligent condition evaluation with context awareness
func (f *TestSLMClient) EvaluateCondition(ctx context.Context, condition interface{}, context interface{}) (bool, error) {
	if f.simulateError {
		return false, errors.New(f.errorMessage)
	}
	return true, nil // Default to true for test success
}

// ValidateCondition validates a condition
// BR-COND-001: MUST support intelligent condition evaluation with context awareness
func (f *TestSLMClient) ValidateCondition(ctx context.Context, condition interface{}) error {
	if f.simulateError {
		return errors.New(f.errorMessage)
	}
	return nil
}
