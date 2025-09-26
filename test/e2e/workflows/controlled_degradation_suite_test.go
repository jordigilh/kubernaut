package workflows

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestControlledDegradationScenarios(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Controlled Graceful Degradation E2E Test Suite")
}


