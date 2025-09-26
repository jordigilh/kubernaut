//go:build integration
// +build integration

package audit

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-SEC-AUDIT-001: Security Audit Business Intelligence Test Suite Organization
// Business Impact: Ensures comprehensive validation of Security Audit business logic
// Stakeholder Value: Provides executive confidence in Security Audit testing and business continuity
//
// Business Scenario: Executive stakeholders need confidence in Security Audit capabilities
// Business Impact: Ensures all Security Audit components deliver measurable system reliability
// Business Outcome: Test suite framework enables Security Audit validation

func TestUaudit(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Security Audit Suite")
}
