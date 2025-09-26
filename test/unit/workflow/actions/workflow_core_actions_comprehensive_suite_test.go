package actions

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-WF-CORE-ACTIONS-SUITE-001: Comprehensive Workflow Core Actions Business Test Suite Organization
// Business Impact: Ensures comprehensive testing of workflow core actions business logic for production reliability
// Stakeholder Value: Operations teams can trust that workflow creation and execution are thoroughly validated

func TestComprehensiveWorkflowCoreActions(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Comprehensive Workflow Core Actions Unit Tests Suite")
}
