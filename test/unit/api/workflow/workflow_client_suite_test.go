package workflow

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-WORKFLOW-API-001: Unified workflow API client for reuse between business logic and tests
// BR-WORKFLOW-API-002: Eliminate code duplication in HTTP client patterns
// BR-WORKFLOW-API-003: Integration with existing webhook response patterns

func TestWorkflowClient(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Workflow API Client Unit Tests Suite")
}

