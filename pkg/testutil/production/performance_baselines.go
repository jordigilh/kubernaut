<<<<<<< HEAD
=======
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

>>>>>>> crd_implementation
package production

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"

	"github.com/jordigilh/kubernaut/pkg/testutil/enhanced"
)

// Production Performance Baselines Establishment
// Business Requirements: BR-PRODUCTION-003 - Performance baseline establishment
// Following 00-project-guidelines.mdc: MANDATORY business requirement mapping
// Following 03-testing-strategy.mdc: Performance testing with real infrastructure
// Following 09-interface-method-validation.mdc: Interface validation before code generation

// PerformanceBaselineManager manages performance baseline establishment for production clusters
type PerformanceBaselineManager struct {
	client    kubernetes.Interface
	logger    *logrus.Logger
	baselines map[string]*PerformanceBaseline
}

// PerformanceBaseline defines performance baseline for a specific scenario
type PerformanceBaseline struct {
	ScenarioName      string                      `json:"scenario_name"`
	Description       string                      `json:"description"`
	Measurements      []*PerformanceMeasurement   `json:"measurements"`
	Statistics        *PerformanceStatistics      `json:"statistics"`
	Targets           *BaselinePerformanceTargets `json:"targets"`
	ValidationResults *BaselineValidationResults  `json:"validation_results"`
	EstablishedAt     time.Time                   `json:"established_at"`
	LastUpdated       time.Time                   `json:"last_updated"`
}

// PerformanceMeasurement represents a single performance measurement
type PerformanceMeasurement struct {
	MetricName   string        `json:"metric_name"`
	Value        time.Duration `json:"value"`
	Timestamp    time.Time     `json:"timestamp"`
	ClusterInfo  *ClusterInfo  `json:"cluster_info"`
	Success      bool          `json:"success"`
	ErrorMessage string        `json:"error_message,omitempty"`
}

// PerformanceStatistics contains statistical analysis of performance measurements
type PerformanceStatistics struct {
	Mean        time.Duration `json:"mean"`
	Median      time.Duration `json:"median"`
	Min         time.Duration `json:"min"`
	Max         time.Duration `json:"max"`
	StdDev      time.Duration `json:"std_dev"`
	P95         time.Duration `json:"p95"`
	P99         time.Duration `json:"p99"`
	SampleSize  int           `json:"sample_size"`
	SuccessRate float64       `json:"success_rate"`
}

// BaselinePerformanceTargets defines performance targets for baseline validation
type BaselinePerformanceTargets struct {
	ClusterSetupTime     time.Duration `json:"cluster_setup_time"`     // Target: <5 minutes
	WorkloadDeployTime   time.Duration `json:"workload_deploy_time"`   // Target: <3 minutes
	ServiceStartTime     time.Duration `json:"service_start_time"`     // Target: <2 minutes
	HealthCheckTime      time.Duration `json:"health_check_time"`      // Target: <30 seconds
	ValidationTime       time.Duration `json:"validation_time"`        // Target: <1 minute
	ResourceUtilization  float64       `json:"resource_utilization"`   // Target: <80%
	SuccessRateThreshold float64       `json:"success_rate_threshold"` // Target: >95%
}

// BaselineValidationResults contains validation results for performance baselines
type BaselineValidationResults struct {
	MeetsTargets       bool      `json:"meets_targets"`
	FailedTargets      []string  `json:"failed_targets"`
	PerformanceGrade   string    `json:"performance_grade"` // "excellent", "good", "acceptable", "poor"
	RecommendedActions []string  `json:"recommended_actions"`
	ValidationTime     time.Time `json:"validation_time"`
}

// BaselineEstablishmentConfig defines configuration for baseline establishment
type BaselineEstablishmentConfig struct {
	SampleSize          int                        `yaml:"sample_size"`          // Number of measurements per metric
	MeasurementInterval time.Duration              `yaml:"measurement_interval"` // Interval between measurements
	Scenarios           []enhanced.ClusterScenario `yaml:"scenarios"`            // Scenarios to baseline
	EnableStatistics    bool                       `yaml:"enable_statistics"`    // Enable statistical analysis
	ValidationEnabled   bool                       `yaml:"validation_enabled"`   // Enable baseline validation
}

