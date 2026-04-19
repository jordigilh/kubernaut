package naming_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestNaming(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Naming Convention Lint Suite")
}
