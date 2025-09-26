//go:build unit
// +build unit

package optimization

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestAdaptiveResourceAllocationComprehensive(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Adaptive Resource Allocation Comprehensive Unit Test Suite")
}
