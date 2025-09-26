//go:build integration
// +build integration

package reporting

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-BI-REPORTING-001: Business Intelligence Reporting Business Intelligence Test Suite Organization
// Business Impact: Ensures comprehensive validation of Business Intelligence Reporting business logic
// Stakeholder Value: Provides executive confidence in Business Intelligence Reporting testing and business continuity
//
// Business Scenario: Executive stakeholders need confidence in Business Intelligence Reporting capabilities
// Business Impact: Ensures all Business Intelligence Reporting components deliver measurable system reliability
// Business Outcome: Test suite framework enables Business Intelligence Reporting validation

func TestUreporting(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Business Intelligence Reporting Suite")
}
