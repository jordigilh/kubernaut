//go:build integration
// +build integration

package execution

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-WF-EXECUTION-001: Workflow Execution Business Intelligence Test Suite Organization
// Business Impact: Ensures comprehensive validation of Workflow Execution business logic
// Stakeholder Value: Provides executive confidence in Workflow Execution testing and business continuity
//
// Business Scenario: Executive stakeholders need confidence in Workflow Execution capabilities
// Business Impact: Ensures all Workflow Execution components deliver measurable system reliability
// Business Outcome: Test suite framework enables Workflow Execution validation

func TestUexecution(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Workflow Execution Suite")
}
