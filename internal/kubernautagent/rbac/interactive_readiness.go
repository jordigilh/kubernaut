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

// Package rbac provides RBAC utilities for the Kubernaut Agent.
package rbac

import "sync"

// InteractiveReadiness tracks whether interactive mode is operational.
// Thread-safe for concurrent reads from the health endpoint.
type InteractiveReadiness struct {
	mu     sync.RWMutex
	state  readinessState
	reason string
}

type readinessState int

const (
	stateNotConfigured readinessState = iota
	stateEnabled
	stateSoftDisabled
)

// NewInteractiveReadiness returns a readiness tracker in the not_configured state.
func NewInteractiveReadiness() *InteractiveReadiness {
	return &InteractiveReadiness{state: stateNotConfigured}
}

// SetEnabled marks interactive mode as fully operational.
func (r *InteractiveReadiness) SetEnabled() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.state = stateEnabled
	r.reason = ""
}

// SetSoftDisabled marks interactive mode as soft-disabled with a reason.
func (r *InteractiveReadiness) SetSoftDisabled(reason string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.state = stateSoftDisabled
	r.reason = reason
}

// IsEnabled returns true only when interactive mode is fully operational.
func (r *InteractiveReadiness) IsEnabled() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.state == stateEnabled
}

// StatusString returns a machine-readable status: "enabled", "soft_disabled", or "not_configured".
func (r *InteractiveReadiness) StatusString() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	switch r.state {
	case stateEnabled:
		return "enabled"
	case stateSoftDisabled:
		return "soft_disabled"
	default:
		return "not_configured"
	}
}

// Reason returns the human-readable reason for soft-disablement, or empty string.
func (r *InteractiveReadiness) Reason() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.reason
}
