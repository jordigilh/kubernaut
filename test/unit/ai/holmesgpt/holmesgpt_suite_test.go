package holmesgpt_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestHolmesGPT(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "HolmesGPT Client - Business Requirements Validation Suite")
}
