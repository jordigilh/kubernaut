package prometheus_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestKubernautAgentPrometheusToolsIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Kubernaut Agent Prometheus Tools Integration Suite — #433")
}
