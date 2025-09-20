//go:build integration
// +build integration

package workflow_pgvector

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestWorkflowPgVectorIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Workflow Engine + pgvector Integration Test Suite")
}
