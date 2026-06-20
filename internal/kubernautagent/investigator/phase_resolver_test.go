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

package investigator_test

import (
	"context"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// noopClient is a minimal llm.Client for resolver tests that don't need LLM behavior.
type noopClient struct{}

func (n *noopClient) Chat(_ context.Context, _ llm.ChatRequest) (llm.ChatResponse, error) {
	return llm.ChatResponse{}, nil
}
func (n *noopClient) StreamChat(_ context.Context, _ llm.ChatRequest, _ func(llm.ChatStreamEvent) error) (llm.ChatResponse, error) {
	return llm.ChatResponse{}, nil
}
func (n *noopClient) Close() error { return nil }

var _ = Describe("Per-Phase LLM Model Routing — Phase Resolver #1470", func() {

	Describe("IT-AI-1470-001: PhaseClientResolver interface satisfied by DefaultPhaseResolver", func() {
		It("should satisfy the interface at compile time", func() {
			// compile-time assertion already in phase_resolver.go (var _ PhaseClientResolver = ...)
			// Runtime verification:
			inner := &noopClient{}
			sw, err := llm.NewSwappableClient(inner, "default-model")
			Expect(err).NotTo(HaveOccurred())
			var resolver investigator.PhaseClientResolver = investigator.NewDefaultPhaseResolver(sw, nil)
			Expect(resolver).NotTo(BeNil())
		})
	})

	Describe("UT-AI-1470-005a: ResolvePhase with nil pinDecorator falls back to InstrumentedClient", func() {
		It("should return a non-nil client without panic", func() {
			inner := &noopClient{}
			sw, err := llm.NewSwappableClient(inner, "default-model")
			Expect(err).NotTo(HaveOccurred())

			resolver := investigator.NewDefaultPhaseResolver(sw, nil)
			client, modelName, _ := resolver.ResolvePhase(katypes.PhaseRCA)
			Expect(client).NotTo(BeNil())
			Expect(modelName).To(Equal("default-model"))
		})
	})

	Describe("UT-AI-1470-005b: ResolvePhase with empty phase map returns default client", func() {
		It("should return default model name for any phase", func() {
			inner := &noopClient{}
			sw, err := llm.NewSwappableClient(inner, "default-model")
			Expect(err).NotTo(HaveOccurred())

			resolver := investigator.NewDefaultPhaseResolver(sw, nil)
			_, modelName, _ := resolver.ResolvePhase(katypes.PhaseWorkflowDiscovery)
			Expect(modelName).To(Equal("default-model"))
		})
	})

	Describe("UT-AI-1470-005c: ResolvePhase for unknown phase returns default", func() {
		It("should fall back to default when phase override does not exist", func() {
			inner := &noopClient{}
			sw, err := llm.NewSwappableClient(inner, "default-model")
			Expect(err).NotTo(HaveOccurred())

			phaseInner := &noopClient{}
			phaseSw, err := llm.NewSwappableClient(phaseInner, "wf-fast-model")
			Expect(err).NotTo(HaveOccurred())

			resolver := investigator.NewDefaultPhaseResolver(sw, nil)
			resolver.SetPhaseSwappable(katypes.PhaseWorkflowDiscovery, phaseSw)

			_, modelName, _ := resolver.ResolvePhase(katypes.PhaseValidation)
			Expect(modelName).To(Equal("default-model"))
		})
	})

	Describe("IT-AI-1470-002a: Default client used when no phase override exists", func() {
		It("should return default model when phase has no override", func() {
			inner := &noopClient{}
			sw, err := llm.NewSwappableClient(inner, "base-reasoning-model")
			Expect(err).NotTo(HaveOccurred())

			resolver := investigator.NewDefaultPhaseResolver(sw, nil)
			_, modelName, _ := resolver.ResolvePhase(katypes.PhaseRCA)
			Expect(modelName).To(Equal("base-reasoning-model"))
		})
	})

	Describe("IT-AI-1470-002b: Phase-specific client used when override exists", func() {
		It("should return phase-specific model name", func() {
			inner := &noopClient{}
			sw, err := llm.NewSwappableClient(inner, "base-reasoning-model")
			Expect(err).NotTo(HaveOccurred())

			phaseInner := &noopClient{}
			phaseSw, err := llm.NewSwappableClient(phaseInner, "fast-haiku")
			Expect(err).NotTo(HaveOccurred())

			resolver := investigator.NewDefaultPhaseResolver(sw, nil)
			resolver.SetPhaseSwappable(katypes.PhaseWorkflowDiscovery, phaseSw)

			_, modelName, _ := resolver.ResolvePhase(katypes.PhaseWorkflowDiscovery)
			Expect(modelName).To(Equal("fast-haiku"))
		})
	})

	Describe("IT-AI-1470-002c: PinDecorator is applied to the resolved client", func() {
		It("should call the decorator for both default and phase-specific clients", func() {
			var decoratorCalls atomic.Int32
			decorator := func(c llm.Client) llm.Client {
				decoratorCalls.Add(1)
				return llm.NewInstrumentedClient(c)
			}

			inner := &noopClient{}
			sw, err := llm.NewSwappableClient(inner, "default-model")
			Expect(err).NotTo(HaveOccurred())

			phaseInner := &noopClient{}
			phaseSw, err := llm.NewSwappableClient(phaseInner, "fast-model")
			Expect(err).NotTo(HaveOccurred())

			resolver := investigator.NewDefaultPhaseResolver(sw, decorator)
			resolver.SetPhaseSwappable(katypes.PhaseWorkflowDiscovery, phaseSw)

			resolver.ResolvePhase(katypes.PhaseRCA)
			resolver.ResolvePhase(katypes.PhaseWorkflowDiscovery)
			Expect(decoratorCalls.Load()).To(BeNumerically(">=", 2),
				"PinDecorator must be applied for both default and phase-specific clients")
		})
	})

	Describe("IT-AI-1470-002d: SetPhaseSwappable / RemovePhaseSwappable reflected", func() {
		It("should add and then remove phase-specific override", func() {
			inner := &noopClient{}
			sw, err := llm.NewSwappableClient(inner, "default-model")
			Expect(err).NotTo(HaveOccurred())

			phaseInner := &noopClient{}
			phaseSw, err := llm.NewSwappableClient(phaseInner, "fast-model")
			Expect(err).NotTo(HaveOccurred())

			resolver := investigator.NewDefaultPhaseResolver(sw, nil)

			// Before set: default model
			_, modelName, _ := resolver.ResolvePhase(katypes.PhaseWorkflowDiscovery)
			Expect(modelName).To(Equal("default-model"))

			// After set: phase-specific model
			resolver.SetPhaseSwappable(katypes.PhaseWorkflowDiscovery, phaseSw)
			_, modelName, _ = resolver.ResolvePhase(katypes.PhaseWorkflowDiscovery)
			Expect(modelName).To(Equal("fast-model"))

			// After remove: back to default
			resolver.RemovePhaseSwappable(katypes.PhaseWorkflowDiscovery)
			_, modelName, _ = resolver.ResolvePhase(katypes.PhaseWorkflowDiscovery)
			Expect(modelName).To(Equal("default-model"))

			// Phases list should be empty after removal
			Expect(resolver.Phases()).To(BeEmpty())
		})
	})
})
