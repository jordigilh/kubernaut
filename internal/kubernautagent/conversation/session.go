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

package conversation

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/investigation"
)

// SessionManager manages in-memory conversation sessions with TTL-based expiry.
type SessionManager struct {
	mu            sync.RWMutex
	sessions      map[string]*Session
	ttl           time.Duration
	promptBuilder *prompt.Builder
}

// NewSessionManager creates a manager with the given session TTL and prompt builder.
// builder may be nil for tests that don't exercise prompt rendering.
func NewSessionManager(ttl time.Duration, builder *prompt.Builder) *SessionManager {
	return &SessionManager{
		sessions:      make(map[string]*Session),
		ttl:           ttl,
		promptBuilder: builder,
	}
}

// Create initialises a new conversation session for the given RAR.
// correlationID is propagated from the create request (DD-F4). Falls back to session ID if empty.
// The error return is reserved for future validation (e.g., max-sessions cap); currently always nil.
func (m *SessionManager) Create(rarName, rarNamespace, userID, correlationID string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	s := &Session{
		ID:            uuid.New().String(),
		RARName:       rarName,
		RARNamespace:  rarNamespace,
		State:         SessionInteractive,
		Participants:  []string{userID},
		TTL:           m.ttl,
		CreatedAt:     now,
		LastActivity:  now,
		Guardrails:    NewGuardrails(rarNamespace, rarName),
		promptBuilder: m.promptBuilder,
		todoWrite:     investigation.NewTodoWriteTool(),
		CorrelationID: correlationID,
	}
	if s.CorrelationID == "" {
		s.CorrelationID = s.ID
	}
	m.sessions[s.ID] = s
	return s, nil
}

// Get retrieves an active session by ID. Returns error if expired or not found.
func (m *SessionManager) Get(id string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.sessions[id]
	if !ok {
		return nil, fmt.Errorf("session %s not found", id)
	}
	if time.Since(s.LastActivity) > s.TTL {
		delete(m.sessions, id)
		return nil, fmt.Errorf("session %s expired", id)
	}
	return s, nil
}

// Delete removes a session.
func (m *SessionManager) Delete(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.sessions[id]; !ok {
		return fmt.Errorf("session %s not found", id)
	}
	delete(m.sessions, id)
	return nil
}

// IncrementTurnAndTouch atomically increments TurnCount and updates LastActivity.
// Returns the new turn count. Must be called AFTER the rate limiter check passes (DD-F5).
func (m *SessionManager) IncrementTurnAndTouch(sessionID string, now time.Time) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[sessionID]
	if !ok {
		return 0, fmt.Errorf("session %s not found", sessionID)
	}
	s.TurnCount++
	s.LastActivity = now
	return s.TurnCount, nil
}

// AddParticipant records an additional user in the session.
func (m *SessionManager) AddParticipant(sessionID, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.sessions[sessionID]
	if !ok {
		return fmt.Errorf("session %s not found", sessionID)
	}
	for _, p := range s.Participants {
		if p == userID {
			return nil
		}
	}
	s.Participants = append(s.Participants, userID)
	return nil
}
