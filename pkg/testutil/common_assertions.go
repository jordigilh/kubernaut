package testutil

import (
	"time"

	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/storage/vector"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
	. "github.com/onsi/gomega" //nolint:revive,errcheck
)

// CommonAssertions provides reusable assertion patterns
type CommonAssertions struct{}

// NewCommonAssertions creates a new common assertions helper
func NewCommonAssertions() *CommonAssertions {
	return &CommonAssertions{}
}

// =============================================================================
// ACTION RECOMMENDATION ASSERTIONS
// =============================================================================

// AssertValidActionRecommendation verifies a standard action recommendation
func (a *CommonAssertions) AssertValidActionRecommendation(recommendation *types.ActionRecommendation) {
	Expect(recommendation).NotTo(BeNil(), "Recommendation should not be nil")
	Expect(recommendation.Action).NotTo(BeEmpty(), "Action should not be empty")
	Expect(recommendation.Confidence).To(BeNumerically(">=", 0), "Confidence should be non-negative")
	Expect(recommendation.Confidence).To(BeNumerically("<=", 1), "Confidence should not exceed 1")
}

// AssertActionRecommendationWithConfidence verifies recommendation with minimum confidence
func (a *CommonAssertions) AssertActionRecommendationWithConfidence(recommendation *types.ActionRecommendation, minConfidence float64) {
	a.AssertValidActionRecommendation(recommendation)
	Expect(recommendation.Confidence).To(BeNumerically(">=", minConfidence),
		"Confidence should be at least %.2f", minConfidence)
}

// AssertActionRecommendationHasReasoning verifies recommendation has reasoning
func (a *CommonAssertions) AssertActionRecommendationHasReasoning(recommendation *types.ActionRecommendation) {
	a.AssertValidActionRecommendation(recommendation)
	Expect(recommendation.Reasoning).NotTo(BeNil(), "Reasoning should not be nil")
	Expect(recommendation.Reasoning.Summary).NotTo(BeEmpty(), "Reasoning summary should not be empty")
}

// AssertActionRecommendationHasParameters verifies recommendation has parameters
func (a *CommonAssertions) AssertActionRecommendationHasParameters(recommendation *types.ActionRecommendation, expectedParams ...string) {
	a.AssertValidActionRecommendation(recommendation)
	Expect(recommendation.Parameters).NotTo(BeNil(), "Parameters should not be nil")

	for _, param := range expectedParams {
		Expect(recommendation.Parameters).To(HaveKey(param),
			"Parameters should contain key '%s'", param)
	}
}

// =============================================================================
// ENHANCED ACTION RECOMMENDATION ASSERTIONS
// =============================================================================

// AssertValidEnhancedRecommendation verifies an enhanced action recommendation
func (a *CommonAssertions) AssertValidEnhancedRecommendation(enhanced *types.EnhancedActionRecommendation) {
	Expect(enhanced).NotTo(BeNil(), "Enhanced recommendation should not be nil")
	Expect(enhanced.ActionRecommendation).NotTo(BeNil(), "Base action recommendation should not be nil")
	a.AssertValidActionRecommendation(enhanced.ActionRecommendation)
}

// AssertEnhancedRecommendationHasValidation verifies validation is present
func (a *CommonAssertions) AssertEnhancedRecommendationHasValidation(enhanced *types.EnhancedActionRecommendation) {
	a.AssertValidEnhancedRecommendation(enhanced)
	Expect(enhanced.ValidationResult).NotTo(BeNil(), "Validation result should not be nil")
	Expect(enhanced.ValidationResult.ValidationScore).To(BeNumerically(">=", 0), "Validation score should be non-negative")
	Expect(enhanced.ValidationResult.ValidationScore).To(BeNumerically("<=", 1), "Validation score should not exceed 1")
}

// AssertEnhancedRecommendationHasRiskAssessment verifies risk assessment is present
func (a *CommonAssertions) AssertEnhancedRecommendationHasRiskAssessment(enhanced *types.EnhancedActionRecommendation) {
	a.AssertEnhancedRecommendationHasValidation(enhanced)
	Expect(enhanced.ValidationResult.RiskAssessment).NotTo(BeNil(), "Risk assessment should not be nil")
	Expect(enhanced.ValidationResult.RiskAssessment.RiskLevel).NotTo(BeEmpty(), "Risk level should not be empty")
	Expect(enhanced.ValidationResult.RiskAssessment.ReversibilityScore).To(BeNumerically(">=", 0), "Reversibility score should be non-negative")
	Expect(enhanced.ValidationResult.RiskAssessment.ReversibilityScore).To(BeNumerically("<=", 1), "Reversibility score should not exceed 1")
}

