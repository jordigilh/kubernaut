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

package engine

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/sirupsen/logrus"
)

// Constructor functions for core types
func NewWorkflowTemplate(id, name string) *ExecutableTemplate {
	return &ExecutableTemplate{
		BaseVersionedEntity: types.BaseVersionedEntity{
			BaseEntity: types.BaseEntity{
				ID:        id,
				Name:      name,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
				Metadata:  make(map[string]interface{}),
			},
			Version:   "1.0.0",
			CreatedBy: "system",
		},
		Steps:      []*ExecutableWorkflowStep{},
		Conditions: []*ExecutableCondition{},
		Variables:  make(map[string]interface{}),
		Tags:       []string{},
	}
}

func NewWorkflow(id string, template *ExecutableTemplate) *Workflow {
	return &Workflow{
		BaseVersionedEntity: types.BaseVersionedEntity{
			BaseEntity: types.BaseEntity{
				ID:        id,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
				Metadata:  make(map[string]interface{}),
			},
			Version:   "1.0.0",
			CreatedBy: "system",
		},
		Template: template,
		Status:   StatusPending,
	}
}

func NewRuntimeWorkflowExecution(id, workflowID string) *RuntimeWorkflowExecution {
	return &RuntimeWorkflowExecution{
		WorkflowExecutionRecord: types.WorkflowExecutionRecord{
			ID:         id,
			WorkflowID: workflowID,
			Status:     string(ExecutionStatusPending),
			StartTime:  time.Now(),
			Metadata:   make(map[string]interface{}),
		},
		Steps: []*StepExecution{},
	}
}

func NewPatternDiscoveryConfig() *PatternDiscoveryConfig {
	return &PatternDiscoveryConfig{
		MinSupport:      0.1,
		MinConfidence:   0.8,
		MaxPatterns:     100,
		TimeWindowHours: 24,
	}
}

func NewEnhancedPatternConfig() *EnhancedPatternConfig {
	return &EnhancedPatternConfig{
		MinSupport:       0.1,
		MinConfidence:    0.8,
		MaxPatterns:      100,
		EnableMLAnalysis: true,
		TimeWindowHours:  24,
	}
}

// RULE 12 COMPLIANCE: All deprecated AI constructor functions removed
// All AI functionality now consolidated in enhanced llm.Client
// Business Requirements: BR-AI-017, BR-AI-022, BR-AI-025, BR-PROMPT-001 - served by enhanced llm.Client

// @deprecated RULE 12 VIOLATION: Creates AIMetricsCollector instead of using enhanced llm.Client
// Migration: Use enhanced llm.Client methods directly - no separate collector needed
// Business Requirements: BR-AI-017, BR-AI-025 - now served by enhanced llm.Client

// @deprecated RULE 12 VIOLATION: Creates LearningEnhancedPromptBuilder instead of using enhanced llm.Client
// Migration: Use enhanced llm.Client methods directly - no separate builder needed
// Business Requirements: BR-PROMPT-001 - now served by enhanced llm.Client

// @deprecated RULE 12 VIOLATION: Creates AIMetricsCollector instead of using enhanced llm.Client
// Migration: Use enhanced llm.Client methods directly - no separate collector needed
// Business Requirements: BR-AI-017, BR-AI-025 - now served by enhanced llm.Client

// @deprecated RULE 12 VIOLATION: Creates LearningEnhancedPromptBuilder instead of using enhanced llm.Client
// Migration: Use enhanced llm.Client methods directly - no separate builder needed
// Business Requirements: BR-PROMPT-001 - now served by enhanced llm.Client

