package severity_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestSeverity(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Severity Suite")
}
