//go:build integration
// +build integration

package monitoring

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-PLAT-MONITORING-001: Platform Monitoring Business Intelligence Test Suite Organization
// Business Impact: Ensures comprehensive validation of Platform Monitoring business logic
// Stakeholder Value: Provides executive confidence in Platform Monitoring testing and business continuity
//
// Business Scenario: Executive stakeholders need confidence in Platform Monitoring capabilities
// Business Impact: Ensures all Platform Monitoring components deliver measurable system reliability
// Business Outcome: Test suite framework enables Platform Monitoring validation

func TestUmonitoring(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Platform Monitoring Suite")
}