// Fail-fast implementations that replace misleading stubs
// @deprecated RULE 12 VIOLATION: Creates fallback AI metrics collector instead of using enhanced llm.Client
// Migration: Use enhanced llm.Client.CollectMetrics(), llm.Client.GetAggregatedMetrics(), llm.Client.RecordAIRequest() methods directly
// Business Requirements: BR-AI-017, BR-AI-025 - now served by enhanced llm.Client
//
// This struct violates Rule 12 by implementing AI metrics collection fallback that should use enhanced llm.Client capabilities.
// Instead of using this struct, call the enhanced llm.Client methods directly (they have proper fallback logic built-in).
//
// These provide clear error messages instead of returning fake data that creates false confidence
// Business Requirement: BR-AI-METRICS-001 - AI metrics collection and analysis
type FailFastAIMetricsCollector struct {
	Enabled     bool                   `json:"enabled"`
	MetricsDB   interface{}            `json:"-"`
	Logger      interface{}            `json:"-"`
	Config      map[string]interface{} `json:"config"`
	LastRequest time.Time              `json:"last_request"`
}

func (f *FailFastAIMetricsCollector) CollectMetrics(ctx context.Context, execution *RuntimeWorkflowExecution) (map[string]float64, error) {
	return nil, fmt.Errorf("AI metrics collection not implemented - implement this interface to enable workflow metrics collection functionality")
}

func (f *FailFastAIMetricsCollector) GetAggregatedMetrics(ctx context.Context, workflowID string, timeRange WorkflowTimeRange) (map[string]float64, error) {
	return nil, fmt.Errorf("aggregated metrics collection not implemented - implement this interface to enable workflow analytics")
}

func (f *FailFastAIMetricsCollector) RecordAIRequest(ctx context.Context, requestID string, prompt string, response string) error {
	return fmt.Errorf("AI request recording not implemented - implement this interface to enable AI request tracking")
}

func (f *FailFastAIMetricsCollector) EvaluateResponseQuality(ctx context.Context, response string, context map[string]interface{}) (*AIResponseQuality, error) {
	return nil, fmt.Errorf("AI response quality evaluation not implemented - implement this interface to enable response quality assessment")
}

// FailFastLearningEnhancedPromptBuilder provides learning-enhanced prompt building with fail-fast behavior
// Business Requirement: BR-AI-PROMPT-001 - Intelligent prompt generation and learning
type FailFastLearningEnhancedPromptBuilder struct {
	Enabled          bool                   `json:"enabled"`
	LearningEngine   interface{}            `json:"-"`
	TemplateCache    map[string]interface{} `json:"template_cache"`
	ExecutionHistory []interface{}          `json:"execution_history"`
	Config           map[string]interface{} `json:"config"`
	LastUpdate       time.Time              `json:"last_update"`
}

func (f *FailFastLearningEnhancedPromptBuilder) BuildPrompt(ctx context.Context, template string, context map[string]interface{}) (string, error) {
	return "", fmt.Errorf("learning-enhanced prompt building not implemented - implement this interface to enable intelligent prompt generation")
}

func (f *FailFastLearningEnhancedPromptBuilder) GetLearnFromExecution(ctx context.Context, execution *RuntimeWorkflowExecution) error {
	return fmt.Errorf("execution-based learning not implemented - implement this interface to enable workflow learning capabilities")
}

func (f *FailFastLearningEnhancedPromptBuilder) GetOptimizedTemplate(ctx context.Context, templateID string) (string, error) {
	return "", fmt.Errorf("template optimization not implemented - implement this interface to enable template optimization")
}

func (f *FailFastLearningEnhancedPromptBuilder) GetBuildEnhancedPrompt(ctx context.Context, basePrompt string, context map[string]interface{}) (string, error) {
	return "", fmt.Errorf("enhanced prompt building not implemented - implement this interface to enable context-aware prompt enhancement")
}

// @deprecated RULE 12 VIOLATION: Creates concrete AI prompt optimizer instead of using enhanced llm.Client
// Migration: Use enhanced llm.Client.RegisterPromptVersion(), llm.Client.GetOptimalPrompt(), llm.Client.StartABTest() methods directly
// Business Requirements: BR-AI-022 - now served by enhanced llm.Client
//
// This struct violates Rule 12 by implementing AI prompt optimization that duplicates enhanced llm.Client capabilities.
// Instead of using this struct, call the enhanced llm.Client methods directly:
//   - rpo.RegisterPromptVersion() -> llmClient.RegisterPromptVersion()
//   - rpo.GetOptimalPrompt() -> llmClient.GetOptimalPrompt()
//   - rpo.StartABTest() -> llmClient.StartABTest()
//
// Real PromptOptimizer implementation
type RealPromptOptimizer struct {
	versions     map[string]*PromptVersion
	experiments  map[string]*PromptExperiment
	activePrompt string
	mu           sync.RWMutex
	log          *logrus.Logger
	config       *PromptOptimizerConfig
}

