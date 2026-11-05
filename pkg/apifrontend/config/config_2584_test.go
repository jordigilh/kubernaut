package config_test

import (
	"github.com/jordigilh/kubernaut/pkg/apifrontend/config"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("BR-AI-088 / UT-AF-2584-001: runtime policy metadata compatibility", func() {
	It("loads a legacy policyRead block without making it a validation dependency", func() {
		cfg, err := config.Load([]byte(`policyRead:
  aiAnalysis:
    configMapName: aianalysis-policy
    confidenceThreshold: "0.85"
  signalProcessing:
    configMapName: signalprocessing-policy
`))
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.PolicyRead.AIAnalysis.ConfigMapName).To(Equal("aianalysis-policy"))
		Expect(cfg.PolicyRead.AIAnalysis.ConfidenceThreshold).To(Equal("0.85"))
		Expect(cfg.PolicyRead.SignalProcessing.ConfigMapName).To(Equal("signalprocessing-policy"))
		Expect(cfg.Validate()).To(Succeed())
	})

	It("accepts an absent legacy block", func() {
		cfg := validConfig()
		Expect(cfg.Validate()).To(Succeed())
	})
})
