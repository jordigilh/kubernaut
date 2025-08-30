package model_comparison_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestModelComparison(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Model Comparison Test Suite")
}
