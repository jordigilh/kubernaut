//go:build integration
// +build integration

package user_journeys

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-E2E-JOURNEY-001: E2E User Journeys Business Intelligence Test Suite Organization
// Business Impact: Ensures comprehensive validation of E2E User Journeys business logic
// Stakeholder Value: Provides executive confidence in E2E User Journeys testing and business continuity
//
// Business Scenario: Executive stakeholders need confidence in E2E User Journeys capabilities
// Business Impact: Ensures all E2E User Journeys components deliver measurable system reliability
// Business Outcome: Test suite framework enables E2E User Journeys validation

func TestUuserUjourneys(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "E2E User Journeys Suite")
}
