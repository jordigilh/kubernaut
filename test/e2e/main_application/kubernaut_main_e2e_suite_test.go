//go:build e2e
// +build e2e

package mainapplication

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-MAIN-E2E-SUITE-001: Main Kubernaut Application E2E Test Suite Organization
// Business Impact: Ensures comprehensive validation of complete kubernaut application business workflows
// Stakeholder Value: Executive confidence in end-to-end alert processing and remediation capabilities

func TestKubernautMainE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Main Kubernaut Application E2E Business Workflow Suite")
}