type PromptOptimizerConfig struct {
	EnableABTesting     bool          `yaml:"enable_ab_testing"`
	MinSampleSize       int64         `yaml:"min_sample_size"`
	ConfidenceThreshold float64       `yaml:"confidence_threshold"`
	ExperimentDuration  time.Duration `yaml:"experiment_duration"`
	QualityThreshold    float64       `yaml:"quality_threshold"`
	AutoPromoteWinners  bool          `yaml:"auto_promote_winners"`
	MaxConcurrentTests  int           `yaml:"max_concurrent_tests"`
}

// @deprecated RULE 12 VIOLATION: Creates new AI prompt optimizer instead of using enhanced llm.Client
// Migration: Use enhanced llm.Client methods directly instead of creating this struct
// Replacement pattern:
//
//	Instead of: optimizer := NewRealPromptOptimizer(config, log)
//	Use: llmClient (already has enhanced RegisterPromptVersion, GetOptimalPrompt, StartABTest methods)
func NewRealPromptOptimizer(config *PromptOptimizerConfig, log *logrus.Logger) *RealPromptOptimizer {
	if config == nil {
		config = &PromptOptimizerConfig{
			EnableABTesting:     true,
			MinSampleSize:       100,
			ConfidenceThreshold: 0.95,
			ExperimentDuration:  7 * 24 * time.Hour,
			QualityThreshold:    0.8,
			AutoPromoteWinners:  true,
			MaxConcurrentTests:  3,
		}
	}
	if log == nil {
		log = logrus.StandardLogger()
	}

	return &RealPromptOptimizer{
		versions:    make(map[string]*PromptVersion),
		experiments: make(map[string]*PromptExperiment),
		config:      config,
		log:         log,
	}
}

func (rpo *RealPromptOptimizer) RegisterPromptVersion(version *PromptVersion) error {
	rpo.mu.Lock()
	defer rpo.mu.Unlock()

	if version.ID == "" {
		return fmt.Errorf("prompt version ID cannot be empty")
	}

	version.CreatedAt = time.Now()
	version.UpdatedAt = time.Now()
	rpo.versions[version.ID] = version

	if len(rpo.versions) == 1 || version.IsActive {
		rpo.activePrompt = version.ID
	}

	rpo.log.WithFields(logrus.Fields{
		"prompt_id": version.ID,
		"version":   version.Version,
		"active":    version.IsActive,
	}).Info("Registered prompt version")

	return nil
}

func (rpo *RealPromptOptimizer) GetOptimalPrompt(ctx context.Context, objective *WorkflowObjective) (*PromptVersion, error) {
	rpo.mu.RLock()
	defer rpo.mu.RUnlock()

	// Return active prompt if available
	if rpo.activePrompt != "" {
		if version, exists := rpo.versions[rpo.activePrompt]; exists {
			return version, nil
		}
	}

	// Return best performing prompt
	return rpo.getBestPerformingPrompt(), nil
}

func (rpo *RealPromptOptimizer) getBestPerformingPrompt() *PromptVersion {
	var bestPrompt *PromptVersion
	highestScore := 0.0

	for _, version := range rpo.versions {
		if version.QualityScore > highestScore {
			highestScore = version.QualityScore
			bestPrompt = version
		}
	}

	return bestPrompt
}