// AssertEnhancedRecommendationHasProcessingMetadata verifies processing metadata is present
func (a *CommonAssertions) AssertEnhancedRecommendationHasProcessingMetadata(enhanced *types.EnhancedActionRecommendation) {
	a.AssertValidEnhancedRecommendation(enhanced)
	Expect(enhanced.ProcessingMetadata).NotTo(BeNil(), "Processing metadata should not be nil")
	Expect(enhanced.ProcessingMetadata.ProcessingTime).To(BeNumerically(">", 0), "Processing time should be positive")
	Expect(enhanced.ProcessingMetadata.AIModelUsed).NotTo(BeEmpty(), "AI model used should not be empty")
}

// =============================================================================
// WORKFLOW ENGINE ASSERTIONS
// =============================================================================

// AssertValidWorkflowStep verifies a workflow step
func (a *CommonAssertions) AssertValidWorkflowStep(step *types.WorkflowStep) {
	Expect(step).NotTo(BeNil(), "Workflow step should not be nil")
	Expect(step.ID).NotTo(BeEmpty(), "Step ID should not be empty")
	Expect(step.Name).NotTo(BeEmpty(), "Step name should not be empty")
}

// AssertValidConditionSpec verifies a condition specification
func (a *CommonAssertions) AssertValidConditionSpec(condition *types.ConditionSpec) {
	Expect(condition).NotTo(BeNil(), "Condition spec should not be nil")
	Expect(condition.ID).NotTo(BeEmpty(), "Condition ID should not be empty")
	Expect(condition.Type).NotTo(BeEmpty(), "Condition type should not be empty")
	Expect(condition.Parameters).NotTo(BeNil(), "Condition parameters should not be nil")
}

// AssertWorkflowStepResult verifies step result
func (a *CommonAssertions) AssertWorkflowStepResult(result *engine.StepResult, shouldSucceed bool) {
	Expect(result).NotTo(BeNil(), "Step result should not be nil")
	Expect(result.Success).To(Equal(shouldSucceed), "Step success should match expectation")

	if shouldSucceed {
		Expect(result.Error).To(BeEmpty(), "Successful step should not have error")
	} else {
		Expect(result.Error).NotTo(BeEmpty(), "Failed step should have error message")
	}
}

// AssertStepContextHasVariables verifies step context has expected variables
func (a *CommonAssertions) AssertStepContextHasVariables(context *engine.StepContext, expectedVars ...string) {
	Expect(context).NotTo(BeNil(), "Step context should not be nil")
	Expect(context.Variables).NotTo(BeNil(), "Context variables should not be nil")

	for _, variable := range expectedVars {
		Expect(context.Variables).To(HaveKey(variable),
			"Context should contain variable '%s'", variable)
	}
}

// =============================================================================
// VECTOR DATABASE ASSERTIONS
// =============================================================================

// AssertValidActionPattern verifies an action pattern
func (a *CommonAssertions) AssertValidActionPattern(pattern *vector.ActionPattern) {
	Expect(pattern).NotTo(BeNil(), "Action pattern should not be nil")
	Expect(pattern.ID).NotTo(BeEmpty(), "Pattern ID should not be empty")
	Expect(pattern.ActionType).NotTo(BeEmpty(), "Action type should not be empty")
	Expect(pattern.CreatedAt).NotTo(BeZero(), "Created at should not be zero")
}

// AssertActionPatternHasEmbedding verifies pattern has embedding
func (a *CommonAssertions) AssertActionPatternHasEmbedding(pattern *vector.ActionPattern) {
	a.AssertValidActionPattern(pattern)
	Expect(pattern.Embedding).NotTo(BeEmpty(), "Pattern embedding should not be empty")

	for i, val := range pattern.Embedding {
		Expect(val).To(BeNumerically(">=", -1), "Embedding value at index %d should be >= -1", i)
		Expect(val).To(BeNumerically("<=", 1), "Embedding value at index %d should be <= 1", i)
	}
}

// AssertActionPatternHasEffectivenessData verifies pattern has effectiveness data
func (a *CommonAssertions) AssertActionPatternHasEffectivenessData(pattern *vector.ActionPattern) {
	a.AssertValidActionPattern(pattern)
	Expect(pattern.EffectivenessData).NotTo(BeNil(), "Effectiveness data should not be nil")
	Expect(pattern.EffectivenessData.Score).To(BeNumerically(">=", 0), "Effectiveness score should be non-negative")
	Expect(pattern.EffectivenessData.Score).To(BeNumerically("<=", 1), "Effectiveness score should not exceed 1")
}

