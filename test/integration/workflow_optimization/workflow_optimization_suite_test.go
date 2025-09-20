//go:build integration
// +build integration

package workflow_optimization

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// TestWorkflowOptimizationIntegration runs the Workflow Optimization Integration test suite
// Business Requirements: BR-SELF-OPT-INT-001 to BR-SELF-OPT-INT-005
// Following project guidelines: Real component integration testing (no mocks)
func TestWorkflowOptimizationIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Workflow Optimization Integration - Business Requirements Testing Suite", Label("integration"))
}
