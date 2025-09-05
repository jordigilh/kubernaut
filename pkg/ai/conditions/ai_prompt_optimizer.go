package conditions

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
	"github.com/sirupsen/logrus"
)

// PromptVersion represents a versioned prompt with performance tracking
type PromptVersion struct {
	ID             string    `json:"id"`
	Version        string    `json:"version"`
	Name           string    `json:"name"`
	Description    string    `json:"description"`
	PromptTemplate string    `json:"prompt_template"`
	Requirements   []string  `json:"requirements"`
	OutputFormat   string    `json:"output_format"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	IsActive       bool      `json:"is_active"`

	// Performance metrics
	UsageCount     int64         `json:"usage_count"`
	SuccessRate    float64       `json:"success_rate"`
	AverageLatency time.Duration `json:"average_latency"`
	ErrorRate      float64       `json:"error_rate"`
	QualityScore   float64       `json:"quality_score"`
}

// PromptExperiment represents an A/B test experiment
type PromptExperiment struct {
	ID            string             `json:"id"`
	Name          string             `json:"name"`
	Description   string             `json:"description"`
	ControlPrompt string             `json:"control_prompt"`
	TestPrompts   []string           `json:"test_prompts"`
	TrafficSplit  map[string]float64 `json:"traffic_split"`
	StartTime     time.Time          `json:"start_time"`
	EndTime       *time.Time         `json:"end_time,omitempty"`
	Status        string             `json:"status"` // "running", "completed", "paused"

	// Results
	Results      map[string]*ExperimentResult `json:"results"`
	WinnerPrompt string                       `json:"winner_prompt,omitempty"`
	Confidence   float64                      `json:"confidence"`
}

// ExperimentResult tracks performance metrics for a prompt variant
type ExperimentResult struct {
	PromptID         string        `json:"prompt_id"`
	SampleSize       int64         `json:"sample_size"`
	SuccessRate      float64       `json:"success_rate"`
	AverageLatency   time.Duration `json:"average_latency"`
	ErrorRate        float64       `json:"error_rate"`
	QualityScore     float64       `json:"quality_score"`
	UserSatisfaction float64       `json:"user_satisfaction"`
}

// PromptOptimizer manages prompt versioning and A/B testing
type PromptOptimizer struct {
	versions     map[string]*PromptVersion
	experiments  map[string]*PromptExperiment
	activePrompt string
	mu           sync.RWMutex
	log          *logrus.Logger

	// Configuration
	config *PromptOptimizerConfig
}

// PromptOptimizerConfig holds configuration for prompt optimization
type PromptOptimizerConfig struct {
	EnableABTesting     bool          `yaml:"enable_ab_testing" default:"true"`
	MinSampleSize       int64         `yaml:"min_sample_size" default:"100"`
	ConfidenceThreshold float64       `yaml:"confidence_threshold" default:"0.95"`
	ExperimentDuration  time.Duration `yaml:"experiment_duration" default:"7d"`
	QualityThreshold    float64       `yaml:"quality_threshold" default:"0.8"`
	AutoPromoteWinners  bool          `yaml:"auto_promote_winners" default:"true"`
	MaxConcurrentTests  int           `yaml:"max_concurrent_tests" default:"3"`
}

// NewPromptOptimizer creates a new prompt optimizer
func NewPromptOptimizer(config *PromptOptimizerConfig, log *logrus.Logger) *PromptOptimizer {
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

	return &PromptOptimizer{
		versions:    make(map[string]*PromptVersion),
		experiments: make(map[string]*PromptExperiment),
		config:      config,
		log:         log,
	}
}

// RegisterPromptVersion registers a new prompt version
func (po *PromptOptimizer) RegisterPromptVersion(version *PromptVersion) error {
	po.mu.Lock()
	defer po.mu.Unlock()

	if version.ID == "" {
		return fmt.Errorf("prompt version ID cannot be empty")
	}

	version.CreatedAt = time.Now()
	version.UpdatedAt = time.Now()

	po.versions[version.ID] = version

	// Set as active if it's the first version or explicitly marked as active
	if len(po.versions) == 1 || version.IsActive {
		po.activePrompt = version.ID
	}

	po.log.WithFields(logrus.Fields{
		"prompt_id":      version.ID,
		"prompt_version": version.Version,
		"is_active":      version.IsActive,
	}).Info("Registered new prompt version")

	return nil
}

// GetOptimalPrompt returns the best prompt based on current experiments or active version
func (po *PromptOptimizer) GetOptimalPrompt(ctx context.Context, objective *engine.WorkflowObjective) (*PromptVersion, error) {
	po.mu.RLock()
	defer po.mu.RUnlock()

	// Check if we're in an A/B test
	if po.config.EnableABTesting {
		for _, experiment := range po.experiments {
			if experiment.Status == "running" {
				selectedPromptID := po.selectPromptFromExperiment(experiment)
				if version, exists := po.versions[selectedPromptID]; exists {
					return version, nil
				}
			}
		}
	}

	// Fall back to active prompt
	if activeVersion, exists := po.versions[po.activePrompt]; exists {
		return activeVersion, nil
	}

	// Fall back to best performing prompt
	return po.getBestPerformingPrompt(), nil
}

// selectPromptFromExperiment selects a prompt variant based on traffic split
func (po *PromptOptimizer) selectPromptFromExperiment(experiment *PromptExperiment) string {
	random := rand.Float64()
	cumulative := 0.0

	for promptID, weight := range experiment.TrafficSplit {
		cumulative += weight
		if random <= cumulative {
			return promptID
		}
	}

	// Fallback to control prompt
	return experiment.ControlPrompt
}

// getBestPerformingPrompt returns the prompt with the highest quality score
func (po *PromptOptimizer) getBestPerformingPrompt() *PromptVersion {
	var bestPrompt *PromptVersion
	highestScore := 0.0

	for _, version := range po.versions {
		if version.QualityScore > highestScore {
			highestScore = version.QualityScore
			bestPrompt = version
		}
	}

	return bestPrompt
}

// StartABTest starts a new A/B test experiment
func (po *PromptOptimizer) StartABTest(experiment *PromptExperiment) error {
	po.mu.Lock()
	defer po.mu.Unlock()

	if !po.config.EnableABTesting {
		return fmt.Errorf("A/B testing is disabled")
	}

	// Check if we're already running max concurrent tests
	runningTests := 0
	for _, exp := range po.experiments {
		if exp.Status == "running" {
			runningTests++
		}
	}

	if runningTests >= po.config.MaxConcurrentTests {
		return fmt.Errorf("maximum concurrent tests (%d) already running", po.config.MaxConcurrentTests)
	}

	experiment.ID = fmt.Sprintf("exp-%d", time.Now().Unix())
	experiment.StartTime = time.Now()
	experiment.Status = "running"
	experiment.Results = make(map[string]*ExperimentResult)

	// Initialize results for each prompt variant
	for promptID := range experiment.TrafficSplit {
		experiment.Results[promptID] = &ExperimentResult{
			PromptID: promptID,
		}
	}

	po.experiments[experiment.ID] = experiment

	po.log.WithFields(logrus.Fields{
		"experiment_id":   experiment.ID,
		"experiment_name": experiment.Name,
		"control_prompt":  experiment.ControlPrompt,
		"test_prompts":    experiment.TestPrompts,
	}).Info("Started A/B test experiment")

	return nil
}

// RecordPromptMetrics records performance metrics for a prompt usage
func (po *PromptOptimizer) RecordPromptMetrics(promptID string, success bool, latency time.Duration, qualityScore float64) {
	po.mu.Lock()
	defer po.mu.Unlock()

	version, exists := po.versions[promptID]
	if !exists {
		return
	}

	// Update version metrics
	version.UsageCount++

	// Update success rate using exponential moving average
	alpha := 2.0 / (float64(version.UsageCount) + 1.0)
	if success {
		version.SuccessRate = version.SuccessRate*(1-alpha) + alpha
	} else {
		version.SuccessRate = version.SuccessRate * (1 - alpha)
		version.ErrorRate = version.ErrorRate*(1-alpha) + alpha
	}

	// Update average latency
	if version.UsageCount == 1 {
		version.AverageLatency = latency
	} else {
		version.AverageLatency = time.Duration(float64(version.AverageLatency)*(1-alpha) + float64(latency)*alpha)
	}

	// Update quality score
	if qualityScore > 0 {
		if version.UsageCount == 1 {
			version.QualityScore = qualityScore
		} else {
			version.QualityScore = version.QualityScore*(1-alpha) + qualityScore*alpha
		}
	}

	version.UpdatedAt = time.Now()

	// Update experiment results if applicable
	for _, experiment := range po.experiments {
		if experiment.Status == "running" {
			if result, exists := experiment.Results[promptID]; exists {
				result.SampleSize++
				if success {
					result.SuccessRate = result.SuccessRate*(1-alpha) + alpha
				} else {
					result.SuccessRate = result.SuccessRate * (1 - alpha)
					result.ErrorRate = result.ErrorRate*(1-alpha) + alpha
				}

				if result.SampleSize == 1 {
					result.AverageLatency = latency
				} else {
					result.AverageLatency = time.Duration(float64(result.AverageLatency)*(1-alpha) + float64(latency)*alpha)
				}

				if qualityScore > 0 {
					if result.SampleSize == 1 {
						result.QualityScore = qualityScore
					} else {
						result.QualityScore = result.QualityScore*(1-alpha) + qualityScore*alpha
					}
				}
			}
		}
	}
}

// EvaluateExperiments evaluates running experiments and promotes winners if applicable
func (po *PromptOptimizer) EvaluateExperiments() {
	po.mu.Lock()
	defer po.mu.Unlock()

	for experimentID, experiment := range po.experiments {
		if experiment.Status != "running" {
			continue
		}

		// Check if experiment has run long enough
		if time.Since(experiment.StartTime) < po.config.ExperimentDuration {
			continue
		}

		// Check if we have enough samples
		totalSamples := int64(0)
		for _, result := range experiment.Results {
			totalSamples += result.SampleSize
		}

		if totalSamples < po.config.MinSampleSize {
			continue
		}

		// Find the best performing variant
		bestResult := po.findBestResult(experiment.Results)
		if bestResult == nil {
			continue
		}

		// Calculate statistical confidence
		confidence := po.calculateStatisticalConfidence(experiment.Results, bestResult)
		experiment.Confidence = confidence

		if confidence >= po.config.ConfidenceThreshold && bestResult.QualityScore >= po.config.QualityThreshold {
			// We have a winner!
			experiment.WinnerPrompt = bestResult.PromptID
			experiment.Status = "completed"
			endTime := time.Now()
			experiment.EndTime = &endTime

			if po.config.AutoPromoteWinners {
				// Promote the winning prompt to active
				po.activePrompt = bestResult.PromptID
				if winnerVersion, exists := po.versions[bestResult.PromptID]; exists {
					winnerVersion.IsActive = true
					// Deactivate other versions
					for _, version := range po.versions {
						if version.ID != bestResult.PromptID {
							version.IsActive = false
						}
					}
				}
			}

			po.log.WithFields(logrus.Fields{
				"experiment_id": experimentID,
				"winner_prompt": experiment.WinnerPrompt,
				"confidence":    confidence,
				"quality_score": bestResult.QualityScore,
				"auto_promoted": po.config.AutoPromoteWinners,
			}).Info("A/B test experiment completed with winner")
		}
	}
}

// findBestResult finds the best performing result from experiment results
func (po *PromptOptimizer) findBestResult(results map[string]*ExperimentResult) *ExperimentResult {
	var bestResult *ExperimentResult
	bestScore := 0.0

	for _, result := range results {
		// Composite score: weighted combination of success rate, quality, and inverse latency
		score := result.SuccessRate*0.4 + result.QualityScore*0.4 + (1.0-result.ErrorRate)*0.2

		if score > bestScore {
			bestScore = score
			bestResult = result
		}
	}

	return bestResult
}

// calculateStatisticalConfidence calculates statistical confidence for the best result
func (po *PromptOptimizer) calculateStatisticalConfidence(results map[string]*ExperimentResult, bestResult *ExperimentResult) float64 {
	// Simplified confidence calculation based on sample size and performance difference
	// In production, this would use proper statistical significance testing

	if bestResult.SampleSize < 30 {
		return 0.0 // Not enough samples for confidence
	}

	// Find second-best result
	var secondBest *ExperimentResult
	secondBestScore := 0.0

	for _, result := range results {
		if result.PromptID == bestResult.PromptID {
			continue
		}

		score := result.SuccessRate*0.4 + result.QualityScore*0.4 + (1.0-result.ErrorRate)*0.2
		if score > secondBestScore {
			secondBestScore = score
			secondBest = result
		}
	}

	if secondBest == nil {
		return 1.0 // Only one variant, full confidence
	}

	bestScore := bestResult.SuccessRate*0.4 + bestResult.QualityScore*0.4 + (1.0-bestResult.ErrorRate)*0.2

	// Simple confidence based on performance gap and sample size
	performanceGap := bestScore - secondBestScore
	sampleFactor := float64(bestResult.SampleSize) / 100.0 // Normalize to 100 samples

	confidence := performanceGap * sampleFactor
	if confidence > 1.0 {
		confidence = 1.0
	}

	return confidence
}

// GetPromptStatistics returns comprehensive statistics for all prompts
func (po *PromptOptimizer) GetPromptStatistics() map[string]*PromptVersion {
	po.mu.RLock()
	defer po.mu.RUnlock()

	stats := make(map[string]*PromptVersion)
	for id, version := range po.versions {
		stats[id] = &PromptVersion{
			ID:             version.ID,
			Version:        version.Version,
			Name:           version.Name,
			UsageCount:     version.UsageCount,
			SuccessRate:    version.SuccessRate,
			AverageLatency: version.AverageLatency,
			ErrorRate:      version.ErrorRate,
			QualityScore:   version.QualityScore,
			IsActive:       version.IsActive,
		}
	}

	return stats
}

// GetRunningExperiments returns all currently running experiments
func (po *PromptOptimizer) GetRunningExperiments() []*PromptExperiment {
	po.mu.RLock()
	defer po.mu.RUnlock()

	var running []*PromptExperiment
	for _, experiment := range po.experiments {
		if experiment.Status == "running" {
			running = append(running, experiment)
		}
	}

	return running
}
