package embedding

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestAIEmbeddingUnit(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AI Embedding Unit Test Suite")
}
