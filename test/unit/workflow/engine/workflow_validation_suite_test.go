package engine

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestWorkflowValidation(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Workflow Validation Suite")
}
