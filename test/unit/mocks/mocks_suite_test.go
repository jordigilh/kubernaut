//go:build unit
// +build unit

package mocks

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-MOCKS-SUITE-001: Mock Components Test Suite Organization
// Business Impact: Ensures comprehensive validation of mock components for testing
// Stakeholder Value: Provides executive confidence in testing infrastructure and mock reliability

func TestMocks(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Mock Components Unit Tests Suite")
}



