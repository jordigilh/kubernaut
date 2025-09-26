//go:build e2e
// +build e2e

package orchestration

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-WORKFLOW-ORCHESTRATION-E2E-SUITE-001: Workflow Orchestration E2E Test Suite Organization
// Business Impact: Ensures comprehensive validation of workflow orchestration and coordination capabilities
// Stakeholder Value: Executive confidence in complex multi-step workflow automation for business operations

func TestWorkflowOrchestrationE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Workflow Orchestration E2E Business Suite")
}
