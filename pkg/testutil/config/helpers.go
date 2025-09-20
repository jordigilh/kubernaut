package config

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// ExpectBusinessRequirement validates that a value meets the specified business requirement
// Following project guidelines: test business requirements, not implementation details
// Business Requirements: Support validation for all BR-XXX-### requirements with meaningful assertions
func ExpectBusinessRequirement(actual interface{}, requirement string, env string, description string) {
	thresholds, err := LoadThresholds(env)
	Expect(err).ToNot(HaveOccurred(), "Failed to load business requirement thresholds")

	switch requirement {
	// Database Business Requirements
	case "BR-DATABASE-001-A-UTILIZATION":
		threshold := thresholds.Database.BRDatabase001A.UtilizationThreshold
		Expect(actual).To(BeNumerically(">=", threshold),
			fmt.Sprintf("%s: Must meet %s utilization threshold (%.1f%%)", requirement, description, threshold*100))
		return

	case "BR-DATABASE-001-B-HEALTH-SCORE":
		threshold := thresholds.Database.BRDatabase001B.HealthScoreThreshold
		Expect(actual).To(BeNumerically(">=", threshold),
			fmt.Sprintf("%s: Must meet %s health score threshold (%.1f%%)", requirement, description, threshold*100))
		return

	case "BR-DATABASE-001-B-FAILURE-RATE":
		threshold := thresholds.Database.BRDatabase001B.FailureRateThreshold
		Expect(actual).To(BeNumerically("<=", threshold),
			fmt.Sprintf("%s: Must maintain %s failure rate below threshold (%.1f%%)", requirement, description, threshold*100))
		return

	case "BR-DATABASE-001-B-WAIT-TIME":
		threshold := thresholds.Database.BRDatabase001B.WaitTimeThreshold
		Expect(actual).To(BeNumerically("<=", threshold),
			fmt.Sprintf("%s: Must maintain %s wait time below threshold (%v)", requirement, description, threshold))
		return

	case "BR-DATABASE-002-RECOVERY-TIME":
		threshold := thresholds.Database.BRDatabase002.ExhaustionRecoveryTime
		Expect(actual).To(BeNumerically("<=", threshold),
			fmt.Sprintf("%s: Must recover from %s within threshold (%v)", requirement, description, threshold))
		return

	// Performance Business Requirements
	case "BR-PERF-001-RESPONSE-TIME":
		threshold := thresholds.Performance.BRPERF001.MaxResponseTime
		Expect(actual).To(BeNumerically("<=", threshold),
			fmt.Sprintf("%s: Must maintain %s response time below threshold (%v)", requirement, description, threshold))
		return

	case "BR-PERF-001-THROUGHPUT":
		threshold := thresholds.Performance.BRPERF001.MinThroughput
		Expect(actual).To(BeNumerically(">=", threshold),
			fmt.Sprintf("%s: Must maintain %s throughput above threshold (%d/sec)", requirement, description, threshold))
		return

	case "BR-PERF-001-ACCURACY":
		threshold := thresholds.Performance.BRPERF001.AccuracyThreshold
		Expect(actual).To(BeNumerically(">=", threshold),
			fmt.Sprintf("%s: Must maintain %s accuracy above threshold (%.1f%%)", requirement, description, threshold*100))
		return

	case "BR-PERF-001-ERROR-RATE":
		threshold := thresholds.Performance.BRPERF001.ErrorRateThreshold
		Expect(actual).To(BeNumerically("<=", threshold),
			fmt.Sprintf("%s: Must maintain %s error rate below threshold (%.1f%%)", requirement, description, threshold*100))
		return

	// AI Business Requirements
	case "BR-AI-001-CONFIDENCE":
		threshold := thresholds.AI.BRAI001.MinConfidenceScore
		Expect(actual).To(BeNumerically(">=", threshold),
			fmt.Sprintf("%s: Must meet %s confidence threshold (%.1f%%)", requirement, description, threshold*100))
		return

	case "BR-AI-001-ANALYSIS-TIME":
		threshold := thresholds.AI.BRAI001.MaxAnalysisTime
		Expect(actual).To(BeNumerically("<=", threshold),
			fmt.Sprintf("%s: Must complete %s within time threshold (%v)", requirement, description, threshold))
		return

	case "BR-AI-001-ACCURACY":
		threshold := thresholds.AI.BRAI001.AccuracyThreshold
		Expect(actual).To(BeNumerically(">=", threshold),
			fmt.Sprintf("%s: Must meet %s accuracy threshold (%.1f%%)", requirement, description, threshold*100))
		return

	case "BR-AI-002-RECOMMENDATION-CONFIDENCE":
		threshold := thresholds.AI.BRAI002.RecommendationConfidence
		Expect(actual).To(BeNumerically(">=", threshold),
			fmt.Sprintf("%s: Must meet %s recommendation confidence threshold (%.1f%%)", requirement, description, threshold*100))
		return

	case "BR-AI-002-ACTION-VALIDATION-TIME":
		threshold := thresholds.AI.BRAI002.ActionValidationTime
		Expect(actual).To(BeNumerically("<=", threshold),
			fmt.Sprintf("%s: Must complete %s action validation within threshold (%v)", requirement, description, threshold))
		return

	case "BR-AI-002-SUCCESS-RATE":
		threshold := thresholds.AI.BRAI002.MinSuccessRate
		Expect(actual).To(BeNumerically(">=", threshold),
			fmt.Sprintf("%s: Must maintain %s success rate above threshold (%.1f%%)", requirement, description, threshold*100))

	// Workflow Business Requirements
	case "BR-WF-001-EXECUTION-TIME":
		threshold := thresholds.Workflow.BRWF001.MaxExecutionTime
		Expect(actual).To(BeNumerically("<=", threshold),
			fmt.Sprintf("%s: Must complete %s within execution time threshold (%v)", requirement, description, threshold))
		return

	case "BR-WF-001-SUCCESS-RATE":
		threshold := thresholds.Workflow.BRWF001.MinSuccessRate
		Expect(actual).To(BeNumerically(">=", threshold),
			fmt.Sprintf("%s: Must maintain %s success rate above threshold (%.1f%%)", requirement, description, threshold*100))

	case "BR-WF-001-RESOURCE-EFFICIENCY":
		threshold := thresholds.Workflow.BRWF001.ResourceEfficiencyThreshold
		Expect(actual).To(BeNumerically(">=", threshold),
			fmt.Sprintf("%s: Must maintain %s resource efficiency above threshold (%.1f%%)", requirement, description, threshold*100))
		return

	case "BR-WF-002-CONCURRENT-WORKFLOWS":
		threshold := thresholds.Workflow.BRWF002.MaxConcurrentWorkflows
		Expect(actual).To(BeNumerically("<=", threshold),
			fmt.Sprintf("%s: Must keep %s concurrent workflows below threshold (%d)", requirement, description, threshold))
		return

	// BR-WF-001 State Management and Business Continuity Requirements
	case "BR-WF-001-STATE-PERSISTENCE":
		// Business Requirement: Workflow state persistence for business continuity and recovery
		// Threshold: Minimum 3 saves (initial + intermediate + final) ensures recovery capability
		minSaves := 3
		Expect(actual).To(BeNumerically(">=", minSaves),
			fmt.Sprintf("%s: Must have >= %d state persistence saves for business continuity in %s", requirement, minSaves, description))
		return

	case "BR-WF-001-NOTIFICATION-TEAMS":
		// Business Requirement: Failure notification distribution for business accountability
		// Threshold: At least 1 team ensures someone is responsible for workflow failures
		minTeams := 1
		Expect(actual).To(BeNumerically(">=", minTeams),
			fmt.Sprintf("%s: Must have >= %d notification teams for business accountability in %s", requirement, minTeams, description))
		return

	case "BR-WF-001-MAX-DOWNTIME-MINUTES":
		// Business Requirement: Maximum acceptable downtime for business continuity
		// Threshold: <= 5 minutes downtime ensures business service availability
		maxDowntimeMinutes := 5
		Expect(actual).To(BeNumerically("<=", maxDowntimeMinutes),
			fmt.Sprintf("%s: Must have <= %d minutes downtime for business continuity in %s", requirement, maxDowntimeMinutes, description))
		return

	// BR-WF-541 Resilient Workflow Execution Requirements
	case "BR-WF-541-TERMINATION-RATE":
		// Business Requirement: Workflow termination rate must be <10% for business continuity
		// Threshold: <10% termination rate ensures resilient workflow execution
		maxTerminationRate := 10.0 // 10% in percentage
		Expect(actual).To(BeNumerically("<", maxTerminationRate),
			fmt.Sprintf("%s: Must have < %.1f%% workflow termination rate for business continuity in %s", requirement, maxTerminationRate, description))
		return

	case "BR-WF-541-PERFORMANCE-IMPROVEMENT":
		// Business Requirement: Parallel execution must achieve >40% performance improvement
		// Threshold: >40% improvement demonstrates parallel execution effectiveness
		minImprovement := 40.0 // 40% in percentage
		Expect(actual).To(BeNumerically(">", minImprovement),
			fmt.Sprintf("%s: Must achieve > %.1f%% performance improvement for parallel execution in %s", requirement, minImprovement, description))
		return

	// BR-ORCH-004 Learning from Execution Failures Requirements
	case "BR-ORCH-004-LEARNING-CONFIDENCE":
		// Business Requirement: Learning adjustments must have ≥80% confidence
		// Threshold: ≥80% confidence ensures reliable learning-based adaptations
		minConfidence := 80.0 // 80% in percentage
		Expect(actual).To(BeNumerically(">=", minConfidence),
			fmt.Sprintf("%s: Must achieve >= %.1f%% learning confidence for reliable adaptation in %s", requirement, minConfidence, description))
		return

	case "BR-ORCH-004-RETRY-EFFECTIVENESS":
		// Business Requirement: Retry effectiveness should improve based on learning
		// Threshold: >70% effectiveness demonstrates meaningful retry learning
		minEffectiveness := 70.0 // 70% in percentage
		Expect(actual).To(BeNumerically(">=", minEffectiveness),
			fmt.Sprintf("%s: Must achieve >= %.1f%% retry effectiveness from learning in %s", requirement, minEffectiveness, description))
		return

	// BR-ORCH-001 Self-Optimization Framework Requirements
	case "BR-ORCH-001-OPTIMIZATION-CONFIDENCE":
		// Business Requirement: Self-optimization must have ≥80% confidence
		// Threshold: ≥80% confidence ensures reliable optimization decisions
		minOptimizationConfidence := 80.0 // 80% in percentage
		Expect(actual).To(BeNumerically(">=", minOptimizationConfidence),
			fmt.Sprintf("%s: Must achieve >= %.1f%% optimization confidence for reliable self-optimization in %s", requirement, minOptimizationConfidence, description))
		return

	case "BR-ORCH-001-PERFORMANCE-GAINS":
		// Business Requirement: Self-optimization must achieve ≥15% performance gains
		// Threshold: ≥15% gains demonstrate meaningful optimization impact
		minPerformanceGains := 15.0 // 15% in percentage
		Expect(actual).To(BeNumerically(">=", minPerformanceGains),
			fmt.Sprintf("%s: Must achieve >= %.1f%% performance gains from self-optimization in %s", requirement, minPerformanceGains, description))
		return

	// Safety Business Requirements
	case "BR-SF-001-RISK-SCORE":
		threshold := thresholds.Safety.BRSF001.MaxRiskScore
		Expect(actual).To(BeNumerically("<=", threshold),
			fmt.Sprintf("%s: Must maintain %s risk score below threshold (%.1f%%)", requirement, description, threshold*100))
		return

	case "BR-SF-001-AUTO-APPROVAL":
		threshold := thresholds.Safety.BRSF001.AutoApprovalLimit
		Expect(actual).To(BeNumerically("<=", threshold),
			fmt.Sprintf("%s: Must keep %s auto-approval risk below threshold (%.1f%%)", requirement, description, threshold*100))
		return

	case "BR-SF-002-ROLLBACK-TIME":
		threshold := thresholds.Safety.BRSF002.RollbackTimeLimit
		Expect(actual).To(BeNumerically("<=", threshold),
			fmt.Sprintf("%s: Must complete %s rollback within time threshold (%v)", requirement, description, threshold))
		return

	case "BR-SF-002-VALIDATION-TIME":
		threshold := thresholds.Safety.BRSF002.ValidationTimeout
		Expect(actual).To(BeNumerically("<=", threshold),
			fmt.Sprintf("%s: Must complete %s validation within time threshold (%v)", requirement, description, threshold))
		return

	// Monitoring Business Requirements
	case "BR-MON-001-ALERT-THRESHOLD":
		threshold := thresholds.Monitoring.BRMON001.AlertThreshold
		Expect(actual).To(BeNumerically(">=", threshold),
			fmt.Sprintf("%s: Must meet %s alert threshold (%.1f%%)", requirement, description, threshold*100))
		return

	case "BR-MON-001-UPTIME":
		threshold := thresholds.Monitoring.BRMON001.UptimeRequirement
		Expect(actual).To(BeNumerically(">=", threshold),
			fmt.Sprintf("%s: Must maintain %s uptime above threshold (%.3f%%)", requirement, description, threshold*100))
		return

	default:
		Fail(fmt.Sprintf("Unknown business requirement: %s", requirement))
	}
}

