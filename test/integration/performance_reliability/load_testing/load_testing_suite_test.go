//go:build integration
// +build integration

package load_testing

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-PERF-LOAD-001: Performance Load Testing Business Intelligence Test Suite Organization
// Business Impact: Ensures comprehensive validation of Performance Load Testing business logic
// Stakeholder Value: Provides executive confidence in Performance Load Testing testing and business continuity
//
// Business Scenario: Executive stakeholders need confidence in Performance Load Testing capabilities
// Business Impact: Ensures all Performance Load Testing components deliver measurable system reliability
// Business Outcome: Test suite framework enables Performance Load Testing validation

func TestUloadUtesting(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Performance Load Testing Suite")
}
