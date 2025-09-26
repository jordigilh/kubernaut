//go:build integration
// +build integration

package compliance

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-SEC-COMPLIANCE-001: Security Compliance Business Intelligence Test Suite Organization
// Business Impact: Ensures comprehensive validation of Security Compliance business logic
// Stakeholder Value: Provides executive confidence in Security Compliance testing and business continuity
//
// Business Scenario: Executive stakeholders need confidence in Security Compliance capabilities
// Business Impact: Ensures all Security Compliance components deliver measurable system reliability
// Business Outcome: Test suite framework enables Security Compliance validation

func TestUcompliance(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Security Compliance Suite")
}