// ExpectDurationWithinBusinessRequirement validates duration against business requirement thresholds
func ExpectDurationWithinBusinessRequirement(actual time.Duration, requirement string, env string, description string) {
	ExpectBusinessRequirement(actual, requirement, env, description)
}

// ExpectCountExactly validates exact counts for business logic expectations
// Following project guidelines: AVOID weak assertions like > 0, provide exact business validation
func ExpectCountExactly(actual interface{}, expected int, requirement string, description string) {
	Expect(actual).To(Equal(expected),
		fmt.Sprintf("%s: Must have exactly %d %s (business logic expectation)", requirement, expected, description))
}

// ExpectBusinessProperty validates business properties exist and have expected values
// Following project guidelines: test business outcomes, not implementation details
func ExpectBusinessProperty(actual interface{}, property string, expectedValue interface{}, requirement string, description string) {
	Expect(actual).To(Equal(expectedValue),
		fmt.Sprintf("%s: Must have %s property set to '%v' for %s", requirement, property, expectedValue, description))
}

// ExpectBusinessState validates that business entities are in expected states
func ExpectBusinessState(actual interface{}, expectedState string, requirement string, description string) {
	Expect(actual).To(Equal(expectedState),
		fmt.Sprintf("%s: Must be in '%s' state for %s", requirement, expectedState, description))
}

