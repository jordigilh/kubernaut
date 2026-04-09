package summarizer_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestKubernautAgentSummarizerIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Kubernaut Agent Summarizer Integration Suite — #433")
}
