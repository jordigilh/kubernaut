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
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Status represents the lifecycle state of an investigation session.
type Status string

const (
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
)

// Session holds the state of a single investigation session.
type Session struct {
	ID        string
	Status    Status
	Result    interface{}
	Error     error
	CreatedAt time.Time
	Metadata  map[string]string
}

// ErrSessionNotFound is returned when a session ID does not exist in the store.
var ErrSessionNotFound = errors.New("session not found")

// Store provides thread-safe session storage with TTL-based cleanup.
type Store struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	ttl      time.Duration
}

// NewStore creates a new session store with the given TTL for cleanup.
func NewStore(ttl time.Duration) *Store {
	return &Store{
		sessions: make(map[string]*Session),
		ttl:      ttl,
	}
}

// Create stores a new session and returns its ID.
func (s *Store) Create() (string, error) {
	id := uuid.New().String()
	sess := &Session{
		ID:        id,
		Status:    StatusPending,
		CreatedAt: time.Now(),
	}
	s.mu.Lock()
	s.sessions[id] = sess
	s.mu.Unlock()
	return id, nil
}

// Get retrieves a snapshot of a session by ID.
// Returns a copy to avoid data races between the caller and background goroutines.
func (s *Store) Get(id string) (*Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[id]
	if !ok {
		return nil, ErrSessionNotFound
	}
	return sess.clone(), nil
}

func (s *Session) clone() *Session {
	cp := *s
	if s.Metadata != nil {
		cp.Metadata = make(map[string]string, len(s.Metadata))
		for k, v := range s.Metadata {
			cp.Metadata[k] = v
		}
	}
	return &cp
}

// SetMetadata stores request-level metadata on an existing session.
func (s *Store) SetMetadata(id string, metadata map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess, ok := s.sessions[id]; ok {
		sess.Metadata = metadata
	}
}

// Update modifies an existing session.
func (s *Store) Update(id string, status Status, result interface{}, err error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok {
		return ErrSessionNotFound
	}
	sess.Status = status
	sess.Result = result
	sess.Error = err
	return nil
}

// StartCleanupLoop runs Cleanup periodically until the context is cancelled.
func (s *Store) StartCleanupLoop(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.Cleanup()
			}
		}
	}()
}

// Cleanup removes sessions older than the configured TTL.
// Returns the number of sessions removed.
func (s *Store) Cleanup() int {
	cutoff := time.Now().Add(-s.ttl)
	removed := 0
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, sess := range s.sessions {
		if sess.CreatedAt.Before(cutoff) {
			delete(s.sessions, id)
			removed++
		}
	}
	return removed
}