// NewPerformanceBaselineManager creates a new performance baseline manager
// Business Requirement: BR-PRODUCTION-003 - Performance baseline establishment and monitoring
func NewPerformanceBaselineManager(client kubernetes.Interface, logger *logrus.Logger) *PerformanceBaselineManager {
	if logger == nil {
		logger = logrus.New()
		logger.SetLevel(logrus.InfoLevel)
	}

	manager := &PerformanceBaselineManager{
		client:    client,
		logger:    logger,
		baselines: make(map[string]*PerformanceBaseline),
	}

	logger.Info("Performance baseline manager initialized")
	return manager
}

// EstablishBaselines establishes performance baselines for production scenarios
// Business Requirement: BR-PRODUCTION-003 - Comprehensive performance baseline establishment
func (pbm *PerformanceBaselineManager) EstablishBaselines(ctx context.Context, clusterManager *RealClusterManager, config *BaselineEstablishmentConfig) (map[string]*PerformanceBaseline, error) {
	pbm.logger.Info("Starting performance baseline establishment")

	if config == nil {
		config = &BaselineEstablishmentConfig{
			SampleSize:          5,
			MeasurementInterval: 30 * time.Second,
			Scenarios: []enhanced.ClusterScenario{
				enhanced.HighLoadProduction,
				enhanced.ResourceConstrained,
				enhanced.MonitoringStack,
			},
			EnableStatistics:  true,
			ValidationEnabled: true,
		}
	}

	establishmentStart := time.Now()
	results := make(map[string]*PerformanceBaseline)

	// Establish baselines for each scenario
	for _, scenario := range config.Scenarios {
		pbm.logger.WithField("scenario", scenario).Info("Establishing baseline for scenario")

		baseline, err := pbm.establishScenarioBaseline(ctx, clusterManager, scenario, config)
		if err != nil {
			pbm.logger.WithError(err).WithField("scenario", scenario).Warn("Failed to establish baseline for scenario")
			continue
		}

		results[string(scenario)] = baseline
		pbm.baselines[string(scenario)] = baseline
	}

	establishmentTime := time.Since(establishmentStart)

	pbm.logger.WithFields(logrus.Fields{
		"scenarios_count":    len(results),
		"establishment_time": establishmentTime,
	}).Info("Performance baseline establishment completed")

	return results, nil
}

// establishScenarioBaseline establishes baseline for a specific scenario
func (pbm *PerformanceBaselineManager) establishScenarioBaseline(ctx context.Context, clusterManager *RealClusterManager, scenario enhanced.ClusterScenario, config *BaselineEstablishmentConfig) (*PerformanceBaseline, error) {
	baseline := &PerformanceBaseline{
		ScenarioName:      string(scenario),
		Description:       fmt.Sprintf("Performance baseline for %s scenario", scenario),
		Measurements:      []*PerformanceMeasurement{},
		EstablishedAt:     time.Now(),
		LastUpdated:       time.Now(),
		Targets:           pbm.getDefaultTargets(),
		ValidationResults: &BaselineValidationResults{},
	}

	// Collect performance measurements
	for i := 0; i < config.SampleSize; i++ {
		pbm.logger.WithFields(logrus.Fields{
			"scenario": scenario,
			"sample":   i + 1,
			"total":    config.SampleSize,
		}).Info("Collecting performance measurement")

		measurements, err := pbm.collectScenarioMeasurements(ctx, clusterManager, scenario)
		if err != nil {
			pbm.logger.WithError(err).Warn("Failed to collect measurements")
			continue
		}

		baseline.Measurements = append(baseline.Measurements, measurements...)

		// Wait between measurements (except for the last one)
		if i < config.SampleSize-1 {
			time.Sleep(config.MeasurementInterval)
		}
	}

	// Calculate statistics if enabled
	if config.EnableStatistics {
		pbm.calculateStatistics(baseline)
	}

	// Validate baseline if enabled
	if config.ValidationEnabled {
		pbm.validateBaseline(baseline)
	}

	pbm.logger.WithFields(logrus.Fields{
		"scenario":          scenario,
		"measurements":      len(baseline.Measurements),
		"success_rate":      baseline.Statistics.SuccessRate,
		"performance_grade": baseline.ValidationResults.PerformanceGrade,
	}).Info("Scenario baseline established")

	return baseline, nil
}

