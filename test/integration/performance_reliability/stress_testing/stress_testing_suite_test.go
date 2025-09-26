//go:build integration
// +build integration

package stress_testing

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-PERF-STRESS-001: Performance Stress Testing Business Intelligence Test Suite Organization
// Business Impact: Ensures comprehensive validation of Performance Stress Testing business logic
// Stakeholder Value: Provides executive confidence in Performance Stress Testing testing and business continuity
//
// Business Scenario: Executive stakeholders need confidence in Performance Stress Testing capabilities
// Business Impact: Ensures all Performance Stress Testing components deliver measurable system reliability
// Business Outcome: Test suite framework enables Performance Stress Testing validation

func TestUstressUtesting(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Performance Stress Testing Suite")
}
