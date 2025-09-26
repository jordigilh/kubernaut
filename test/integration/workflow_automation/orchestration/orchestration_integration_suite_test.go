//go:build integration
// +build integration

package orchestration

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-ORCHESTRATION-INTEGRATION-SUITE-001: Orchestration Integration Business Intelligence Test Suite Organization
// Business Impact: Ensures comprehensive validation of orchestration integration business logic
// Stakeholder Value: Provides executive confidence in orchestration integration and business continuity

func TestOrchestrationIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Orchestration Integration Suite")
}
