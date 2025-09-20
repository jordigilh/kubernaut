package main

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestPatternExtractorIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Main App Pattern Extractor Integration Suite")
}
