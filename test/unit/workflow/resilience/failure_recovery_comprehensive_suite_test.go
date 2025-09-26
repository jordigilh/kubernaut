//go:build unit
// +build unit

package resilience

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestFailureRecoveryComprehensive(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Failure Recovery Comprehensive Unit Test Suite")
}
