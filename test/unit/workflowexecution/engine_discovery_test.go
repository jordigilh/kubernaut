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

package workflowexecution

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	weconfig "github.com/jordigilh/kubernaut/pkg/workflowexecution/config"
	"github.com/jordigilh/kubernaut/pkg/workflowexecution/executor"
)

// ========================================
// Issue #868: CRD-Aware Engine Registration
// Tests: TektonConfig, TektonEnabled helper, engine availability summary
// ========================================

var _ = Describe("TektonConfig (Issue #868)", func() {

	Context("DefaultConfig", func() {
		It("UT-WE-868-001: default config should have nil Tekton (auto-discovery)", func() {
			cfg := weconfig.DefaultConfig()
			Expect(cfg.Tekton).To(BeNil(), "Tekton config should be nil by default (auto-discovery)")
		})
	})

	Context("TektonEnabled", func() {
		It("UT-WE-868-002: should return true when Tekton config is nil (auto-discovery default)", func() {
			cfg := weconfig.DefaultConfig()
			Expect(cfg.TektonEnabled()).To(BeTrue())
		})

		It("UT-WE-868-003: should return true when Tekton.Enabled is nil (auto-discovery)", func() {
			cfg := weconfig.DefaultConfig()
			cfg.Tekton = &weconfig.TektonConfig{}
			Expect(cfg.TektonEnabled()).To(BeTrue())
		})

		It("UT-WE-868-004: should return true when Tekton.Enabled is explicitly true", func() {
			cfg := weconfig.DefaultConfig()
			enabled := true
			cfg.Tekton = &weconfig.TektonConfig{Enabled: &enabled}
			Expect(cfg.TektonEnabled()).To(BeTrue())
		})

		It("UT-WE-868-005: should return false when Tekton.Enabled is explicitly false", func() {
			cfg := weconfig.DefaultConfig()
			disabled := false
			cfg.Tekton = &weconfig.TektonConfig{Enabled: &disabled}
			Expect(cfg.TektonEnabled()).To(BeFalse())
		})
	})
})

var _ = Describe("Registry.Get error for unavailable engines (Issue #868)", func() {
	It("UT-WE-868-020: Get for unregistered tekton returns actionable error", func() {
		registry := executor.NewRegistry()
		registry.Register("job", nil)
		_, err := registry.Get("tekton")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("tekton"))
		Expect(err.Error()).To(ContainSubstring("unsupported execution engine"))
	})

	It("UT-WE-868-021: Get for unregistered ansible returns actionable error", func() {
		registry := executor.NewRegistry()
		registry.Register("job", nil)
		_, err := registry.Get("ansible")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("ansible"))
	})
})

var _ = Describe("Engine Availability Summary (Issue #868)", func() {

	Context("Registry.EngineAvailability", func() {
		It("UT-WE-868-010: job-only registry reports tekton and ansible unavailable", func() {
			registry := executor.NewRegistry()
			registry.Register("job", nil) // nil executor is fine for registry test

			available, unavailable := registry.EngineAvailability([]string{"tekton", "ansible"})
			Expect(available).To(ConsistOf("job"))
			Expect(unavailable).To(ConsistOf("tekton", "ansible"))
		})

		It("UT-WE-868-011: all engines registered reports none unavailable", func() {
			registry := executor.NewRegistry()
			registry.Register("job", nil)
			registry.Register("tekton", nil)

			available, unavailable := registry.EngineAvailability([]string{"tekton"})
			Expect(available).To(ConsistOf("job", "tekton"))
			Expect(unavailable).To(BeEmpty())
		})

		It("UT-WE-868-012: empty known set reports only registered as available", func() {
			registry := executor.NewRegistry()
			registry.Register("job", nil)

			available, unavailable := registry.EngineAvailability(nil)
			Expect(available).To(ConsistOf("job"))
			Expect(unavailable).To(BeEmpty())
		})
	})
})
