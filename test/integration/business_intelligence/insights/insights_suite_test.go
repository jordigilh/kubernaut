//go:build integration
// +build integration

package insights

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-BI-INSIGHTS-001: Business Intelligence Insights Business Intelligence Test Suite Organization
// Business Impact: Ensures comprehensive validation of Business Intelligence Insights business logic
// Stakeholder Value: Provides executive confidence in Business Intelligence Insights testing and business continuity
//
// Business Scenario: Executive stakeholders need confidence in Business Intelligence Insights capabilities
// Business Impact: Ensures all Business Intelligence Insights components deliver measurable system reliability
// Business Outcome: Test suite framework enables Business Intelligence Insights validation

func TestUinsights(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Business Intelligence Insights Suite")
}