// collectScenarioMeasurements collects performance measurements for a scenario
func (pbm *PerformanceBaselineManager) collectScenarioMeasurements(ctx context.Context, clusterManager *RealClusterManager, scenario enhanced.ClusterScenario) ([]*PerformanceMeasurement, error) {
	var measurements []*PerformanceMeasurement

	// Measure cluster setup time
	setupStart := time.Now()
	clusterEnv, err := clusterManager.SetupScenario(ctx, scenario)
	setupTime := time.Since(setupStart)

	setupMeasurement := &PerformanceMeasurement{
		MetricName: "cluster_setup_time",
		Value:      setupTime,
		Timestamp:  time.Now(),
		Success:    err == nil,
	}

	if err != nil {
		setupMeasurement.ErrorMessage = err.Error()
		measurements = append(measurements, setupMeasurement)
		return measurements, fmt.Errorf("cluster setup failed: %w", err)
	}

	// Get cluster info for context
	clusterInfo, err := clusterEnv.GetClusterInfo(ctx)
	if err != nil {
		pbm.logger.WithError(err).Warn("Failed to get cluster info")
	} else {
		setupMeasurement.ClusterInfo = clusterInfo
	}

	measurements = append(measurements, setupMeasurement)

	// Measure health check time
	healthStart := time.Now()
	healthErr := pbm.performHealthCheck(ctx, clusterEnv)
	healthTime := time.Since(healthStart)

	healthMeasurement := &PerformanceMeasurement{
		MetricName:  "health_check_time",
		Value:       healthTime,
		Timestamp:   time.Now(),
		ClusterInfo: clusterInfo,
		Success:     healthErr == nil,
	}

	if healthErr != nil {
		healthMeasurement.ErrorMessage = healthErr.Error()
	}

	measurements = append(measurements, healthMeasurement)

	// Measure validation time
	validationStart := time.Now()
	validationErr := pbm.performValidation(ctx, clusterEnv)
	validationTime := time.Since(validationStart)

	validationMeasurement := &PerformanceMeasurement{
		MetricName:  "validation_time",
		Value:       validationTime,
		Timestamp:   time.Now(),
		ClusterInfo: clusterInfo,
		Success:     validationErr == nil,
	}

	if validationErr != nil {
		validationMeasurement.ErrorMessage = validationErr.Error()
	}

	measurements = append(measurements, validationMeasurement)

	// Cleanup cluster environment
	if clusterEnv != nil {
		cleanupStart := time.Now()
		cleanupErr := clusterEnv.Cleanup(ctx)
		cleanupTime := time.Since(cleanupStart)

		cleanupMeasurement := &PerformanceMeasurement{
			MetricName:  "cleanup_time",
			Value:       cleanupTime,
			Timestamp:   time.Now(),
			ClusterInfo: clusterInfo,
			Success:     cleanupErr == nil,
		}

		if cleanupErr != nil {
			cleanupMeasurement.ErrorMessage = cleanupErr.Error()
		}

		measurements = append(measurements, cleanupMeasurement)
	}

	return measurements, nil
}

// performHealthCheck performs a health check on the cluster
func (pbm *PerformanceBaselineManager) performHealthCheck(ctx context.Context, clusterEnv *RealClusterEnvironment) error {
	// Basic health check - verify cluster is responsive
	clusterInfo, err := clusterEnv.GetClusterInfo(ctx)
	if err != nil {
		return fmt.Errorf("cluster info retrieval failed: %w", err)
	}

	if clusterInfo.NodeCount == 0 {
		return fmt.Errorf("cluster has no nodes")
	}

	if clusterInfo.PodCount == 0 {
		return fmt.Errorf("cluster has no pods")
	}

	return nil
}

