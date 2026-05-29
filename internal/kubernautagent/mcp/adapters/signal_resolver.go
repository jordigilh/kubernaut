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

package adapters

import (
	"context"
	"fmt"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// SignalProvider retrieves the SignalContext stored on the session when the
// AA payload was received. This avoids cross-CRD reads (KA → AA or KA → RR)
// and ensures both autonomous and interactive paths derive signal context
// from the same source: the original AA IncidentRequest payload.
type SignalProvider interface {
	GetSignalForRemediation(rrID string) (*katypes.SignalContext, error)
}

// SessionSignalContextResolver reads the SignalContext that was persisted on the
// investigation session at creation time (from the AA IncidentRequest payload).
// Both autonomous and interactive sessions store the full signal via
// StartInvestigationWithContext / StartInteractiveSessionWithContext, so
// discover_workflows always gets severity, environment, priority, and all
// other fields without reading any CRD.
type SessionSignalContextResolver struct {
	provider SignalProvider
}

// NewSessionSignalContextResolver creates a resolver backed by the session manager.
func NewSessionSignalContextResolver(provider SignalProvider) *SessionSignalContextResolver {
	return &SessionSignalContextResolver{provider: provider}
}

// ResolveSignalContext returns the SignalContext stored on the session for the
// given remediation ID.
func (r *SessionSignalContextResolver) ResolveSignalContext(_ context.Context, rrID string) (*katypes.SignalContext, error) {
	signal, err := r.provider.GetSignalForRemediation(rrID)
	if err != nil {
		return nil, fmt.Errorf("resolve signal context for %s: %w", rrID, err)
	}
	return signal, nil
}

// ResolveEnrichmentData returns empty enrichment data. Full enrichment is
// handled by the investigator's enrichment pipeline, not the signal resolver.
func (r *SessionSignalContextResolver) ResolveEnrichmentData(_ context.Context, _ string) (*prompt.EnrichmentData, error) {
	return &prompt.EnrichmentData{}, nil
}

// ResolvePostRCAEnrichment performs Phase 2 re-enrichment for the RCA-identified
// target. Delegates to the enricher if wired, otherwise returns empty data.
func (r *SessionSignalContextResolver) ResolvePostRCAEnrichment(_ context.Context, _, _, _, _ string) (*prompt.EnrichmentData, error) {
	return &prompt.EnrichmentData{}, nil
}
