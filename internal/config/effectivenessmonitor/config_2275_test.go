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

package effectivenessmonitor_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"

	"github.com/jordigilh/kubernaut/internal/config/effectivenessmonitor"
)

// BR-PLATFORM-012, Issue #2275: extend the shared Debug.PprofEnabled toggle
// (secure-by-default, off) to Effectiveness Monitor, a controller-runtime-
// managed service that had no pprof gate at all before this issue.
var _ = Describe("BR-PLATFORM-012: Debug.PprofEnabled secure default (Issue #2275)", func() {
	It("UT-EM-2275-001 DefaultConfig disables profiling by default (secure-by-default)", func() {
		cfg := effectivenessmonitor.DefaultConfig()
		Expect(cfg.Debug.PprofEnabled).To(BeFalse())
	})

	It("UT-EM-2275-002 YAML can opt in to profiling by setting debug.pprofEnabled: true", func() {
		cfg := effectivenessmonitor.DefaultConfig()
		data := []byte("debug:\n  pprofEnabled: true\n")
		Expect(yaml.Unmarshal(data, cfg)).To(Succeed())
		Expect(cfg.Debug.PprofEnabled).To(BeTrue())
	})
})
