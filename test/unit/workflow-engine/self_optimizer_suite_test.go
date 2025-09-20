package workflowengine

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestSelfOptimizer(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Self Optimizer Unit Tests Suite")
}
