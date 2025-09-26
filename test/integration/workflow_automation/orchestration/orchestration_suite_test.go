//go:build integration
// +build integration

package orchestration

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-WF-ORCHESTRATION-001: Workflow Orchestration Business Intelligence Test Suite Organization
// Business Impact: Ensures comprehensive validation of Workflow Orchestration business logic
// Stakeholder Value: Provides executive confidence in Workflow Orchestration testing and business continuity
//
// Business Scenario: Executive stakeholders need confidence in Workflow Orchestration capabilities
// Business Impact: Ensures all Workflow Orchestration components deliver measurable system reliability
// Business Outcome: Test suite framework enables Workflow Orchestration validation

func TestUorchestration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Workflow Orchestration Suite")
}
