//go:build integration
// +build integration

package authentication

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-SEC-AUTH-001: Security Authentication Business Intelligence Test Suite Organization
// Business Impact: Ensures comprehensive validation of Security Authentication business logic
// Stakeholder Value: Provides executive confidence in Security Authentication testing and business continuity
//
// Business Scenario: Executive stakeholders need confidence in Security Authentication capabilities
// Business Impact: Ensures all Security Authentication components deliver measurable system reliability
// Business Outcome: Test suite framework enables Security Authentication validation

func TestUauthentication(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Security Authentication Suite")
}
