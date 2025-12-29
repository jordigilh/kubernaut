/*
Copyright 2025 Jordi Gil.

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

package remediationorchestrator

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ro "github.com/jordigilh/kubernaut/pkg/remediationorchestrator"
)

var _ = Describe("Core Types (BR-ORCH-027, BR-ORCH-028)", func() {

	Describe("PhaseTimeouts", func() {

		It("should return sensible defaults from DefaultPhaseTimeouts (BR-ORCH-028)", func() {
			// Given: Request for default timeouts
			// When: We call DefaultPhaseTimeouts
			defaults := ro.DefaultPhaseTimeouts()

			// Then: Timeouts should be sensible per BR-ORCH-028
			Expect(defaults.Processing).To(Equal(5 * time.Minute))
			Expect(defaults.Analyzing).To(Equal(10 * time.Minute))
			Expect(defaults.Executing).To(Equal(30 * time.Minute))
			Expect(defaults.Global).To(Equal(60 * time.Minute)) // BR-ORCH-027
		})
	})

	Describe("OrchestratorConfig", func() {

		It("should return sensible defaults from DefaultConfig", func() {
			// Given: Request for default config
			// When: We call DefaultConfig
			config := ro.DefaultConfig()

			// Then: Config should have sensible defaults
			Expect(config.Timeouts.Global).To(Equal(60 * time.Minute))
			Expect(config.RetentionPeriod).To(Equal(24 * time.Hour))
			Expect(config.MaxConcurrentReconciles).To(Equal(10))
			Expect(config.EnableMetrics).To(BeTrue())
		})
	})
})
