package insights

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestEffectiveness(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Effectiveness Suite")
}
