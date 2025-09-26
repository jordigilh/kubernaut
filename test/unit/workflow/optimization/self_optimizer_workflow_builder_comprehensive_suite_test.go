package optimization

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-SELF-OPT-SUITE-001: Self Optimizer + Workflow Builder Business Test Suite
// Business Impact: Ensures automated workflow optimization meets business requirements for operational efficiency
// Stakeholder Value: Operations teams can trust automated optimization delivers measurable performance improvements

func TestSelfOptimizerWorkflowBuilderComprehensive(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Self Optimizer + Workflow Builder Comprehensive Suite")
}