// performValidation performs validation on the cluster
func (pbm *PerformanceBaselineManager) performValidation(ctx context.Context, clusterEnv *RealClusterEnvironment) error {
	// Basic validation - verify cluster meets minimum requirements
	clusterInfo, err := clusterEnv.GetClusterInfo(ctx)
	if err != nil {
		return fmt.Errorf("cluster validation failed: %w", err)
	}

	// Validate based on scenario requirements
	if clusterInfo.NodeCount < 1 {
		return fmt.Errorf("insufficient nodes for validation")
	}

	return nil
}

// calculateStatistics calculates statistical metrics for performance measurements
func (pbm *PerformanceBaselineManager) calculateStatistics(baseline *PerformanceBaseline) {
	if len(baseline.Measurements) == 0 {
		return
	}

	// Group measurements by metric name
	metricGroups := make(map[string][]*PerformanceMeasurement)
	for _, measurement := range baseline.Measurements {
		metricGroups[measurement.MetricName] = append(metricGroups[measurement.MetricName], measurement)
	}

	// Calculate statistics for the most important metric (cluster_setup_time)
	setupMeasurements := metricGroups["cluster_setup_time"]
	if len(setupMeasurements) == 0 {
		return
	}

	// Extract successful measurements
	var values []time.Duration
	successCount := 0
	for _, measurement := range setupMeasurements {
		if measurement.Success {
			values = append(values, measurement.Value)
			successCount++
		}
	}

	if len(values) == 0 {
		baseline.Statistics = &PerformanceStatistics{
			SampleSize:  len(setupMeasurements),
			SuccessRate: 0.0,
		}
		return
	}

	// Sort values for percentile calculations
	sort.Slice(values, func(i, j int) bool {
		return values[i] < values[j]
	})

	// Calculate basic statistics
	stats := &PerformanceStatistics{
		Min:         values[0],
		Max:         values[len(values)-1],
		SampleSize:  len(setupMeasurements),
		SuccessRate: float64(successCount) / float64(len(setupMeasurements)),
	}

	// Calculate mean
	var total time.Duration
	for _, value := range values {
		total += value
	}
	stats.Mean = total / time.Duration(len(values))

	// Calculate median
	if len(values)%2 == 0 {
		stats.Median = (values[len(values)/2-1] + values[len(values)/2]) / 2
	} else {
		stats.Median = values[len(values)/2]
	}

	// Calculate percentiles
	if len(values) > 1 {
		p95Index := int(float64(len(values)) * 0.95)
		if p95Index >= len(values) {
			p95Index = len(values) - 1
		}
		stats.P95 = values[p95Index]

		p99Index := int(float64(len(values)) * 0.99)
		if p99Index >= len(values) {
			p99Index = len(values) - 1
		}
		stats.P99 = values[p99Index]
	} else {
		stats.P95 = values[0]
		stats.P99 = values[0]
	}

	// Calculate standard deviation
	var variance float64
	meanFloat := float64(stats.Mean)
	for _, value := range values {
		diff := float64(value) - meanFloat
		variance += diff * diff
	}
	variance /= float64(len(values))
	stats.StdDev = time.Duration(variance)

	baseline.Statistics = stats

	pbm.logger.WithFields(logrus.Fields{
		"scenario":     baseline.ScenarioName,
		"mean":         stats.Mean,
		"median":       stats.Median,
		"p95":          stats.P95,
		"success_rate": stats.SuccessRate,
	}).Debug("Performance statistics calculated")
}

