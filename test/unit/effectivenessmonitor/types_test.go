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

package effectivenessmonitor

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/config"
	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/types"
)

var _ = Describe("Core Types (BR-EM-001 through BR-EM-008)", func() {

	// ========================================
	// Component Type Constants
	// ========================================
	Describe("ComponentType constants", func() {

		It("should define all four component types", func() {
			Expect(string(types.ComponentHealth)).To(Equal("health"))
			Expect(string(types.ComponentAlert)).To(Equal("alert"))
			Expect(string(types.ComponentMetrics)).To(Equal("metrics"))
			Expect(string(types.ComponentHash)).To(Equal("hash"))
		})
	})

	// ========================================
	// Audit Event Type Constants
	// ========================================
	Describe("AuditEventType constants", func() {

		It("should define all five audit event types", func() {
			Expect(string(types.AuditHealthAssessed)).To(Equal("effectiveness.health.assessed"))
			Expect(string(types.AuditHashComputed)).To(Equal("effectiveness.hash.computed"))
			Expect(string(types.AuditAlertAssessed)).To(Equal("effectiveness.alert.assessed"))
			Expect(string(types.AuditMetricsAssessed)).To(Equal("effectiveness.metrics.assessed"))
			Expect(string(types.AuditAssessmentCompleted)).To(Equal("effectiveness.assessment.completed"))
		})
	})

	// ========================================
	// ComponentResult
	// ========================================
	Describe("ComponentResult", func() {

		It("should represent an assessed component with score", func() {
			score := 0.85
			result := types.ComponentResult{
				Component: types.ComponentHealth,
				Assessed:  true,
				Score:     &score,
				Details:   "all pods ready",
			}

			Expect(result.Component).To(Equal(types.ComponentHealth))
			Expect(result.Assessed).To(BeTrue())
			Expect(*result.Score).To(Equal(0.85))
			Expect(result.Error).To(BeNil())
		})

		It("should represent an unassessed component", func() {
			result := types.ComponentResult{
				Component: types.ComponentMetrics,
				Assessed:  false,
				Score:     nil,
				Details:   "Prometheus unavailable",
			}

			Expect(result.Assessed).To(BeFalse())
			Expect(result.Score).To(BeNil())
		})
	})

	// ========================================
	// Timeout Constants (REFACTOR-EM-001)
	// ========================================
	Describe("Timeout Constants", func() {

		It("should define RequeueStabilizationPending as 30s", func() {
			Expect(config.RequeueStabilizationPending).To(Equal(30 * time.Second))
		})

		It("should define RequeueAssessmentInProgress as 15s", func() {
			Expect(config.RequeueAssessmentInProgress).To(Equal(15 * time.Second))
		})

		It("should define RequeueGenericError as 5s", func() {
			Expect(config.RequeueGenericError).To(Equal(5 * time.Second))
		})

		It("should define RequeueExternalServiceDown as 30s", func() {
			Expect(config.RequeueExternalServiceDown).To(Equal(30 * time.Second))
		})
	})

	// ========================================
	// UT-EM-CF-009: MaxConcurrentAssessments
	// Tested via config.AssessmentConfig defaults
	// ========================================
	Describe("Default Assessment Config", func() {

		It("should provide valid defaults for all assessment parameters", func() {
			cfg := config.DefaultAssessmentConfig()

			Expect(cfg.StabilizationWindow).To(Equal(5 * time.Minute))
			Expect(cfg.ValidityWindow).To(Equal(30 * time.Minute))
			Expect(cfg.PrometheusEnabled).To(BeTrue())
			Expect(cfg.AlertManagerEnabled).To(BeTrue())
		})
	})
})
