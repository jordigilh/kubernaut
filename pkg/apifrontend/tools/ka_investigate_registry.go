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

package tools

import "sync"

// MonitorRegistry tracks active investigation sessions and their cleanup
// functions. It provides lifecycle management for background goroutines.
type MonitorRegistry struct {
	mu       sync.Mutex
	sessions map[string]func()
}

// NewMonitorRegistry creates a new empty monitor registry.
func NewMonitorRegistry() *MonitorRegistry {
	return &MonitorRegistry{
		sessions: make(map[string]func()),
	}
}

// Register adds a session to the registry with its closer function.
func (r *MonitorRegistry) Register(sessionID string, closer func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessions[sessionID] = closer
}

// Deregister removes a session from the registry without calling its closer.
// Safe to call if the session is not registered.
func (r *MonitorRegistry) Deregister(sessionID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.sessions, sessionID)
}

// Active returns true if the session is tracked in the registry.
func (r *MonitorRegistry) Active(sessionID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.sessions[sessionID]
	return ok
}

// Stop calls the closer for the session and removes it from the registry.
func (r *MonitorRegistry) Stop(sessionID string) {
	r.mu.Lock()
	closer, ok := r.sessions[sessionID]
	if ok {
		delete(r.sessions, sessionID)
	}
	r.mu.Unlock()

	if ok && closer != nil {
		closer()
	}
}

// StopAll calls all closers and clears the registry.
func (r *MonitorRegistry) StopAll() {
	r.mu.Lock()
	sessions := r.sessions
	r.sessions = make(map[string]func())
	r.mu.Unlock()

	for _, closer := range sessions {
		if closer != nil {
			closer()
		}
	}
}
