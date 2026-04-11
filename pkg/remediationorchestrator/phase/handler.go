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

package phase

import (
	"context"
	"fmt"
	"sort"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
)

// PhaseHandler encapsulates the reconcile logic for a single RO phase.
// Implementations perform side effects (e.g., creating child CRDs) and
// return a TransitionIntent expressing the desired next state.
//
// The error return signals an infrastructure failure that prevented the
// handler from completing. Business-level failures (e.g., approval rejected)
// are expressed via TransitionIntent with TransitionFailed, not via error.
//
// Reference: Issue #666 (RO Phase Handler Registry refactoring)
type PhaseHandler interface {
	// Phase returns which phase this handler manages.
	Phase() Phase

	// Handle processes a RemediationRequest in this handler's phase.
	Handle(ctx context.Context, rr *remediationv1.RemediationRequest) (TransitionIntent, error)
}

// Registry maps phases to their PhaseHandler implementations.
// It enforces single-handler-per-phase and provides O(1) lookup.
//
// Reference: Issue #666 (RO Phase Handler Registry refactoring)
type Registry struct {
	handlers map[Phase]PhaseHandler
}

// NewRegistry creates an empty handler registry.
func NewRegistry() *Registry {
	return &Registry{
		handlers: make(map[Phase]PhaseHandler),
	}
}

// Register adds a handler to the registry. Returns an error if:
//   - h is nil
//   - a handler is already registered for the same phase
func (r *Registry) Register(h PhaseHandler) error {
	if h == nil {
		return fmt.Errorf("cannot register nil handler")
	}
	p := h.Phase()
	if _, exists := r.handlers[p]; exists {
		return fmt.Errorf("handler already registered for phase %s", p)
	}
	r.handlers[p] = h
	return nil
}

// MustRegister adds a handler to the registry and panics on duplicate.
func (r *Registry) MustRegister(h PhaseHandler) {
	if err := r.Register(h); err != nil {
		panic(err)
	}
}

// Lookup returns the handler for the given phase, or false if none registered.
func (r *Registry) Lookup(p Phase) (PhaseHandler, bool) {
	h, ok := r.handlers[p]
	return h, ok
}

// Phases returns all phases that have registered handlers, sorted for determinism.
func (r *Registry) Phases() []Phase {
	phases := make([]Phase, 0, len(r.handlers))
	for p := range r.handlers {
		phases = append(phases, p)
	}
	sort.Slice(phases, func(i, j int) bool {
		return string(phases[i]) < string(phases[j])
	})
	return phases
}
