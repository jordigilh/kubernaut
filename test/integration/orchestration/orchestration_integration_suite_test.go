//go:build integration
// +build integration

package orchestration

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// TestOrchestrationIntegration runs the Orchestration Integration test suite
// Business Requirements: BR-ORCH-INTEGRATION-001, BR-MONITORING-001, BR-RESILIENCE-001
// Following project guidelines: Real component integration testing (no mocks)
func TestOrchestrationIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Orchestration Integration - Business Requirements Testing Suite", Label("integration"))
}
