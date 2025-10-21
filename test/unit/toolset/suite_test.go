package toolset

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestToolset(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Toolset Unit Test Suite")
}