// ExpectBusinessCollection validates collections meet business requirements
func ExpectBusinessCollection(collection interface{}, requirement string, description string) BusinessCollectionAssertion {
	return BusinessCollectionAssertion{
		Collection:  collection,
		Requirement: requirement,
		Description: description,
	}
}

// BusinessCollectionAssertion provides business-focused collection assertions
type BusinessCollectionAssertion struct {
	Collection  interface{}
	Requirement string
	Description string
}

// ToContainBusinessEntity validates that a collection contains expected business entities
func (b BusinessCollectionAssertion) ToContainBusinessEntity(entity interface{}) {
	Expect(b.Collection).To(ContainElement(entity),
		fmt.Sprintf("%s: Collection must contain business entity '%v' for %s", b.Requirement, entity, b.Description))
}

// ToHaveBusinessCount validates that a collection has the expected business count
func (b BusinessCollectionAssertion) ToHaveBusinessCount(count int) {
	Expect(b.Collection).To(HaveLen(count),
		fmt.Sprintf("%s: Collection must have exactly %d items for %s", b.Requirement, count, b.Description))
}

// ToNotBeEmptyForBusiness validates that a collection is not empty for business reasons
func (b BusinessCollectionAssertion) ToNotBeEmptyForBusiness() {
	Expect(b.Collection).ToNot(BeEmpty(),
		fmt.Sprintf("%s: Collection must not be empty for %s", b.Requirement, b.Description))
}