// AssertSimilarActionPatterns verifies similarity between patterns
func (a *CommonAssertions) AssertSimilarActionPatterns(pattern1, pattern2 *vector.ActionPattern, minSimilarity float64) {
	a.AssertValidActionPattern(pattern1)
	a.AssertValidActionPattern(pattern2)

	// Basic similarity checks
	Expect(pattern1.ActionType).To(Equal(pattern2.ActionType), "Patterns should have same action type for high similarity")
	Expect(pattern1.AlertSeverity).To(Equal(pattern2.AlertSeverity), "Patterns should have same alert severity for high similarity")
}

// =============================================================================
// ALERT ASSERTIONS
// =============================================================================

// AssertValidAlert verifies an alert
func (a *CommonAssertions) AssertValidAlert(alert types.Alert) {
	Expect(alert.Name).NotTo(BeEmpty(), "Alert name should not be empty")
	Expect(alert.Description).NotTo(BeEmpty(), "Alert description should not be empty")
	Expect(alert.Severity).NotTo(BeEmpty(), "Alert severity should not be empty")
	Expect(alert.Status).NotTo(BeEmpty(), "Alert status should not be empty")
	Expect(alert.Namespace).NotTo(BeEmpty(), "Alert namespace should not be empty")
	Expect(alert.Resource).NotTo(BeEmpty(), "Alert resource should not be empty")
}

// AssertAlertHasLabels verifies alert has expected labels
func (a *CommonAssertions) AssertAlertHasLabels(alert types.Alert, expectedLabels ...string) {
	a.AssertValidAlert(alert)
	Expect(alert.Labels).NotTo(BeNil(), "Alert labels should not be nil")

	for _, label := range expectedLabels {
		Expect(alert.Labels).To(HaveKey(label),
			"Alert should have label '%s'", label)
	}
}

// AssertAlertSeverity verifies alert severity
func (a *CommonAssertions) AssertAlertSeverity(alert types.Alert, expectedSeverity string) {
	a.AssertValidAlert(alert)
	Expect(alert.Severity).To(Equal(expectedSeverity),
		"Alert severity should be '%s'", expectedSeverity)
}

// =============================================================================
// TIME-BASED ASSERTIONS
// =============================================================================

// AssertRecentTimestamp verifies timestamp is recent
func (a *CommonAssertions) AssertRecentTimestamp(timestamp time.Time, maxAge time.Duration) {
	Expect(timestamp).NotTo(BeZero(), "Timestamp should not be zero")
	Expect(time.Since(timestamp)).To(BeNumerically("<=", maxAge),
		"Timestamp should be within %v", maxAge)
}

// AssertDurationWithinRange verifies duration is within expected range
func (a *CommonAssertions) AssertDurationWithinRange(duration time.Duration, min, max time.Duration) {
	Expect(duration).To(BeNumerically(">=", min),
		"Duration should be at least %v", min)
	Expect(duration).To(BeNumerically("<=", max),
		"Duration should not exceed %v", max)
}

// =============================================================================
// HEALTH CHECK ASSERTIONS
// =============================================================================

// AssertComponentHealthy verifies a component is healthy
func (a *CommonAssertions) AssertComponentHealthy(healthyComponent interface{ IsHealthy() bool }) {
	Expect(healthyComponent).NotTo(BeNil(), "Component should not be nil")
	Expect(healthyComponent.IsHealthy()).To(BeTrue(), "Component should be healthy")
}

// AssertComponentUnhealthy verifies a component is unhealthy
func (a *CommonAssertions) AssertComponentUnhealthy(healthyComponent interface{ IsHealthy() bool }) {
	Expect(healthyComponent).NotTo(BeNil(), "Component should not be nil")
	Expect(healthyComponent.IsHealthy()).To(BeFalse(), "Component should be unhealthy")
}

// =============================================================================
// ERROR ASSERTIONS
// =============================================================================

// AssertNoError verifies no error occurred
func (a *CommonAssertions) AssertNoError(err error, context string) {
	Expect(err).NotTo(HaveOccurred(), "No error should occur in %s", context)
}

// AssertErrorContains verifies error contains expected text
func (a *CommonAssertions) AssertErrorContains(err error, expectedText string) {
	Expect(err).To(HaveOccurred(), "Error should have occurred")
	Expect(err.Error()).To(ContainSubstring(expectedText),
		"Error message should contain '%s'", expectedText)
}

// AssertErrorOfType verifies error is of expected type
func (a *CommonAssertions) AssertErrorOfType(err error, expectedType interface{}) {
	Expect(err).To(HaveOccurred(), "Error should have occurred")
	Expect(err).To(BeAssignableToTypeOf(expectedType),
		"Error should be of expected type")
}