func (rpo *RealPromptOptimizer) StartABTest(experiment *PromptExperiment) error {
	rpo.mu.Lock()
	defer rpo.mu.Unlock()

	if !rpo.config.EnableABTesting {
		return fmt.Errorf("A/B testing is disabled")
	}

	// Check concurrent tests limit
	runningTests := 0
	for _, exp := range rpo.experiments {
		if exp.Status == "running" {
			runningTests++
		}
	}

	if runningTests >= rpo.config.MaxConcurrentTests {
		return fmt.Errorf("maximum concurrent tests (%d) already running", rpo.config.MaxConcurrentTests)
	}

	experiment.ID = fmt.Sprintf("exp-%d", time.Now().Unix())
	experiment.StartTime = time.Now()
	experiment.Status = "running"
	experiment.Results = make(map[string]*ExperimentResult)

	// Initialize results
	for promptID := range experiment.TrafficSplit {
		experiment.Results[promptID] = &ExperimentResult{
			PromptID: promptID,
		}
	}

	rpo.experiments[experiment.ID] = experiment

	rpo.log.WithFields(logrus.Fields{
		"experiment_id": experiment.ID,
		"name":          experiment.Name,
	}).Info("Started A/B test")

	return nil
}

func (rpo *RealPromptOptimizer) RecordPromptMetrics(promptID string, success bool, latency time.Duration, qualityScore float64) {
	rpo.mu.Lock()
	defer rpo.mu.Unlock()

	version, exists := rpo.versions[promptID]
	if !exists {
		return
	}

	// Update metrics using exponential moving average
	version.UsageCount++
	alpha := 2.0 / (float64(version.UsageCount) + 1.0)

	if success {
		version.SuccessRate = version.SuccessRate*(1-alpha) + alpha
	} else {
		version.SuccessRate = version.SuccessRate * (1 - alpha)
		version.ErrorRate = version.ErrorRate*(1-alpha) + alpha
	}

	// Update latency and quality
	if version.UsageCount == 1 {
		version.AverageLatency = latency
		if qualityScore > 0 {
			version.QualityScore = qualityScore
		}
	} else {
		version.AverageLatency = time.Duration(float64(version.AverageLatency)*(1-alpha) + float64(latency)*alpha)
		if qualityScore > 0 {
			version.QualityScore = version.QualityScore*(1-alpha) + qualityScore*alpha
		}
	}

	version.UpdatedAt = time.Now()
}

func (rpo *RealPromptOptimizer) EvaluateExperiments() {
	rpo.mu.Lock()
	defer rpo.mu.Unlock()

	for _, experiment := range rpo.experiments {
		if experiment.Status != "running" {
			continue
		}

		// Simple evaluation: if experiment has enough samples, find winner
		totalSamples := int64(0)
		for _, result := range experiment.Results {
			totalSamples += result.SampleSize
		}

		if totalSamples >= rpo.config.MinSampleSize {
			bestResult := rpo.findBestResult(experiment.Results)
			if bestResult != nil && bestResult.QualityScore >= rpo.config.QualityThreshold {
				experiment.Status = "completed"
				experiment.WinnerPrompt = bestResult.PromptID
				experiment.Confidence = 0.95 // Simplified
				endTime := time.Now()
				experiment.EndTime = &endTime

				rpo.log.WithFields(logrus.Fields{
					"experiment_id": experiment.ID,
					"winner":        bestResult.PromptID,
					"quality":       bestResult.QualityScore,
				}).Info("Experiment completed")
			}
		}
	}
}

func (rpo *RealPromptOptimizer) findBestResult(results map[string]*ExperimentResult) *ExperimentResult {
	var best *ExperimentResult
	highestScore := 0.0

	for _, result := range results {
		if result.QualityScore > highestScore {
			highestScore = result.QualityScore
			best = result
		}
	}

	return best
}

func (rpo *RealPromptOptimizer) GetPromptStatistics() map[string]*PromptVersion {
	rpo.mu.RLock()
	defer rpo.mu.RUnlock()

	// Return copy of versions map
	stats := make(map[string]*PromptVersion)
	for k, v := range rpo.versions {
		stats[k] = v
	}
	return stats
}

