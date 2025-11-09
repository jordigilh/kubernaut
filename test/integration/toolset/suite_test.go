package toolset

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestToolsetIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Dynamic Toolset Integration Test Suite")
}