// validateBaseline validates performance baseline against targets
func (pbm *PerformanceBaselineManager) validateBaseline(baseline *PerformanceBaseline) {
	if baseline.Statistics == nil {
		baseline.ValidationResults = &BaselineValidationResults{
			MeetsTargets:       false,
			FailedTargets:      []string{"no_statistics"},
			PerformanceGrade:   "poor",
			RecommendedActions: []string{"Collect more measurements", "Enable statistics calculation"},
			ValidationTime:     time.Now(),
		}
		return
	}

	validation := &BaselineValidationResults{
		MeetsTargets:       true,
		FailedTargets:      []string{},
		RecommendedActions: []string{},
		ValidationTime:     time.Now(),
	}

	// Validate cluster setup time
	if baseline.Statistics.Mean > baseline.Targets.ClusterSetupTime {
		validation.MeetsTargets = false
		validation.FailedTargets = append(validation.FailedTargets, "cluster_setup_time")
		validation.RecommendedActions = append(validation.RecommendedActions, "Optimize cluster setup process")
	}

	// Validate success rate
	if baseline.Statistics.SuccessRate < baseline.Targets.SuccessRateThreshold {
		validation.MeetsTargets = false
		validation.FailedTargets = append(validation.FailedTargets, "success_rate")
		validation.RecommendedActions = append(validation.RecommendedActions, "Investigate and fix reliability issues")
	}

	// Determine performance grade
	if validation.MeetsTargets && baseline.Statistics.SuccessRate >= 0.98 {
		validation.PerformanceGrade = "excellent"
	} else if validation.MeetsTargets && baseline.Statistics.SuccessRate >= 0.95 {
		validation.PerformanceGrade = "good"
	} else if baseline.Statistics.SuccessRate >= 0.90 {
		validation.PerformanceGrade = "acceptable"
	} else {
		validation.PerformanceGrade = "poor"
		validation.RecommendedActions = append(validation.RecommendedActions, "Significant performance improvements needed")
	}

	baseline.ValidationResults = validation

	pbm.logger.WithFields(logrus.Fields{
		"scenario":          baseline.ScenarioName,
		"meets_targets":     validation.MeetsTargets,
		"performance_grade": validation.PerformanceGrade,
		"failed_targets":    len(validation.FailedTargets),
	}).Info("Baseline validation completed")
}

// getDefaultTargets returns default performance targets
func (pbm *PerformanceBaselineManager) getDefaultTargets() *BaselinePerformanceTargets {
	return &BaselinePerformanceTargets{
		ClusterSetupTime:     5 * time.Minute,
		WorkloadDeployTime:   3 * time.Minute,
		ServiceStartTime:     2 * time.Minute,
		HealthCheckTime:      30 * time.Second,
		ValidationTime:       1 * time.Minute,
		ResourceUtilization:  0.80, // 80%
		SuccessRateThreshold: 0.95, // 95%
	}
}

// GetBaseline returns the baseline for a specific scenario
func (pbm *PerformanceBaselineManager) GetBaseline(scenarioName string) (*PerformanceBaseline, bool) {
	baseline, exists := pbm.baselines[scenarioName]
	return baseline, exists
}

// GetAllBaselines returns all established baselines
func (pbm *PerformanceBaselineManager) GetAllBaselines() map[string]*PerformanceBaseline {
	return pbm.baselines
}

// ValidateAgainstBaseline validates current performance against established baseline
func (pbm *PerformanceBaselineManager) ValidateAgainstBaseline(scenarioName string, currentMeasurement time.Duration) (*BaselineComparison, error) {
	baseline, exists := pbm.baselines[scenarioName]
	if !exists {
		return nil, fmt.Errorf("no baseline found for scenario: %s", scenarioName)
	}

	if baseline.Statistics == nil {
		return nil, fmt.Errorf("baseline statistics not available for scenario: %s", scenarioName)
	}

	comparison := &BaselineComparison{
		ScenarioName:       scenarioName,
		CurrentMeasurement: currentMeasurement,
		BaselineMean:       baseline.Statistics.Mean,
		BaselineMedian:     baseline.Statistics.Median,
		BaselineP95:        baseline.Statistics.P95,
		ComparisonTime:     time.Now(),
	}

	// Calculate performance ratio
	comparison.PerformanceRatio = float64(currentMeasurement) / float64(baseline.Statistics.Mean)

	// Determine performance status
	if currentMeasurement <= baseline.Statistics.Median {
		comparison.Status = "excellent"
	} else if currentMeasurement <= baseline.Statistics.Mean {
		comparison.Status = "good"
	} else if currentMeasurement <= baseline.Statistics.P95 {
		comparison.Status = "acceptable"
	} else {
		comparison.Status = "poor"
	}

	// Generate recommendations
	if comparison.PerformanceRatio > 1.5 {
		comparison.Recommendations = append(comparison.Recommendations, "Performance significantly below baseline - investigate system issues")
	} else if comparison.PerformanceRatio > 1.2 {
		comparison.Recommendations = append(comparison.Recommendations, "Performance below baseline - consider optimization")
	} else if comparison.PerformanceRatio < 0.8 {
		comparison.Recommendations = append(comparison.Recommendations, "Performance above baseline - consider updating baseline")
	}

	return comparison, nil
}