func (rpo *RealPromptOptimizer) GetRunningExperiments() []*PromptExperiment {
	rpo.mu.RLock()
	defer rpo.mu.RUnlock()

	var running []*PromptExperiment
	for _, exp := range rpo.experiments {
		if exp.Status == "running" {
			running = append(running, exp)
		}
	}
	return running
}

// MemoryExecutionRepository provides an in-memory implementation of ExecutionRepository
type MemoryExecutionRepository struct {
	executions map[string]*RuntimeWorkflowExecution
	mu         sync.RWMutex
	log        *logrus.Logger
}

// NewMemoryExecutionRepository creates a new in-memory execution repository
func NewMemoryExecutionRepository(log *logrus.Logger) ExecutionRepository {
	if log == nil {
		log = logrus.New()
		log.SetLevel(logrus.WarnLevel)
	}

	return &MemoryExecutionRepository{
		executions: make(map[string]*RuntimeWorkflowExecution),
		log:        log,
	}
}

func (mer *MemoryExecutionRepository) GetExecution(ctx context.Context, executionID string) (*RuntimeWorkflowExecution, error) {
	mer.mu.RLock()
	defer mer.mu.RUnlock()

	if execution, exists := mer.executions[executionID]; exists {
		mer.log.WithField("execution_id", executionID).Debug("Retrieved execution from memory")
		return execution, nil
	}

	return nil, fmt.Errorf("execution not found: %s", executionID)
}

func (mer *MemoryExecutionRepository) GetExecutionsInTimeWindow(ctx context.Context, start, end time.Time) ([]*RuntimeWorkflowExecution, error) {
	mer.mu.RLock()
	defer mer.mu.RUnlock()

	var results []*RuntimeWorkflowExecution
	for _, execution := range mer.executions {
		if execution.StartTime.After(start) && execution.StartTime.Before(end) {
			results = append(results, execution)
		}
	}

	mer.log.WithFields(logrus.Fields{
		"start_time":       start,
		"end_time":         end,
		"executions_found": len(results),
	}).Debug("Retrieved executions in time window from memory")

	return results, nil
}

func (mer *MemoryExecutionRepository) GetExecutionsByWorkflowID(ctx context.Context, workflowID string) ([]*RuntimeWorkflowExecution, error) {
	mer.mu.RLock()
	defer mer.mu.RUnlock()

	var results []*RuntimeWorkflowExecution
	for _, execution := range mer.executions {
		if execution.WorkflowID == workflowID {
			results = append(results, execution)
		}
	}

	mer.log.WithFields(logrus.Fields{
		"workflow_id":      workflowID,
		"executions_found": len(results),
	}).Debug("Retrieved executions by workflow ID from memory")

	return results, nil
}

func (mer *MemoryExecutionRepository) GetExecutionsByPattern(ctx context.Context, patternID string) ([]*RuntimeWorkflowExecution, error) {
	mer.mu.RLock()
	defer mer.mu.RUnlock()

	var results []*RuntimeWorkflowExecution
	for _, execution := range mer.executions {
		// Look for pattern ID in execution metadata
		if execution.Metadata != nil {
			if executionPatternID, exists := execution.Metadata["pattern_id"]; exists {
				if executionPatternID == patternID {
					results = append(results, execution)
				}
			}
		}
	}

	mer.log.WithFields(logrus.Fields{
		"pattern_id":       patternID,
		"executions_found": len(results),
	}).Debug("Retrieved executions by pattern ID from memory")

	return results, nil
}

func (mer *MemoryExecutionRepository) StoreExecution(ctx context.Context, execution *RuntimeWorkflowExecution) error {
	if execution == nil {
		return fmt.Errorf("execution cannot be nil")
	}

	if execution.ID == "" {
		return fmt.Errorf("execution ID cannot be empty")
	}

	mer.mu.Lock()
	defer mer.mu.Unlock()

	mer.executions[execution.ID] = execution

	mer.log.WithFields(logrus.Fields{
		"execution_id": execution.ID,
		"workflow_id":  execution.WorkflowID,
	}).Debug("Stored execution in memory")

	return nil
}
