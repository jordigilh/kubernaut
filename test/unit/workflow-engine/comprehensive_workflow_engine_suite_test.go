package workflowengine

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-WF-ENGINE-SUITE-001: Comprehensive Workflow Engine Business Test Suite Organization
// Business Impact: Ensures comprehensive testing of workflow engine business logic for production reliability
// Stakeholder Value: Operations teams can trust that workflow engines are thoroughly validated before deployment

func TestComprehensiveWorkflowEngine(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Comprehensive Workflow Engine Unit Tests Suite")
}
