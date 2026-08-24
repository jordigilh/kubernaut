/*
Copyright 2026 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package config_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"

	"github.com/jordigilh/kubernaut/pkg/fleet/fmc/config"
)

// BR-PLATFORM-012, Issue #2275: FMC is the one service whose buildFMCServers
// hardcoded enableProfiling=true (profiling always ON), unlike the other 12
// services which defaulted OFF. This closes that gap -- FMC now shares the
// same secure-by-default Debug.PprofEnabled toggle as everyone else.
var _ = Describe("BR-PLATFORM-012: Debug.PprofEnabled secure default (Issue #2275)", func() {
	It("UT-FMC-2275-001 DefaultServiceConfig disables profiling by default (secure-by-default, was hardcoded-on pre-#2275)", func() {
		cfg := config.DefaultServiceConfig()
		Expect(cfg.Debug.PprofEnabled).To(BeFalse())
	})

	It("UT-FMC-2275-002 YAML can opt in to profiling by setting debug.pprofEnabled: true", func() {
		cfg := config.DefaultServiceConfig()
		Expect(yaml.Unmarshal([]byte("debug:\n  pprofEnabled: true\n"), cfg)).To(Succeed())
		Expect(cfg.Debug.PprofEnabled).To(BeTrue())
	})

	It("UT-FMC-2275-003 omitted debug in YAML keeps the secure default", func() {
		cfg := config.DefaultServiceConfig()
		Expect(yaml.Unmarshal([]byte("server:\n  apiAddr: \":9999\"\n"), cfg)).To(Succeed())
		Expect(cfg.Debug.PprofEnabled).To(BeFalse())
	})
})
