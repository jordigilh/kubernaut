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

package investigator

import (
	"sync"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// PhaseClientResolver resolves the appropriate LLM client for a given
// investigation phase. When per-phase overrides are configured, each
// phase may use a different model (e.g. fast model for workflow discovery,
// reasoning model for RCA).
type PhaseClientResolver interface {
	ResolvePhase(phase katypes.Phase) (client llm.Client, modelName string, runtimeParams llm.RuntimeParams)
}

// DefaultPhaseResolver looks up a phase-specific SwappableClient when one
// has been registered; otherwise it falls back to the default SwappableClient.
// In both cases, it takes a Snapshot() of the resolved SwappableClient
// (pinning the client for the duration of the phase) and applies the
// PinDecorator if one is set.
type DefaultPhaseResolver struct {
	mu               sync.RWMutex
	defaultSwappable *llm.SwappableClient
	phaseSwappables  map[katypes.Phase]*llm.SwappableClient
	pinDecorator     func(llm.Client) llm.Client
}

// NewDefaultPhaseResolver creates a DefaultPhaseResolver with the given
// default SwappableClient and optional PinDecorator.
func NewDefaultPhaseResolver(
	defaultSwappable *llm.SwappableClient,
	pinDecorator func(llm.Client) llm.Client,
) *DefaultPhaseResolver {
	return &DefaultPhaseResolver{
		defaultSwappable: defaultSwappable,
		phaseSwappables:  make(map[katypes.Phase]*llm.SwappableClient),
		pinDecorator:     pinDecorator,
	}
}

// ResolvePhase returns a pinned, decorated LLM client for the given phase.
// If a phase-specific SwappableClient exists, it is used; otherwise the
// default is used. The returned client is a snapshot that is unaffected by
// subsequent hot-reload swaps.
func (r *DefaultPhaseResolver) ResolvePhase(phase katypes.Phase) (llm.Client, string, llm.RuntimeParams) {
	r.mu.RLock()
	sw, ok := r.phaseSwappables[phase]
	r.mu.RUnlock()

	if !ok || sw == nil {
		sw = r.defaultSwappable
	}

	pinned := sw.Snapshot()
	modelName := sw.ModelName()
	params := sw.RuntimeParameters()

	var client llm.Client
	if r.pinDecorator != nil {
		client = r.pinDecorator(pinned)
		if client == nil {
			client = llm.NewInstrumentedClient(pinned)
		}
	} else {
		client = llm.NewInstrumentedClient(pinned)
	}
	return client, modelName, params
}

// SetPhaseSwappable adds or replaces a phase-specific SwappableClient.
func (r *DefaultPhaseResolver) SetPhaseSwappable(phase katypes.Phase, sw *llm.SwappableClient) {
	r.mu.Lock()
	r.phaseSwappables[phase] = sw
	r.mu.Unlock()
}

// RemovePhaseSwappable removes a phase-specific SwappableClient, causing
// the phase to fall back to the default client.
func (r *DefaultPhaseResolver) RemovePhaseSwappable(phase katypes.Phase) {
	r.mu.Lock()
	delete(r.phaseSwappables, phase)
	r.mu.Unlock()
}

// PhaseSwappable returns the SwappableClient for a phase, or nil if the
// phase uses the default client.
func (r *DefaultPhaseResolver) PhaseSwappable(phase katypes.Phase) *llm.SwappableClient {
	r.mu.RLock()
	sw := r.phaseSwappables[phase]
	r.mu.RUnlock()
	return sw
}

// Phases returns the list of phases that have specific overrides.
func (r *DefaultPhaseResolver) Phases() []katypes.Phase {
	r.mu.RLock()
	defer r.mu.RUnlock()
	phases := make([]katypes.Phase, 0, len(r.phaseSwappables))
	for p := range r.phaseSwappables {
		phases = append(phases, p)
	}
	return phases
}

var _ PhaseClientResolver = (*DefaultPhaseResolver)(nil)
