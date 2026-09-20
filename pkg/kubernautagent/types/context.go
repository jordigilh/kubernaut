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

package types

import (
	"context"
	"sync"
)

type signalContextKey struct{}
type discoveredWorkflowStateKey struct{}

// WithSignalContext returns a new context carrying the given SignalContext.
func WithSignalContext(ctx context.Context, signal SignalContext) context.Context {
	return context.WithValue(ctx, signalContextKey{}, signal)
}

// SignalContextFromContext extracts the SignalContext from the context.
// Returns the signal and true if found, zero value and false otherwise.
func SignalContextFromContext(ctx context.Context) (SignalContext, bool) {
	signal, ok := ctx.Value(signalContextKey{}).(SignalContext)
	return signal, ok
}

// DiscoveredWorkflowState tracks workflow IDs returned by list_workflows for
// one workflow-selection context. The state is shared by concurrent tool calls
// and by self-correction retries that may rediscover additional pages.
type DiscoveredWorkflowState struct {
	mu          sync.RWMutex
	workflowIDs map[string]struct{}
}

// NewDiscoveredWorkflowState creates empty workflow-discovery state.
func NewDiscoveredWorkflowState() *DiscoveredWorkflowState {
	return &DiscoveredWorkflowState{workflowIDs: make(map[string]struct{})}
}

// Add records workflow IDs returned by list_workflows. Empty IDs are ignored.
func (s *DiscoveredWorkflowState) Add(workflowIDs ...string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.workflowIDs == nil {
		s.workflowIDs = make(map[string]struct{})
	}
	for _, workflowID := range workflowIDs {
		if workflowID != "" {
			s.workflowIDs[workflowID] = struct{}{}
		}
	}
}

// Contains reports whether workflowID was returned by list_workflows.
func (s *DiscoveredWorkflowState) Contains(workflowID string) bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.workflowIDs[workflowID]
	return ok
}

// WithDiscoveredWorkflowState returns a context carrying workflow-discovery state.
func WithDiscoveredWorkflowState(ctx context.Context, state *DiscoveredWorkflowState) context.Context {
	return context.WithValue(ctx, discoveredWorkflowStateKey{}, state)
}

// DiscoveredWorkflowStateFromContext extracts workflow-discovery state.
func DiscoveredWorkflowStateFromContext(ctx context.Context) (*DiscoveredWorkflowState, bool) {
	state, ok := ctx.Value(discoveredWorkflowStateKey{}).(*DiscoveredWorkflowState)
	return state, ok && state != nil
}
