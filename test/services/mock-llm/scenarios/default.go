/*
Copyright 2026 Jordi Gil.

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
package scenarios

import (
	"regexp"
	"strings"

	"github.com/jordigilh/kubernaut/pkg/shared/uuid"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/conversation"
)

var reSignalName = regexp.MustCompile(`(?i)signal name:\s*(\S+)`)

// MockScenarioConfig holds the static configuration for a mock scenario.
type MockScenarioConfig struct {
	ScenarioName     string
	SignalName       string
	Severity         string
	WorkflowName     string
	WorkflowID       string
	WorkflowTitle    string
	Confidence       float64
	RootCause        string
	ResourceKind     string
	ResourceNS       string
	ResourceName     string
	APIVersion       string
	IncludeAffected  bool
	OverrideResource bool
	Parameters       map[string]string
	ExecutionEngine  string
	Contributing     []string
	NeedsHumanReview *bool
}

// configScenario is a Scenario backed by a static MockScenarioConfig and
// a MatchFunc that implements the detection logic.
type configScenario struct {
	config    MockScenarioConfig
	matchFunc func(ctx *DetectionContext) (bool, float64)
}

func (s *configScenario) Name() string { return s.config.ScenarioName }
func (s *configScenario) Match(ctx *DetectionContext) (bool, float64) {
	return s.matchFunc(ctx)
}
func (s *configScenario) Metadata() ScenarioMetadata {
	return ScenarioMetadata{
		Name:        s.config.ScenarioName,
		Description: s.config.RootCause,
	}
}
func (s *configScenario) DAG() *conversation.DAG { return nil }

// Config returns the underlying MockScenarioConfig for response building.
func (s *configScenario) Config() MockScenarioConfig { return s.config }

// ScenarioWithConfig is implemented by scenarios that expose their config.
type ScenarioWithConfig interface {
	Scenario
	Config() MockScenarioConfig
}

// DefaultRegistry returns a fully populated registry with all 15 scenarios
// and a default fallback, matching the Python MOCK_SCENARIOS catalog.
func DefaultRegistry() *Registry {
	r := NewRegistry()

	// Mock keyword scenarios (highest priority = 1.0)
	r.Register(mockKeywordScenario("no_workflow_found", "mock_no_workflow_found", noWorkflowFoundConfig()))
	r.Register(mockKeywordScenario("low_confidence", "mock_low_confidence", lowConfidenceConfig()))
	r.Register(mockKeywordScenario("problem_resolved_contradiction", "mock_problem_resolved_contradiction", problemResolvedContradictionConfig()))
	r.Register(mockKeywordScenario("problem_resolved", "mock_problem_resolved", problemResolvedConfig()))
	r.Register(mockKeywordScenarioMulti("problem_resolved", []string{"mock_not_reproducible", "mock not reproducible"}, problemResolvedConfig()))
	r.Register(mockKeywordScenario("rca_incomplete", "mock_rca_incomplete", rcaIncompleteConfig()))
	r.Register(mockKeywordScenario("max_retries_exhausted", "mock_max_retries_exhausted", maxRetriesExhaustedConfig()))

	// Test signal scenario
	r.Register(testSignalScenario())

	// Proactive scenarios (checked before signal-name)
	r.Register(predictiveNoActionScenario())
	r.Register(oomkilledPredictiveScenario())

	// Signal name scenarios
	r.Register(signalScenario("cert_not_ready", []string{"certmanagercertnotready", "cert_not_ready"}, certNotReadyConfig()))
	r.Register(signalScenario("node_not_ready", []string{"nodenotready"}, nodeNotReadyConfig()))
	r.Register(signalScenario("oomkilled", []string{"memoryexceedslimit", "memoryexceeds", "oomkilled", "oomkill"}, oomkilledConfig()))
	r.Register(signalScenario("crashloop", []string{"crashloop", "backoff"}, crashloopConfig()))

	// Default fallback (lowest priority = 0.01)
	r.Register(defaultFallbackScenario())

	return r
}

func mockKeywordScenario(name, keyword string, cfg MockScenarioConfig) *configScenario {
	return mockKeywordScenarioMulti(name, []string{keyword, strings.ReplaceAll(keyword, "_", " ")}, cfg)
}

func mockKeywordScenarioMulti(name string, keywords []string, cfg MockScenarioConfig) *configScenario {
	cfg.ScenarioName = name
	return &configScenario{
		config: cfg,
		matchFunc: func(ctx *DetectionContext) (bool, float64) {
			combined := strings.ToLower(ctx.Content + " " + ctx.AllText)
			for _, kw := range keywords {
				if strings.Contains(combined, kw) {
					return true, 1.0
				}
			}
			return false, 0
		},
	}
}

func testSignalScenario() *configScenario {
	cfg := testSignalConfig()
	return &configScenario{
		config: cfg,
		matchFunc: func(ctx *DetectionContext) (bool, float64) {
			lower := strings.ToLower(ctx.Content)
			if strings.Contains(lower, "testsignal") || strings.Contains(lower, "test signal") {
				return true, 0.95
			}
			return false, 0
		},
	}
}

func predictiveNoActionScenario() *configScenario {
	cfg := predictiveNoActionConfig()
	return &configScenario{
		config: cfg,
		matchFunc: func(ctx *DetectionContext) (bool, float64) {
			if !isProactive(ctx) {
				return false, 0
			}
			lower := strings.ToLower(ctx.Content + " " + ctx.AllText)
			if strings.Contains(lower, "predictive_no_action") || strings.Contains(lower, "mock_predictive_no_action") {
				return true, 0.98
			}
			return false, 0
		},
	}
}

func oomkilledPredictiveScenario() *configScenario {
	cfg := oomkilledPredictiveConfig()
	return &configScenario{
		config: cfg,
		matchFunc: func(ctx *DetectionContext) (bool, float64) {
			if !isProactive(ctx) {
				return false, 0
			}
			signal := extractSignal(ctx)
			if strings.Contains(signal, "oomkilled") || strings.Contains(signal, "oomkill") {
				return true, 0.96
			}
			return false, 0
		},
	}
}

func signalScenario(name string, patterns []string, cfg MockScenarioConfig) *configScenario {
	cfg.ScenarioName = name
	return &configScenario{
		config: cfg,
		matchFunc: func(ctx *DetectionContext) (bool, float64) {
			signal := extractSignal(ctx)
			if signal == "" {
				return false, 0
			}
			for _, p := range patterns {
				if strings.Contains(signal, p) {
					return true, 0.8
				}
			}
			return false, 0
		},
	}
}

func defaultFallbackScenario() *configScenario {
	cfg := defaultConfig()
	return &configScenario{
		config: cfg,
		matchFunc: func(_ *DetectionContext) (bool, float64) {
			return true, 0.01
		},
	}
}

func isProactive(ctx *DetectionContext) bool {
	if ctx.IsProactive {
		return true
	}
	lower := strings.ToLower(ctx.Content)
	return (strings.Contains(lower, "proactive mode") || strings.Contains(lower, "proactive signal")) ||
		(strings.Contains(lower, "predicted") && strings.Contains(lower, "not yet occurred"))
}

func extractSignal(ctx *DetectionContext) string {
	if ctx.SignalName != "" {
		return strings.ToLower(ctx.SignalName)
	}
	m := reSignalName.FindStringSubmatch(ctx.Content)
	if len(m) > 1 {
		return strings.ToLower(strings.TrimSpace(m[1]))
	}
	return ""
}

// Scenario config constructors matching Python MOCK_SCENARIOS values.

func oomkilledConfig() MockScenarioConfig {
	return MockScenarioConfig{
		ScenarioName: "oomkilled", SignalName: "OOMKilled", Severity: "critical",
		WorkflowName: "oomkill-increase-memory-v1", WorkflowID: uuid.DeterministicUUID("oomkill-increase-memory-v1"),
		WorkflowTitle: "OOMKill Recovery - Increase Memory Limits", Confidence: 0.95,
		RootCause: "Container exceeded memory limits due to traffic spike",
		ResourceKind: "Deployment", ResourceNS: "production", ResourceName: "api-server",
		Parameters: map[string]string{"MEMORY_LIMIT_NEW": "512Mi"}, ExecutionEngine: "job",
	}
}

func crashloopConfig() MockScenarioConfig {
	return MockScenarioConfig{
		ScenarioName: "crashloop", SignalName: "CrashLoopBackOff", Severity: "high",
		WorkflowName: "crashloop-config-fix-v1", WorkflowID: uuid.DeterministicUUID("crashloop-config-fix-v1"),
		WorkflowTitle: "CrashLoopBackOff - Configuration Fix", Confidence: 0.88,
		RootCause: "Container failing due to missing configuration",
		ResourceKind: "Deployment", ResourceNS: "staging", ResourceName: "worker",
		Parameters: map[string]string{"NAMESPACE": "staging", "DEPLOYMENT_NAME": "worker"},
	}
}

func nodeNotReadyConfig() MockScenarioConfig {
	return MockScenarioConfig{
		ScenarioName: "node_not_ready", SignalName: "NodeNotReady", Severity: "critical",
		WorkflowName: "node-drain-reboot-v1", WorkflowID: uuid.DeterministicUUID("node-drain-reboot-v1"),
		WorkflowTitle: "NodeNotReady - Drain and Reboot", Confidence: 0.90,
		RootCause: "Node experiencing disk pressure",
		ResourceKind: "Node", ResourceNS: "", ResourceName: "worker-node-1",
		Parameters: map[string]string{"NODE_NAME": "worker-node-1"},
	}
}

func testSignalConfig() MockScenarioConfig {
	return MockScenarioConfig{
		ScenarioName: "test_signal", SignalName: "TestSignal", Severity: "critical",
		WorkflowName: "test-signal-handler-v1", WorkflowID: uuid.DeterministicUUID("test-signal-handler-v1"),
		WorkflowTitle: "Test Signal Handler", Confidence: 0.90,
		RootCause: "Test signal for graceful shutdown validation",
		ResourceKind: "Pod", ResourceNS: "test", ResourceName: "test-pod",
		Parameters: map[string]string{"NAMESPACE": "test", "POD_NAME": "test-pod"},
	}
}

func noWorkflowFoundConfig() MockScenarioConfig {
	return MockScenarioConfig{
		ScenarioName: "no_workflow_found", SignalName: "MOCK_NO_WORKFLOW_FOUND", Severity: "critical",
		Confidence: 0.0,
		RootCause: "No suitable workflow found in catalog for this signal type",
		ResourceKind: "Pod", ResourceNS: "production", ResourceName: "failing-pod",
	}
}

func lowConfidenceConfig() MockScenarioConfig {
	return MockScenarioConfig{
		ScenarioName: "low_confidence", SignalName: "MOCK_LOW_CONFIDENCE", Severity: "critical",
		WorkflowName: "generic-restart-v1", WorkflowID: uuid.DeterministicUUID("generic-restart-v1"),
		WorkflowTitle: "Generic Pod Restart", Confidence: 0.35,
		RootCause: "Multiple possible root causes identified, requires human judgment",
		ResourceKind: "Pod", ResourceNS: "production", ResourceName: "ambiguous-pod",
		Parameters: map[string]string{"NAMESPACE": "production", "POD_NAME": "ambiguous-pod"},
	}
}

func problemResolvedConfig() MockScenarioConfig {
	return MockScenarioConfig{
		ScenarioName: "problem_resolved", SignalName: "MOCK_PROBLEM_RESOLVED", Severity: "low",
		Confidence: 0.85,
		RootCause: "Problem self-resolved through auto-scaling or transient condition cleared",
		ResourceKind: "Pod", ResourceNS: "production", ResourceName: "recovered-pod",
		Contributing: []string{"Transient condition", "Auto-recovery"},
	}
}

func problemResolvedContradictionConfig() MockScenarioConfig {
	b := true
	return MockScenarioConfig{
		ScenarioName: "problem_resolved_contradiction", SignalName: "MOCK_PROBLEM_RESOLVED_CONTRADICTION", Severity: "low",
		Confidence: 0.85,
		RootCause: "Problem self-resolved. Transient OOM cleared after pod restart",
		ResourceKind: "Pod", ResourceNS: "production", ResourceName: "recovered-pod",
		Contributing:     []string{"Transient condition", "Auto-recovery"},
		NeedsHumanReview: &b,
	}
}

func maxRetriesExhaustedConfig() MockScenarioConfig {
	return MockScenarioConfig{
		ScenarioName: "max_retries_exhausted", SignalName: "MOCK_MAX_RETRIES_EXHAUSTED", Severity: "high",
		Confidence: 0.0,
		RootCause:    "LLM analysis completed but failed validation after maximum retry attempts. Response format was unparseable or contained invalid data.",
		ResourceKind: "Pod", ResourceNS: "production", ResourceName: "failed-analysis-pod",
	}
}

func rcaIncompleteConfig() MockScenarioConfig {
	return MockScenarioConfig{
		ScenarioName: "rca_incomplete", SignalName: "MOCK_RCA_INCOMPLETE", Severity: "critical",
		WorkflowName: "generic-restart-v1", WorkflowID: uuid.DeterministicUUID("generic-restart-v1"),
		WorkflowTitle: "Generic Pod Restart", Confidence: 0.88,
		RootCause:    "Root cause identified but affected resource could not be determined from signal context",
		ResourceKind: "Pod", ResourceNS: "production", ResourceName: "ambiguous-pod",
		APIVersion: "v1",
		Parameters: map[string]string{"NAMESPACE": "production", "POD_NAME": "ambiguous-pod"},
	}
}

func oomkilledPredictiveConfig() MockScenarioConfig {
	return MockScenarioConfig{
		ScenarioName: "oomkilled_predictive", SignalName: "OOMKilled", Severity: "critical",
		WorkflowName: "oomkill-increase-memory-v1", WorkflowID: uuid.DeterministicUUID("oomkill-increase-memory-v1"),
		WorkflowTitle: "OOMKill Recovery - Increase Memory Limits", Confidence: 0.88,
		RootCause: "Predicted OOMKill based on memory utilization trend analysis (predict_linear). Current memory usage is 85% of limit and growing at 50MB/min. Preemptive action recommended to increase memory limits before the predicted OOMKill event occurs.",
		ResourceKind: "Deployment", ResourceNS: "production", ResourceName: "api-server",
		Parameters: map[string]string{"MEMORY_LIMIT_NEW": "512Mi"}, ExecutionEngine: "job",
	}
}

func predictiveNoActionConfig() MockScenarioConfig {
	return MockScenarioConfig{
		ScenarioName: "predictive_no_action", SignalName: "OOMKilled", Severity: "medium",
		Confidence: 0.82,
		RootCause:    "Predicted OOMKill based on trend analysis, but current assessment shows the trend is reversing. Memory usage has stabilized at 60% of limit after recent deployment rollout. No preemptive action needed — the prediction is unlikely to materialize.",
		ResourceKind: "Pod", ResourceNS: "production", ResourceName: "api-server-def456",
	}
}

func certNotReadyConfig() MockScenarioConfig {
	b := true
	_ = b
	return MockScenarioConfig{
		ScenarioName: "cert_not_ready", SignalName: "CertManagerCertNotReady", Severity: "critical",
		WorkflowName: "fix-certificate-v1", WorkflowID: uuid.DeterministicUUID("fix-certificate-v1"),
		WorkflowTitle: "Fix Certificate - Recreate CA Secret", Confidence: 0.92,
		RootCause:        "cert-manager Certificate stuck in NotReady state due to missing or corrupted CA Secret backing the ClusterIssuer",
		ResourceKind:     "Certificate", ResourceNS: "default", ResourceName: "demo-app-cert",
		APIVersion:       "cert-manager.io/v1",
		OverrideResource: true,
		Parameters: map[string]string{
			"TARGET_NAMESPACE":   "default",
			"TARGET_CERTIFICATE": "demo-app-cert",
			"ISSUER_NAME":       "demo-selfsigned-ca",
			"CA_SECRET_NAME":    "demo-ca-key-pair",
		},
		ExecutionEngine: "job",
	}
}

func defaultConfig() MockScenarioConfig {
	return MockScenarioConfig{
		ScenarioName: "default", SignalName: "Unknown", Severity: "medium",
		WorkflowName: "generic-restart-v1", WorkflowID: uuid.DeterministicUUID("generic-restart-v1"),
		WorkflowTitle: "Generic Pod Restart", Confidence: 0.75,
		RootCause:    "Unable to determine specific root cause",
		ResourceKind: "Pod", ResourceNS: "default", ResourceName: "test-pod",
		Contributing: []string{"traffic_spike", "resource_limits"},
	}
}
