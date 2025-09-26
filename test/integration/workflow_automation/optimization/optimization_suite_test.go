//go:build integration
// +build integration

package optimization

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-WF-OPTIMIZATION-001: Workflow Optimization Business Intelligence Test Suite Organization
// Business Impact: Ensures comprehensive validation of Workflow Optimization business logic
// Stakeholder Value: Provides executive confidence in Workflow Optimization testing and business continuity
//
// Business Scenario: Executive stakeholders need confidence in Workflow Optimization capabilities
// Business Impact: Ensures all Workflow Optimization components deliver measurable system reliability
// Business Outcome: Test suite framework enables Workflow Optimization validation

func TestUoptimization(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Workflow Optimization Suite")
}
