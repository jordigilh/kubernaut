package configuration

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-INFRASTRUCTURE-SUITE-001: Infrastructure Configuration Test Suite Organization
// Business Impact: Ensures infrastructure configuration and platform executor business requirements are systematically validated
// Stakeholder Value: Executive confidence in infrastructure scalability and automated business operations

func TestInfrastructureConfiguration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Infrastructure Configuration Unit Tests Suite")
}
