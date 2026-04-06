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

package session

import (
	"context"
	"log/slog"
)

// InvestigateFunc is the function signature for running an investigation.
type InvestigateFunc func(ctx context.Context) (interface{}, error)

// Manager orchestrates investigation sessions, running each in a
// background goroutine and tracking progress via the Store.
type Manager struct {
	store  *Store
	logger *slog.Logger
}

// NewManager creates a session manager backed by the given store.
func NewManager(store *Store, logger *slog.Logger) *Manager {
	return &Manager{store: store, logger: logger}
}

// StartInvestigation creates a new session and launches the investigation
// function in a background goroutine. Returns the session ID immediately.
// metadata is stored on the session for later retrieval (e.g., incident_id).
//
// The goroutine uses context.Background() to ensure the investigation outlives
// the originating HTTP request. The incoming ctx is NOT propagated because the
// request context is cancelled as soon as the 202 response is sent.
func (m *Manager) StartInvestigation(_ context.Context, fn InvestigateFunc, metadata map[string]string) (string, error) {
	id, err := m.store.Create()
	if err != nil {
		return "", err
	}
	if metadata != nil {
		m.store.SetMetadata(id, metadata)
	}
	_ = m.store.Update(id, StatusRunning, nil, nil)

	go func() {
		bgCtx := context.Background()
		result, fnErr := fn(bgCtx)
		if fnErr != nil {
			m.logger.Error("investigation failed", slog.String("session_id", id), slog.String("error", fnErr.Error()))
			_ = m.store.Update(id, StatusFailed, nil, fnErr)
			return
		}
		_ = m.store.Update(id, StatusCompleted, result, nil)
	}()

	return id, nil
}

// GetSession retrieves the current state of an investigation session.
func (m *Manager) GetSession(id string) (*Session, error) {
	return m.store.Get(id)
}
