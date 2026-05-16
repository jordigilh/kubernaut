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
}

func (s *fakeScenario) Name() string { return s.name }

func (s *fakeScenario) Match(_ *scenarios.DetectionContext) (bool, float64) {
	if s.confidence > 0 {
		return true, s.confidence
	}
	return false, 0
}

func (s *fakeScenario) Metadata() scenarios.ScenarioMetadata {
	return scenarios.ScenarioMetadata{Name: s.name}
}

func (s *fakeScenario) DAG() *conversation.DAG { return nil }
