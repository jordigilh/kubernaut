//go:build integration
// +build integration

package simulation

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-WF-SIMULATION-001: Workflow Simulation Business Intelligence Test Suite Organization
// Business Impact: Ensures comprehensive validation of Workflow Simulation business logic
// Stakeholder Value: Provides executive confidence in Workflow Simulation testing and business continuity
//
// Business Scenario: Executive stakeholders need confidence in Workflow Simulation capabilities
// Business Impact: Ensures all Workflow Simulation components deliver measurable system reliability
// Business Outcome: Test suite framework enables Workflow Simulation validation

func TestUsimulation(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Workflow Simulation Suite")
}
