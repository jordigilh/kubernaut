//go:build integration
// +build integration

package code_quality

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-DEV-QUALITY-001: Development Code Quality Business Intelligence Test Suite Organization
// Business Impact: Ensures comprehensive validation of Development Code Quality business logic
// Stakeholder Value: Provides executive confidence in Development Code Quality testing and business continuity
//
// Business Scenario: Executive stakeholders need confidence in Development Code Quality capabilities
// Business Impact: Ensures all Development Code Quality components deliver measurable system reliability
// Business Outcome: Test suite framework enables Development Code Quality validation

func TestUcodeUquality(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Development Code Quality Suite")
}
