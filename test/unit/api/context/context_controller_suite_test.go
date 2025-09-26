package context

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-API-SUITE-001: Context API Business Logic Test Suite Organization
// Business Impact: Ensures Context API business requirements are systematically validated
// Stakeholder Value: Operations teams can trust Context API functionality for alert investigations

func TestContextController(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Context Controller Unit Tests Suite")
}
