package config_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"

	config "github.com/jordigilh/kubernaut/pkg/gateway/config"
)

// BR-PLATFORM-012, Issue #2275: rename Server.DisableProfiling (default
// false = profiling ON) to shared, positive-polarity Debug.PprofEnabled
// (default false = profiling OFF). Gateway is switching from
// profiling-ON-by-default to secure-by-default, consistent with the other
// 12 services.
var _ = Describe("BR-PLATFORM-012: Debug.PprofEnabled secure default (Issue #2275)", func() {
	It("UT-GW-2275-001 DefaultServerConfig disables profiling by default (secure-by-default)", func() {
		cfg := config.DefaultServerConfig()
		Expect(cfg.Debug.PprofEnabled).To(BeFalse())
	})

	It("UT-GW-2275-002 YAML can opt in to profiling by setting debug.pprofEnabled: true", func() {
		cfg := config.DefaultServerConfig()
		Expect(yaml.Unmarshal([]byte("debug:\n  pprofEnabled: true\n"), cfg)).To(Succeed())
		Expect(cfg.Debug.PprofEnabled).To(BeTrue())
	})

	It("UT-GW-2275-003 omitted debug in YAML keeps the secure default", func() {
		cfg := config.DefaultServerConfig()
		Expect(yaml.Unmarshal([]byte("server:\n  listenAddr: \":9999\"\n"), cfg)).To(Succeed())
		Expect(cfg.Debug.PprofEnabled).To(BeFalse())
	})
})
