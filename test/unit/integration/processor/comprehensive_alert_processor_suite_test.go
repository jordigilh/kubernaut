package processor

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-AP-PROCESSOR-SUITE-001: Comprehensive Alert Processor Business Test Suite Organization
// Business Impact: Ensures comprehensive testing of alert processing business logic for production reliability
// Stakeholder Value: Operations teams can trust that alert processors are thoroughly validated before deployment

func TestComprehensiveAlertProcessor(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Comprehensive Alert Processor Unit Tests Suite")
}
