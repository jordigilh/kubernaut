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
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/phase"
)

// stubHandler is a minimal PhaseHandler for registry tests.
type stubHandler struct {
	phase  phase.Phase
	intent phase.TransitionIntent
}

func (s *stubHandler) Phase() phase.Phase { return s.phase }
func (s *stubHandler) Handle(_ context.Context, _ *remediationv1.RemediationRequest) (phase.TransitionIntent, error) {
	return s.intent, nil
}

var _ = Describe("Issue #666 - PhaseHandler Registry (BR-ORCH-025)", func() {

	var registry *phase.Registry

	BeforeEach(func() {
		registry = phase.NewRegistry()
	})

	// ========================================
	// Registration
	// ========================================
	Describe("Register", func() {

		It("UT-REG-001: registers a handler for a phase", func() {
			h := &stubHandler{phase: phase.Pending, intent: phase.NoOp("stub")}
			err := registry.Register(h)
			Expect(err).ToNot(HaveOccurred())
		})

		It("UT-REG-002: rejects duplicate registration for the same phase", func() {
			h1 := &stubHandler{phase: phase.Pending, intent: phase.NoOp("first")}
			h2 := &stubHandler{phase: phase.Pending, intent: phase.NoOp("second")}
			Expect(registry.Register(h1)).To(Succeed())
			err := registry.Register(h2)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Pending"))
		})

		It("UT-REG-003: allows registration of handlers for different phases", func() {
			h1 := &stubHandler{phase: phase.Pending}
			h2 := &stubHandler{phase: phase.Processing}
			h3 := &stubHandler{phase: phase.Analyzing}
			Expect(registry.Register(h1)).To(Succeed())
			Expect(registry.Register(h2)).To(Succeed())
			Expect(registry.Register(h3)).To(Succeed())
		})

		It("UT-REG-013: rejects nil handler registration", func() {
			err := registry.Register(nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("nil"))
		})
	})

	// ========================================
	// MustRegister
	// ========================================
	Describe("MustRegister", func() {

		It("UT-REG-004: succeeds for new phase", func() {
			h := &stubHandler{phase: phase.Executing}
			Expect(func() {
				registry.MustRegister(h)
			}).ToNot(Panic())
		})

		It("UT-REG-005: panics on duplicate phase", func() {
			h1 := &stubHandler{phase: phase.Executing}
			h2 := &stubHandler{phase: phase.Executing}
			registry.MustRegister(h1)
			Expect(func() {
				registry.MustRegister(h2)
			}).To(Panic())
		})
	})

	// ========================================
	// Lookup
	// ========================================
	Describe("Lookup", func() {

		It("UT-REG-006: returns registered handler", func() {
			expected := &stubHandler{
				phase:  phase.Analyzing,
				intent: phase.Advance(phase.Executing, "test"),
			}
			Expect(registry.Register(expected)).To(Succeed())
			h, ok := registry.Lookup(phase.Analyzing)
			Expect(ok).To(BeTrue())
			Expect(h).To(Equal(expected))
		})

		It("UT-REG-007: returns false for unregistered phase", func() {
			h, ok := registry.Lookup(phase.Verifying)
			Expect(ok).To(BeFalse())
			Expect(h).To(BeNil())
		})

		It("UT-REG-008: returns correct handler when multiple registered", func() {
			hPending := &stubHandler{phase: phase.Pending, intent: phase.NoOp("pending")}
			hProc := &stubHandler{phase: phase.Processing, intent: phase.NoOp("processing")}
			registry.MustRegister(hPending)
			registry.MustRegister(hProc)

			got, ok := registry.Lookup(phase.Processing)
			Expect(ok).To(BeTrue())
			Expect(got.Phase()).To(Equal(phase.Processing))
		})
	})

	// ========================================
	// Phases
	// ========================================
	Describe("Phases", func() {

		It("UT-REG-009: returns empty slice for empty registry", func() {
			Expect(registry.Phases()).To(BeEmpty())
		})

		It("UT-REG-010: returns all registered phases", func() {
			registry.MustRegister(&stubHandler{phase: phase.Pending})
			registry.MustRegister(&stubHandler{phase: phase.Analyzing})
			registry.MustRegister(&stubHandler{phase: phase.Executing})

			phases := registry.Phases()
			Expect(phases).To(HaveLen(3))
			Expect(phases).To(ContainElements(phase.Pending, phase.Analyzing, phase.Executing))
		})
	})

	// ========================================
	// Interface Compliance
	// ========================================
	Describe("Interface compliance", func() {

		It("UT-REG-011: stubHandler satisfies PhaseHandler interface", func() {
			var _ phase.PhaseHandler = &stubHandler{}
		})

		It("UT-REG-012: Handle returns the configured intent", func() {
			expected := phase.Advance(phase.Executing, "test intent")
			h := &stubHandler{phase: phase.Analyzing, intent: expected}

			got, err := h.Handle(context.Background(), &remediationv1.RemediationRequest{})
			Expect(err).ToNot(HaveOccurred())
			Expect(got.Type).To(Equal(phase.TransitionAdvance))
			Expect(got.TargetPhase).To(Equal(phase.Executing))
			Expect(got.Reason).To(Equal("test intent"))
		})
	})
})
