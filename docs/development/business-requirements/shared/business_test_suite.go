package shared

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:staticcheck
	. "github.com/onsi/gomega"    //nolint:staticcheck
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/testutil"
)

// BusinessTestSuite provides utilities for business requirement testing
type BusinessTestSuite struct {
	Logger      *logrus.Logger
	Context     context.Context
	Components  *testutil.TestSuiteComponents
	Factory     *testutil.MockFactory
	DataFactory *testutil.TestDataFactory
	Assertions  *testutil.CommonAssertions
}

// NewBusinessTestSuite creates a standardized business test environment
func NewBusinessTestSuite(suiteName string) *BusinessTestSuite {
	components := testutil.AITestSuite(suiteName)

	return &BusinessTestSuite{
		Logger:      components.Logger,
		Context:     components.Context,
		Components:  components,
		Factory:     testutil.NewMockFactory(components.Logger),
		DataFactory: testutil.NewTestDataFactory(),
		Assertions:  testutil.NewCommonAssertions(),
	}
}

// BusinessRequirement represents a testable business requirement with acceptance criteria
type BusinessRequirement struct {
	ID                 string
	Component          string
	Description        string
	BusinessValue      string
	AcceptanceCriteria []AcceptanceCriterion
	Setup              func(ctx context.Context, suite *BusinessTestSuite) error
	Cleanup            func(ctx context.Context, suite *BusinessTestSuite) error
}

// AcceptanceCriterion defines measurable success criteria for a business requirement
type AcceptanceCriterion struct {
	Description      string
	Measurement      string
	SuccessThreshold interface{}
	TestFunction     func(ctx context.Context, suite *BusinessTestSuite) (interface{}, error)
	Timeout          time.Duration
}

// TestBusinessRequirement executes a business requirement test with proper structure
func (suite *BusinessTestSuite) TestBusinessRequirement(req BusinessRequirement) {
	Describe(formatBusinessRequirementTitle(req), func() {
		var ctx context.Context

		BeforeEach(func() {
			ctx = suite.Context
			suite.Logger.WithFields(logrus.Fields{
				"requirement_id": req.ID,
				"component":      req.Component,
			}).Info("Starting business requirement test")

			if req.Setup != nil {
				err := req.Setup(ctx, suite)
				Expect(err).ToNot(HaveOccurred(), "Business requirement setup should succeed")
			}
		})

		AfterEach(func() {
			if req.Cleanup != nil {
				err := req.Cleanup(ctx, suite)
				Expect(err).ToNot(HaveOccurred(), "Business requirement cleanup should succeed")
			}
		})

		// Test each acceptance criterion
		for _, criterion := range req.AcceptanceCriteria {
			criterion := criterion // Capture loop variable

			It(formatAcceptanceCriterion(criterion), func() {
				timeout := criterion.Timeout
				if timeout == 0 {
					timeout = 30 * time.Second // Default business operation timeout
				}

				testCtx, cancel := context.WithTimeout(ctx, timeout)
				defer cancel()

				// Execute business logic and measure outcome
				Eventually(func(g Gomega) {
					result, err := criterion.TestFunction(testCtx, suite)
					g.Expect(err).ToNot(HaveOccurred(), "Business requirement test should not error")

					// Validate against success threshold
					suite.validateBusinessOutcome(g, result, criterion)
				}, timeout, 1*time.Second).Should(Succeed())
			})
		}
	})
}

// validateBusinessOutcome validates the business outcome against the success threshold
func (suite *BusinessTestSuite) validateBusinessOutcome(g Gomega, result interface{}, criterion AcceptanceCriterion) {
	switch threshold := criterion.SuccessThreshold.(type) {
	case float64:
		if resultVal, ok := result.(float64); ok {
			g.Expect(resultVal).To(BeNumerically(">=", threshold),
				"Business outcome should meet threshold: %s", criterion.Measurement)
		}
	case int:
		if resultVal, ok := result.(int); ok {
			g.Expect(resultVal).To(BeNumerically(">=", threshold),
				"Business outcome should meet threshold: %s", criterion.Measurement)
		}
	case bool:
		if resultVal, ok := result.(bool); ok {
			g.Expect(resultVal).To(Equal(threshold),
				"Business outcome should match expected value: %s", criterion.Measurement)
		}
	case string:
		if resultVal, ok := result.(string); ok {
			g.Expect(resultVal).To(ContainSubstring(threshold),
				"Business outcome should contain expected value: %s", criterion.Measurement)
		}
	default:
		g.Expect(result).To(Equal(threshold),
			"Business outcome should match expected value: %s", criterion.Measurement)
	}
}

// Helper functions for formatting
func formatBusinessRequirementTitle(req BusinessRequirement) string {
	return req.ID + ": " + req.Description
}

func formatAcceptanceCriterion(criterion AcceptanceCriterion) string {
	return "should " + criterion.Description + " (" + criterion.Measurement + ")"
}

// Business value measurement helpers
type BusinessMetric struct {
	Name           string
	BaselineValue  interface{}
	ActualValue    interface{}
	ImprovementPct float64
	Unit           string
}

// CalculateBusinessImpact calculates the business impact of a change
func (suite *BusinessTestSuite) CalculateBusinessImpact(baseline, actual interface{}) *BusinessMetric {
	// Implementation for calculating business impact metrics
	// This would be expanded based on specific business metrics needed
	return &BusinessMetric{
		Name:           "Performance Improvement",
		BaselineValue:  baseline,
		ActualValue:    actual,
		ImprovementPct: 0.0, // Calculate based on values
		Unit:           "percent",
	}
}

// LogBusinessOutcome logs business test outcomes in a stakeholder-friendly format
func (suite *BusinessTestSuite) LogBusinessOutcome(requirementID string, metric *BusinessMetric, success bool) {
	suite.Logger.WithFields(logrus.Fields{
		"requirement_id":      requirementID,
		"metric_name":         metric.Name,
		"baseline_value":      metric.BaselineValue,
		"actual_value":        metric.ActualValue,
		"improvement_percent": metric.ImprovementPct,
		"unit":                metric.Unit,
		"business_success":    success,
	}).Info("Business requirement test outcome")
}
