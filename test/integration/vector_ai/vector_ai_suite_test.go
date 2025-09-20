//go:build integration
// +build integration

package vector_ai

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestVectorAI(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Vector Database + AI Integration Suite")
}


