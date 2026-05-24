package config_test

import (
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/config"
)

var _ = Describe("ApplyPortEnvOverride (BR-PLATFORM-1262)", Label("config", "cm-6"), func() {
	DescribeTable("CM-6/SI-10: environment-injected port override",
		func(envValue string, setEnv bool, initialPort, expectedPort int) {
			cfg := &config.Config{}
			cfg.Server.Port = initialPort
			if setEnv {
				Expect(os.Setenv("PORT", envValue)).To(Succeed())
				DeferCleanup(os.Unsetenv, "PORT")
			} else {
				Expect(os.Unsetenv("PORT")).To(Succeed())
			}
			config.ApplyPortEnvOverride(cfg)
			Expect(cfg.Server.Port).To(Equal(expectedPort))
		},
		Entry("UT-AF-1262-001 CM-6: valid PORT overrides config", "8444", true, 8443, 8444),
		Entry("UT-AF-1262-002 CM-6: unset PORT preserves config", "", false, 8443, 8443),
		Entry("UT-AF-1262-003 SI-10: non-numeric PORT ignored", "abc", true, 8443, 8443),
		Entry("UT-AF-1262-004 SI-10: PORT=0 out-of-range ignored", "0", true, 8443, 8443),
		Entry("UT-AF-1262-005 SI-10: PORT>65535 ignored", "99999", true, 8443, 8443),
		Entry("UT-AF-1262-006 SI-10: empty string PORT ignored", "", true, 8443, 8443),
		Entry("UT-AF-1262-007 CM-6: PORT=1 minimum valid", "1", true, 8443, 1),
		Entry("UT-AF-1262-008 CM-6: PORT=65535 maximum valid", "65535", true, 8443, 65535),
	)
})
