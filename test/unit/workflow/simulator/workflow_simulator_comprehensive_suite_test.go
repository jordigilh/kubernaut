package simulator

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-SIM-SUITE-001: Workflow Simulator Business Test Suite
// Business Impact: Ensures workflow simulation meets business requirements for safe testing and validation
// Stakeholder Value: Operations teams can confidently test workflows before production deployment

func TestWorkflowSimulatorComprehensive(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Workflow Simulator Comprehensive Suite")
}
