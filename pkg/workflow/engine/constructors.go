package engine

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// Constructor functions for core types
func NewWorkflowTemplate(id, name string) *WorkflowTemplate {
	return &WorkflowTemplate{
		ID:         id,
		Name:       name,
		Steps:      []*WorkflowStep{},
		Conditions: []*WorkflowCondition{},
		Variables:  make(map[string]interface{}),
		Tags:       []string{},
	}
}

func NewWorkflow(id string, template *WorkflowTemplate) *Workflow {
	return &Workflow{
		ID:       id,
		Template: template,
		Status:   StatusPending,
		Metadata: make(map[string]interface{}),
	}
}

func NewWorkflowExecution(id, workflowID string) *WorkflowExecution {
	return &WorkflowExecution{
		ID:         id,
		WorkflowID: workflowID,
		Status:     ExecutionStatusPending,
		Steps:      []*StepExecution{},
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

// Interface constructor functions
func NewPromptOptimizer() PromptOptimizer {
	return NewRealPromptOptimizer(nil, logrus.StandardLogger())
}

// Stub implementations for other interfaces

// Missing constructor functions for missing types
func NewAIMetricsCollector() AIMetricsCollector {
	return &StubAIMetricsCollector{}
}

func NewLearningEnhancedPromptBuilder() LearningEnhancedPromptBuilder {
	return &StubLearningEnhancedPromptBuilder{}
}

// Stub implementations
type StubAIMetricsCollector struct{}

func (s *StubAIMetricsCollector) CollectMetrics(ctx context.Context, execution *WorkflowExecution) (map[string]float64, error) {
	return make(map[string]float64), nil
}

func (s *StubAIMetricsCollector) GetAggregatedMetrics(ctx context.Context, workflowID string, timeRange TimeRange) (map[string]float64, error) {
	return make(map[string]float64), nil
}

type StubLearningEnhancedPromptBuilder struct{}

func (s *StubLearningEnhancedPromptBuilder) BuildPrompt(ctx context.Context, template string, context map[string]interface{}) (string, error) {
	return template, nil
}

func (s *StubLearningEnhancedPromptBuilder) GetLearnFromExecution(ctx context.Context, execution *WorkflowExecution) error {
	return nil
}

func (s *StubLearningEnhancedPromptBuilder) GetGetOptimizedTemplate(ctx context.Context, templateID string) (string, error) {
	return "", nil
}

// Fix missing AIMetricsCollector methods
func (s *StubAIMetricsCollector) RecordAIRequest(ctx context.Context, requestID string, prompt string, response string) error {
	return nil
}

// Add missing methods to StubAIMetricsCollector
func (s *StubAIMetricsCollector) EvaluateResponseQuality(ctx context.Context, response string, context map[string]interface{}) (*AIResponseQuality, error) {
	return &AIResponseQuality{
		Score:      0.8,
		Confidence: 0.9,
		Relevance:  0.85,
		Clarity:    0.9,
	}, nil
}

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

// Add missing method to LearningEnhancedPromptBuilder stub
func (s *StubLearningEnhancedPromptBuilder) GetBuildEnhancedPrompt(ctx context.Context, basePrompt string, context map[string]interface{}) (string, error) {
	return basePrompt, nil
}

// DefaultIntelligentWorkflowBuilder methods are declared in intelligent_workflow_builder_types.go
