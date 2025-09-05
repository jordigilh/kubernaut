//go:build integration
// +build integration

package shared

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jordigilh/kubernaut/pkg/infrastructure/types"
)

// FakeSLMClient provides a comprehensive fake SLM client for testing
type FakeSLMClient struct {
	responses     map[string]*types.ActionRecommendation
	chatResponses map[string]string
	healthStatus  bool
	callHistory   []FakeSLMCall
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

type FakeSLMCall struct {
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

// NewFakeSLMClient creates a new fake SLM client with realistic behavior
func NewFakeSLMClient() *FakeSLMClient {
	client := &FakeSLMClient{
		responses:            make(map[string]*types.ActionRecommendation),
		chatResponses:        make(map[string]string),
		healthStatus:         true,
		callHistory:          make([]FakeSLMCall, 0),
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

// NewFakeSLMClientWithScenario creates a fake client configured for a specific testing scenario
func NewFakeSLMClientWithScenario(scenario RealisticScenario) *FakeSLMClient {
	client := NewFakeSLMClient()
	client.ConfigureForScenario(scenario)
	return client
}

// setupDefaultResponses configures realistic responses for common scenarios
func (f *FakeSLMClient) setupDefaultResponses() {
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
func (f *FakeSLMClient) SetHealthStatus(healthy bool) {
	f.healthStatus = healthy
}

func (f *FakeSLMClient) SetLatency(latency time.Duration) {
	f.latency = latency
}

func (f *FakeSLMClient) SimulateError(message string) {
	f.simulateError = true
	f.errorMessage = message
}

func (f *FakeSLMClient) ClearError() {
	f.simulateError = false
	f.errorMessage = ""
}

func (f *FakeSLMClient) SetMaxCalls(maxCalls int) {
	f.maxCalls = maxCalls
}

func (f *FakeSLMClient) AddResponse(alertName string, response *types.ActionRecommendation) {
	f.responses[alertName] = response
}

func (f *FakeSLMClient) AddChatResponse(prompt string, response string) {
	f.chatResponses[prompt] = response
}

// Interface implementation
func (f *FakeSLMClient) AnalyzeAlert(ctx context.Context, alert types.Alert) (*types.ActionRecommendation, error) {
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
	f.callHistory = append(f.callHistory, FakeSLMCall{
		Method:    "AnalyzeAlert",
		Alert:     &alert,
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
			return nil, errors.New("network failure simulation")
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

	// Return configured response or default
	if response, exists := f.responses[alert.Name]; exists {
		// Clone the response to avoid mutations affecting other tests
		cloned := *response
		if response.Reasoning != nil {
			reasoningClone := *response.Reasoning
			cloned.Reasoning = &reasoningClone
		}
		return &cloned, nil
	}

	// Generate default response based on alert characteristics
	if f.useEnhancedDecisions && f.decisionEngine != nil {
		return f.generateEnhancedResponse(alert), nil
	}
	return f.generateDefaultResponse(alert), nil
}

func (f *FakeSLMClient) ChatCompletion(ctx context.Context, prompt string) (string, error) {
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
	f.callHistory = append(f.callHistory, FakeSLMCall{
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
			return "", errors.New("network failure simulation")
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

func (f *FakeSLMClient) IsHealthy() bool {
	return f.healthStatus
}

// Helper methods for testing
func (f *FakeSLMClient) GetCallHistory() []FakeSLMCall {
	return f.callHistory
}

func (f *FakeSLMClient) GetCallCount() int {
	return f.callCount
}

func (f *FakeSLMClient) ResetCallHistory() {
	f.callHistory = make([]FakeSLMCall, 0)
	f.callCount = 0
}

func (f *FakeSLMClient) generateDefaultResponse(alert types.Alert) *types.ActionRecommendation {
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
func (f *FakeSLMClient) generateEnhancedResponse(alert types.Alert) *types.ActionRecommendation {
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
func (f *FakeSLMClient) SetUseEnhancedDecisions(use bool) {
	f.useEnhancedDecisions = use
}

// GetDecisionEngine returns the decision engine for advanced configuration
func (f *FakeSLMClient) GetDecisionEngine() *SophisticatedDecisionEngine {
	return f.decisionEngine
}

// Enhanced Error Injection Methods as specified in TODO requirements

// ConfigureErrorInjection configures comprehensive error injection behavior
func (f *FakeSLMClient) ConfigureErrorInjection(config ErrorInjectionConfig) {
	f.errorInjection = config

	// Reset circuit breaker state if error injection is being reconfigured
	if config.Enabled {
		f.circuitBreaker = CircuitClosed
		f.failureCount = 0
		f.successCount = 0
	}
}

// TriggerErrorScenario triggers a specific error scenario for testing
func (f *FakeSLMClient) TriggerErrorScenario(scenario ErrorScenario) error {
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
func (f *FakeSLMClient) GetCircuitBreakerState() CircuitBreakerState {
	return f.circuitBreaker
}

// ResetErrorState resets all error injection and circuit breaker state
func (f *FakeSLMClient) ResetErrorState() {
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
func (f *FakeSLMClient) EnableCircuitBreaker(config CircuitBreakerConfig) {
	f.circuitBreakerConfig = config
	f.circuitBreakerConfig.Enabled = true
	f.circuitBreaker = CircuitClosed
}

// DisableCircuitBreaker disables circuit breaker functionality
func (f *FakeSLMClient) DisableCircuitBreaker() {
	f.circuitBreakerConfig.Enabled = false
	f.circuitBreaker = CircuitClosed
}

// Internal helper methods for error injection

// recoverFromScenario recovers from a specific error scenario
func (f *FakeSLMClient) recoverFromScenario(scenarioName string) {
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
func (f *FakeSLMClient) shouldInjectError() (bool, ErrorCategory) {
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
func (f *FakeSLMClient) updateCircuitBreakerState(success bool) {
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
func (f *FakeSLMClient) isCircuitBreakerOpen() bool {
	return f.circuitBreakerConfig.Enabled && f.circuitBreaker == CircuitOpen
}
