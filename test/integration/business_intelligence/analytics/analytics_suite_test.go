//go:build integration
// +build integration

package analytics

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-BI-ANALYTICS-001: Business Intelligence Analytics Business Intelligence Test Suite Organization
// Business Impact: Ensures comprehensive validation of Business Intelligence Analytics business logic
// Stakeholder Value: Provides executive confidence in Business Intelligence Analytics testing and business continuity
//
// Business Scenario: Executive stakeholders need confidence in Business Intelligence Analytics capabilities
// Business Impact: Ensures all Business Intelligence Analytics components deliver measurable system reliability
// Business Outcome: Test suite framework enables Business Intelligence Analytics validation

func TestUanalytics(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Business Intelligence Analytics Suite")
}
