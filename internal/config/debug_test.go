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

	"github.com/jordigilh/kubernaut/internal/config"
)

// BR-PLATFORM-012: Secure-by-default runtime profiling toggle.
// Issue #2275: rename disableProfiling (default true) -> positive-polarity
// debug.pprofEnabled (default false), shared across all 13 services so
// kubernaut-operator's CRD-level spec.<component>.debug.pprofEnabled maps
// 1:1 with no negation-translation layer.
var _ = Describe("Shared DebugConfig — BR-PLATFORM-012", func() {

	Describe("UT-CFG-2275-001: zero-value DebugConfig defaults PprofEnabled to false", func() {
		It("should default profiling OFF (Go zero value, AC-6 least privilege)", func() {
			var cfg config.DebugConfig
			Expect(cfg.PprofEnabled).To(BeFalse(),
				"AC-6: pprof must be off by default -- an operator must explicitly opt in")
		})
	})

	// UT-CFG-2275-002/003: PprofBindAddress is the single shared
	// implementation used by all 7 controller-runtime-managed services'
	// build*Manager functions (cmd/aianalysis, cmd/authwebhook,
	// cmd/effectivenessmonitor, cmd/notification,
	// cmd/remediationorchestrator, cmd/signalprocessing,
	// cmd/workflowexecution) to populate ctrl.Options.PprofBindAddress --
	// extracted here (REFACTOR) to eliminate 7 duplicate inline ternaries.
	Describe("UT-CFG-2275-002/003: PprofBindAddress maps enabled to a controller-runtime bind address", func() {
		It("UT-CFG-2275-002 returns :6060 when enabled", func() {
			Expect(config.PprofBindAddress(true)).To(Equal(":6060"))
		})

		It("UT-CFG-2275-003 returns empty string (disabled) when not enabled", func() {
			Expect(config.PprofBindAddress(false)).To(Equal(""))
		})
	})
})
