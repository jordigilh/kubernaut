package persistence

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-WORKFLOW-STATE-PERSISTENCE-SUITE-001: Comprehensive Workflow State Persistence Business Test Suite Organization
// Business Impact: Ensures comprehensive testing of workflow state persistence business logic for production reliability
// Stakeholder Value: Operations teams can trust that workflow state management is thoroughly validated

func TestComprehensiveWorkflowStatePersistence(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Comprehensive Workflow State Persistence Unit Tests Suite")
}
