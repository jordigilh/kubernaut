//go:build e2e
// +build e2e

package monitoring

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-HEALTH-MONITORING-E2E-SUITE-001: Health Monitoring E2E Test Suite Organization
// Business Impact: Ensures comprehensive validation of health monitoring and observability business workflows
// Stakeholder Value: Executive confidence in complete system health monitoring for operational excellence

func TestHealthMonitoringE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Health Monitoring E2E Business Workflow Suite")
}
