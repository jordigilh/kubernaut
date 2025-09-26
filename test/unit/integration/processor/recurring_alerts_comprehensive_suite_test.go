package processor

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-RECURRING-ALERTS-SUITE-001: Comprehensive Recurring Alert Processing Business Test Suite Organization
// Business Impact: Ensures comprehensive testing of recurring alert processing business logic for production reliability
// Stakeholder Value: Operations teams can trust that alert processing and learning are thoroughly validated

func TestComprehensiveRecurringAlerts(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Comprehensive Recurring Alert Processing Unit Tests Suite")
}
