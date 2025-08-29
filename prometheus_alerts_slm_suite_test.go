package prometheus_alerts_slm_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestPrometheusAlertsSlm(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "PrometheusAlertsSlm Suite")
}
