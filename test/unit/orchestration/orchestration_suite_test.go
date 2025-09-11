package orchestration

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestOrchestration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Orchestration - Business Requirements Testing Suite")
}