// ValidateThresholdConfiguration validates that the threshold configuration is properly loaded
func ValidateThresholdConfiguration(env string) error {
	thresholds, err := LoadThresholds(env)
	if err != nil {
		return fmt.Errorf("failed to load thresholds for environment %s: %w", env, err)
	}

	if thresholds == nil {
		return fmt.Errorf("loaded thresholds are nil for environment %s", env)
	}

	// Validate critical thresholds are set
	if thresholds.Database.BRDatabase001A.UtilizationThreshold <= 0 {
		return fmt.Errorf("database utilization threshold not set for environment %s", env)
	}

	if thresholds.AI.BRAI001.MinConfidenceScore <= 0 {
		return fmt.Errorf("AI confidence threshold not set for environment %s", env)
	}

	return nil
}

// GetEnvironmentThresholdSummary returns a summary of key thresholds for an environment
func GetEnvironmentThresholdSummary(env string) (map[string]interface{}, error) {
	thresholds, err := LoadThresholds(env)
	if err != nil {
		return nil, err
	}

	summary := map[string]interface{}{
		"environment": env,
		"database": map[string]interface{}{
			"utilization_threshold":  thresholds.Database.BRDatabase001A.UtilizationThreshold,
			"health_score_threshold": thresholds.Database.BRDatabase001B.HealthScoreThreshold,
			"failure_rate_threshold": thresholds.Database.BRDatabase001B.FailureRateThreshold,
			"wait_time_threshold":    thresholds.Database.BRDatabase001B.WaitTimeThreshold,
		},
		"performance": map[string]interface{}{
			"max_response_time":    thresholds.Performance.BRPERF001.MaxResponseTime,
			"min_throughput":       thresholds.Performance.BRPERF001.MinThroughput,
			"accuracy_threshold":   thresholds.Performance.BRPERF001.AccuracyThreshold,
			"error_rate_threshold": thresholds.Performance.BRPERF001.ErrorRateThreshold,
		},
		"ai": map[string]interface{}{
			"min_confidence_score":      thresholds.AI.BRAI001.MinConfidenceScore,
			"max_analysis_time":         thresholds.AI.BRAI001.MaxAnalysisTime,
			"recommendation_confidence": thresholds.AI.BRAI002.RecommendationConfidence,
			"action_validation_time":    thresholds.AI.BRAI002.ActionValidationTime,
		},
		"workflow": map[string]interface{}{
			"max_execution_time":            thresholds.Workflow.BRWF001.MaxExecutionTime,
			"min_success_rate":              thresholds.Workflow.BRWF001.MinSuccessRate,
			"resource_efficiency_threshold": thresholds.Workflow.BRWF001.ResourceEfficiencyThreshold,
			"max_concurrent_workflows":      thresholds.Workflow.BRWF002.MaxConcurrentWorkflows,
		},
		"safety": map[string]interface{}{
			"max_risk_score":      thresholds.Safety.BRSF001.MaxRiskScore,
			"auto_approval_limit": thresholds.Safety.BRSF001.AutoApprovalLimit,
			"rollback_time_limit": thresholds.Safety.BRSF002.RollbackTimeLimit,
			"validation_timeout":  thresholds.Safety.BRSF002.ValidationTimeout,
		},
	}

	return summary, nil
}
