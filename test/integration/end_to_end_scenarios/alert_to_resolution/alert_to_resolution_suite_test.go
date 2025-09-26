//go:build integration
// +build integration

package alert_to_resolution

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-E2E-ALERT-001: E2E Alert to Resolution Business Intelligence Test Suite Organization
// Business Impact: Ensures comprehensive validation of E2E Alert to Resolution business logic
// Stakeholder Value: Provides executive confidence in E2E Alert to Resolution testing and business continuity
//
// Business Scenario: Executive stakeholders need confidence in E2E Alert to Resolution capabilities
// Business Impact: Ensures all E2E Alert to Resolution components deliver measurable system reliability
// Business Outcome: Test suite framework enables E2E Alert to Resolution validation

func TestUalertUtoUresolution(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "E2E Alert to Resolution Suite")
}
