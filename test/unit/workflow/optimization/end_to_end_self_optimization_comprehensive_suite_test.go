package optimization

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestEndToEndSelfOptimizationComprehensive(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "End-to-End Self Optimization Comprehensive Suite")
}
