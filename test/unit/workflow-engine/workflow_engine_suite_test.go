package workflowengine

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestWorkflowEngine(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Workflow Engine - Business Requirements Testing Suite")
}
