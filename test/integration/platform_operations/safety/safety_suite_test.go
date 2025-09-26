//go:build integration
// +build integration

package safety

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-PLAT-SAFETY-001: Platform Safety Framework Business Intelligence Test Suite Organization
// Business Impact: Ensures comprehensive validation of Platform Safety Framework business logic
// Stakeholder Value: Provides executive confidence in Platform Safety Framework testing and business continuity
//
// Business Scenario: Executive stakeholders need confidence in Platform Safety Framework capabilities
// Business Impact: Ensures all Platform Safety Framework components deliver measurable system reliability
// Business Outcome: Test suite framework enables Platform Safety Framework validation

func TestUsafety(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Platform Safety Framework Suite")
}
