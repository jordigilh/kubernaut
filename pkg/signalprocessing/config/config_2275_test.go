package config_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"

	config "github.com/jordigilh/kubernaut/pkg/signalprocessing/config"
)

// BR-PLATFORM-012, Issue #2275: extend the shared Debug.PprofEnabled toggle
// (secure-by-default, off) to SignalProcessing, a controller-runtime-managed
// service that had no pprof gate at all before this issue.
var _ = Describe("BR-PLATFORM-012: Debug.PprofEnabled secure default (Issue #2275)", func() {
	It("UT-SP-2275-001 DefaultConfig disables profiling by default (secure-by-default)", func() {
		cfg := config.DefaultConfig()
		Expect(cfg.Debug.PprofEnabled).To(BeFalse())
	})

	It("UT-SP-2275-002 YAML can opt in to profiling by setting debug.pprofEnabled: true", func() {
		cfg := config.DefaultConfig()
		Expect(yaml.Unmarshal([]byte("debug:\n  pprofEnabled: true\n"), cfg)).To(Succeed())
		Expect(cfg.Debug.PprofEnabled).To(BeTrue())
	})
})