// BaselineComparison represents a comparison between current performance and baseline
type BaselineComparison struct {
	ScenarioName       string        `json:"scenario_name"`
	CurrentMeasurement time.Duration `json:"current_measurement"`
	BaselineMean       time.Duration `json:"baseline_mean"`
	BaselineMedian     time.Duration `json:"baseline_median"`
	BaselineP95        time.Duration `json:"baseline_p95"`
	PerformanceRatio   float64       `json:"performance_ratio"`
	Status             string        `json:"status"`
	Recommendations    []string      `json:"recommendations"`
	ComparisonTime     time.Time     `json:"comparison_time"`
}

// GetPerformanceReport generates a comprehensive performance report
func (pbm *PerformanceBaselineManager) GetPerformanceReport() *PerformanceReport {
	report := &PerformanceReport{
		GeneratedAt:     time.Now(),
		TotalScenarios:  len(pbm.baselines),
		ScenarioReports: make(map[string]*ScenarioPerformanceReport),
	}

	excellentCount := 0
	goodCount := 0
	acceptableCount := 0
	poorCount := 0

	for scenarioName, baseline := range pbm.baselines {
		scenarioReport := &ScenarioPerformanceReport{
			ScenarioName:      scenarioName,
			PerformanceGrade:  baseline.ValidationResults.PerformanceGrade,
			MeetsTargets:      baseline.ValidationResults.MeetsTargets,
			SuccessRate:       baseline.Statistics.SuccessRate,
			MeanPerformance:   baseline.Statistics.Mean,
			MedianPerformance: baseline.Statistics.Median,
			P95Performance:    baseline.Statistics.P95,
			SampleSize:        baseline.Statistics.SampleSize,
		}

		report.ScenarioReports[scenarioName] = scenarioReport

		// Count performance grades
		switch baseline.ValidationResults.PerformanceGrade {
		case "excellent":
			excellentCount++
		case "good":
			goodCount++
		case "acceptable":
			acceptableCount++
		case "poor":
			poorCount++
		}
	}

	// Calculate overall performance grade
	if excellentCount == len(pbm.baselines) {
		report.OverallGrade = "excellent"
	} else if excellentCount+goodCount >= len(pbm.baselines)*2/3 {
		report.OverallGrade = "good"
	} else if poorCount <= len(pbm.baselines)/3 {
		report.OverallGrade = "acceptable"
	} else {
		report.OverallGrade = "poor"
	}

	report.GradeDistribution = map[string]int{
		"excellent":  excellentCount,
		"good":       goodCount,
		"acceptable": acceptableCount,
		"poor":       poorCount,
	}

	return report
}

// PerformanceReport represents a comprehensive performance report
type PerformanceReport struct {
	GeneratedAt       time.Time                             `json:"generated_at"`
	TotalScenarios    int                                   `json:"total_scenarios"`
	OverallGrade      string                                `json:"overall_grade"`
	GradeDistribution map[string]int                        `json:"grade_distribution"`
	ScenarioReports   map[string]*ScenarioPerformanceReport `json:"scenario_reports"`
}

// ScenarioPerformanceReport represents performance report for a single scenario
type ScenarioPerformanceReport struct {
	ScenarioName      string        `json:"scenario_name"`
	PerformanceGrade  string        `json:"performance_grade"`
	MeetsTargets      bool          `json:"meets_targets"`
	SuccessRate       float64       `json:"success_rate"`
	MeanPerformance   time.Duration `json:"mean_performance"`
	MedianPerformance time.Duration `json:"median_performance"`
	P95Performance    time.Duration `json:"p95_performance"`
	SampleSize        int           `json:"sample_size"`
}
