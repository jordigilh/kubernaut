//go:build integration
// +build integration

package decision_making

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-AI-DECISION-001: AI Decision Making Business Intelligence Test Suite Organization
// Business Impact: Ensures comprehensive validation of AI Decision Making business logic
// Stakeholder Value: Provides executive confidence in AI Decision Making testing and business continuity
//
// Business Scenario: Executive stakeholders need confidence in AI Decision Making capabilities
// Business Impact: Ensures all AI Decision Making components deliver measurable system reliability
// Business Outcome: Test suite framework enables AI Decision Making validation

func TestUdecisionUmaking(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AI Decision Making Suite")
}
