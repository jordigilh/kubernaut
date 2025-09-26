//go:build unit
// +build unit

package resilience

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestErrorRecoveryComprehensive(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Error Recovery Comprehensive Unit Test Suite")
}
