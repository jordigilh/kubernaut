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
package mockllm_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/shared/uuid"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/config"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/conversation"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/scenarios"
)

var _ = Describe("Scenario Registry", func() {

	var registry *scenarios.Registry

	BeforeEach(func() {
		registry = scenarios.NewRegistry()
	})

	Describe("UT-MOCK-020-001: Scenario registered via Register is discoverable by Get", func() {
		It("should find a registered scenario by name", func() {
			s := &fakeScenario{name: "test_scenario", confidence: 1.0}
			registry.Register(s)

			found, ok := registry.Get("test_scenario")
			Expect(ok).To(BeTrue())
			Expect(found.Name()).To(Equal("test_scenario"))
		})

		It("should return false for unregistered name", func() {
			_, ok := registry.Get("nonexistent")
			Expect(ok).To(BeFalse())
		})
	})

	Describe("UT-MOCK-020-002: Detect returns highest-confidence match", func() {
		It("should select the scenario with the highest confidence", func() {
			low := &fakeScenario{name: "low", confidence: 0.3}
			high := &fakeScenario{name: "high", confidence: 0.9}
			registry.Register(low)
			registry.Register(high)

			ctx := &scenarios.DetectionContext{Content: "something"}
			result := registry.Detect(ctx)
			Expect(result).NotTo(BeNil())
			Expect(result.Scenario.Name()).To(Equal("high"))
			Expect(result.Confidence).To(BeNumerically(">=", 0.9))
		})

		It("should return nil when no scenario matches", func() {
			nomatch := &fakeScenario{name: "nomatch", confidence: 0.0}
			registry.Register(nomatch)

			ctx := &scenarios.DetectionContext{Content: "something"}
			result := registry.Detect(ctx)
			Expect(result).To(BeNil())
		})
	})

	Describe("UT-MOCK-2390-001: Detect honors caller and phase scope", func() {
		It("should select only the scenario scoped to the request caller", func() {
			af := &fakeScenario{
				name:       "af_investigate",
				confidence: 1.0,
				caller:     scenarios.CallerAF,
				phase:      scenarios.PhaseInvestigation,
			}
			ka := &fakeScenario{
				name:       "ka_investigate",
				confidence: 1.0,
				caller:     scenarios.CallerKA,
				phase:      scenarios.PhaseRCA,
			}
			registry.Register(af)
			registry.Register(ka)

			result := registry.Detect(&scenarios.DetectionContext{
				Caller: scenarios.CallerKA,
				Phase:  scenarios.PhaseRCA,
			})
			Expect(result).NotTo(BeNil())
			Expect(result.Scenario.Name()).To(Equal("ka_investigate"))
		})

		It("should not apply a scoped scenario when the request scope is unknown", func() {
			scoped := &fakeScenario{
				name:       "af_investigate",
				confidence: 1.0,
				caller:     scenarios.CallerAF,
				phase:      scenarios.PhaseInvestigation,
			}
			fallback := &fakeScenario{name: "legacy", confidence: 0.5}
			registry.Register(scoped)
			registry.Register(fallback)

			result := registry.Detect(&scenarios.DetectionContext{Content: "investigate"})
			Expect(result).NotTo(BeNil())
			Expect(result.Scenario.Name()).To(Equal("legacy"))
		})
	})

	It("UT-MOCK-2390-003: should wire YAML caller and phase fields into detection", func() {
		registry = scenarios.DefaultRegistryFull(&config.Overrides{
			KeywordScenarios: []config.KeywordScenarioOverride{{
				Name:     "af_scoped",
				Caller:   "af",
				Phase:    "investigation",
				Keywords: []string{"same keyword"},
			}},
		}, "")

		result := registry.Detect(&scenarios.DetectionContext{
			Content: "same keyword",
			Caller:  scenarios.CallerAF,
			Phase:   scenarios.PhaseInvestigation,
		})
		Expect(result).NotTo(BeNil())
		Expect(result.Scenario.Name()).To(Equal("af_scoped"))

		result = registry.Detect(&scenarios.DetectionContext{
			Content: "same keyword",
			Caller:  scenarios.CallerKA,
			Phase:   scenarios.PhaseRCA,
		})
		Expect(result).NotTo(BeNil())
		Expect(result.Scenario.Name()).To(Equal("default"))
	})

	It("UT-MOCK-2442-018: rejects ambiguous workflow overrides", func() {
		registry = scenarios.DefaultRegistryWithOverrides(&config.Overrides{
			Scenarios: map[string]config.ScenarioOverride{
				"oomkill-increase-memory-v1:production": {WorkflowID: "production-id"},
				"oomkill-increase-memory-v1:staging":    {WorkflowID: "staging-id"},
				"oomkill-increase-memory-v1:test":       {WorkflowID: "test-id"},
			},
		})

		scenario, ok := registry.Get("oomkilled")
		Expect(ok).To(BeTrue())
		configured, ok := scenario.(scenarios.ScenarioWithConfig)
		Expect(ok).To(BeTrue())
		Expect(configured.Config().WorkflowID).To(Equal(uuid.DeterministicUUID("oomkill-increase-memory-v1")))
	})

	It("UT-MOCK-2442-019: applies identical workflow overrides across environments", func() {
		registry = scenarios.DefaultRegistryWithOverrides(&config.Overrides{
			Scenarios: map[string]config.ScenarioOverride{
				"oomkill-increase-memory-v1:production": {WorkflowID: "catalog-id"},
				"oomkill-increase-memory-v1:staging":    {WorkflowID: "catalog-id"},
				"oomkill-increase-memory-v1:test":       {WorkflowID: "catalog-id"},
			},
		})

		scenario, ok := registry.Get("oomkilled")
		Expect(ok).To(BeTrue())
		configured, ok := scenario.(scenarios.ScenarioWithConfig)
		Expect(ok).To(BeTrue())
		Expect(configured.Config().WorkflowID).To(Equal("catalog-id"))
	})

	It("UT-MOCK-2442-020: keeps max-retry workflow discoverable while forcing validation failures", func() {
		registry = scenarios.DefaultRegistry()

		scenario, ok := registry.Get("max_retries_exhausted")
		Expect(ok).To(BeTrue())
		configured, ok := scenario.(scenarios.ScenarioWithConfig)
		Expect(ok).To(BeTrue())

		cfg := configured.Config()
		Expect(cfg.ActionType).To(Equal("IncreaseMemoryLimits"))
		Expect(cfg.WorkflowID).To(Equal(uuid.DeterministicUUID("oomkill-increase-memory-v1")))
		Expect(cfg.RawParameters).To(HaveKeyWithValue("MEMORY_LIMIT_NEW", BeNumerically("==", 123)))
	})

	It("UT-MOCK-2442-021: selects isolated AIAnalysis fixture scenarios by fingerprint", func() {
		registry = scenarios.DefaultRegistryWithOverrides(&config.Overrides{
			Scenarios: map[string]config.ScenarioOverride{
				"oomkill-increase-memory-aa-staging-v1:staging": {WorkflowID: "catalog-id"},
			},
		})

		result := registry.Detect(&scenarios.DetectionContext{Content: "e2e-fingerprint-002"})
		Expect(result).NotTo(BeNil())
		Expect(result.Scenario.Name()).To(Equal("aa_e2e_staging_oom"))

		configured, ok := result.Scenario.(scenarios.ScenarioWithConfig)
		Expect(ok).To(BeTrue())
		Expect(configured.Config().WorkflowID).To(Equal("catalog-id"))
	})

	It("UT-MOCK-2442-022: selects isolated AIAnalysis fixtures from prompt-visible fields", func() {
		registry = scenarios.DefaultRegistryWithOverrides(&config.Overrides{
			Scenarios: map[string]config.ScenarioOverride{
				"crashloop-config-fix-aa-approval-v1:production": {WorkflowID: "catalog-id"},
			},
		})

		result := registry.Detect(&scenarios.DetectionContext{
			Content: "- Signal Name: CrashLoopBackOff\n- Severity: critical\n- Resource: payments/Deployment/payment-service",
		})
		Expect(result).NotTo(BeNil())
		Expect(result.Scenario.Name()).To(Equal("aa_e2e_approval_crashloop"))

		configured, ok := result.Scenario.(scenarios.ScenarioWithConfig)
		Expect(ok).To(BeTrue())
		Expect(configured.Config().WorkflowID).To(Equal("catalog-id"))
	})

	It("UT-MOCK-2442-023: selects the warning production audit fixture", func() {
		registry = scenarios.DefaultRegistry()

		result := registry.Detect(&scenarios.DetectionContext{
			Content: "Signal Name: CrashLoopBackOff Severity: warning Namespace: payments Resource Name: payment-service",
		})
		Expect(result).NotTo(BeNil())
		Expect(result.Scenario.Name()).To(Equal("aa_e2e_audit_crashloop"))
	})

	It("UT-MOCK-2442-024: selects the warning staging Rego fixture", func() {
		registry = scenarios.DefaultRegistry()

		result := registry.Detect(&scenarios.DetectionContext{
			Content: "Signal Name: CrashLoopBackOff Severity: warning Namespace: default Resource Name: frontend",
		})
		Expect(result).NotTo(BeNil())
		Expect(result.Scenario.Name()).To(Equal("aa_e2e_rego_crashloop"))
	})

	It("UT-MOCK-2442-025: matches detected-label fixtures in dynamic namespaces", func() {
		registry = scenarios.DefaultRegistry()

		ctx := &scenarios.DetectionContext{
			Content: "Signal Name: CrashLoopBackOff Severity: critical Resource: adr056-e2e-1234/Deployment/app-e2e-001",
		}
		fixture, ok := registry.Get("aa_e2e_detected_labels_crashloop")
		Expect(ok).To(BeTrue())
		configured, ok := fixture.(scenarios.ScenarioWithConfig)
		Expect(ok).To(BeTrue())
		Expect(configured.Config().ResourceName).To(Equal("app-e2e-001"))
		Expect(configured.Config().ResourceNS).To(BeEmpty())
		matched, confidence := fixture.Match(ctx)
		Expect(matched).To(BeTrue(), "detected-label fixture confidence=%v", confidence)

		result := registry.Detect(ctx)
		Expect(result).NotTo(BeNil())
		Expect(result.Scenario.Name()).To(Equal("aa_e2e_detected_labels_crashloop"))
	})

	It("UT-MOCK-2442-026: matches AIAnalysis fixtures when structured fields are only in accumulated prompt text", func() {
		registry = scenarios.DefaultRegistry()

		result := registry.Detect(&scenarios.DetectionContext{
			Content: "RCA findings: configuration regression. Select the appropriate remediation workflow.",
			AllText: "# Workflow Selection Request\n- Signal Name: CrashLoopBackOff\n- Severity: critical\n- Resource: adr056-e2e-1234/Deployment/app-e2e-001",
		})
		Expect(result).NotTo(BeNil())
		Expect(result.Scenario.Name()).To(Equal("aa_e2e_detected_labels_crashloop"))
	})

	It("UT-MOCK-2442-027: matches AIAnalysis fixtures when prompt fields are serialized", func() {
		registry = scenarios.DefaultRegistry()

		result := registry.Detect(&scenarios.DetectionContext{
			AllText: `{"signal_name":"CrashLoopBackOff","severity":"critical","resource":"adr056-e2e-1234/Deployment/app-e2e-001"}`,
		})
		Expect(result).NotTo(BeNil())
		Expect(result.Scenario.Name()).To(Equal("aa_e2e_detected_labels_crashloop"))
	})

	Describe("UT-MOCK-020-003: List returns metadata for all registered scenarios", func() {
		It("should return metadata entries for each registered scenario", func() {
			s1 := &fakeScenario{name: "alpha", confidence: 0.5}
			s2 := &fakeScenario{name: "beta", confidence: 0.7}
			registry.Register(s1)
			registry.Register(s2)

			list := registry.List()
			Expect(list).To(HaveLen(2))

			names := make([]string, len(list))
			for i, m := range list {
				names[i] = m.Name
			}
			Expect(names).To(ContainElements("alpha", "beta"))
		})
	})
})

// fakeScenario is a test double implementing scenarios.Scenario.
type fakeScenario struct {
	name       string
	confidence float64
	caller     scenarios.Caller
	phase      scenarios.Phase
}

func (s *fakeScenario) Name() string { return s.name }

func (s *fakeScenario) Match(_ *scenarios.DetectionContext) (bool, float64) {
	if s.confidence > 0 {
		return true, s.confidence
	}
	return false, 0
}

func (s *fakeScenario) Metadata() scenarios.ScenarioMetadata {
	return scenarios.ScenarioMetadata{Name: s.name, Caller: s.caller, Phase: s.phase}
}

func (s *fakeScenario) DAG() *conversation.DAG { return nil }
