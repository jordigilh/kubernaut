//go:build integration
// +build integration

package metrics

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-BI-METRICS-001: Business Intelligence Metrics Business Intelligence Test Suite Organization
// Business Impact: Ensures comprehensive validation of Business Intelligence Metrics business logic
// Stakeholder Value: Provides executive confidence in Business Intelligence Metrics testing and business continuity
//
// Business Scenario: Executive stakeholders need confidence in Business Intelligence Metrics capabilities
// Business Impact: Ensures all Business Intelligence Metrics components deliver measurable system reliability
// Business Outcome: Test suite framework enables Business Intelligence Metrics validation

func TestUmetrics(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Business Intelligence Metrics Suite")
}
