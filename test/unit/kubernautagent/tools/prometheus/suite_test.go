package prometheus_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestKubernautAgentPrometheusTools(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Kubernaut Agent Prometheus Tools Unit Suite — #433")
}
