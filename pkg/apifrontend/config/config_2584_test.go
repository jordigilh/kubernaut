package config_test

import (
	"github.com/jordigilh/kubernaut/pkg/apifrontend/config"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("BR-AI-088 / UT-AF-2584-001: policyRead configuration", func() {
	It("loads the operator-rendered names and required threshold", func() {
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

	DescribeTable("rejects an absent or invalid operator threshold",
		func(threshold string, want string) {
			cfg := validConfig()
			cfg.PolicyRead = config.PolicyReadConfig{
				AIAnalysis: config.PolicyReadReference{
					ConfigMapName:       "aianalysis-policy",
					ConfidenceThreshold: threshold,
				},
				SignalProcessing: config.PolicyReadReference{ConfigMapName: "signalprocessing-policy"},
			}
			Expect(cfg.Validate()).To(MatchError(ContainSubstring(want)))
		},
		Entry("missing", "", "confidenceThreshold is required"),
		Entry("zero", "0", "confidenceThreshold must be finite"),
		Entry("above one", "1.01", "confidenceThreshold must be finite"),
		Entry("nan", "NaN", "confidenceThreshold must be finite"),
	)

	It("rejects an invalid ConfigMap name without reading policy data", func() {
		cfg := validConfig()
		cfg.PolicyRead = config.PolicyReadConfig{
			AIAnalysis: config.PolicyReadReference{
				ConfigMapName:       "../raw-policy",
				ConfidenceThreshold: "0.85",
			},
			SignalProcessing: config.PolicyReadReference{ConfigMapName: "signalprocessing-policy"},
		}
		Expect(cfg.Validate()).To(MatchError(ContainSubstring("configMapName")))
	})
})
