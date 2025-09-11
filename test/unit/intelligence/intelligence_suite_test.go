package intelligence

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestIntelligence(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Intelligence - Business Requirements Testing Suite")
}
