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

package mcp

import (
	"context"
	"sync"
)

// EventStore defines the MCP SDK event store interface that tracks session lifecycle.
// This matches the go-sdk/mcp.EventStore contract.
type EventStore interface {
	Open(ctx context.Context, sessionID string) error
	SessionClosed(ctx context.Context, sessionID string) error
}

// InMemoryEventStore is a minimal event store that tracks open sessions.
type InMemoryEventStore struct {
	mu       sync.Mutex
	sessions map[string]bool
}

// NewInMemoryEventStore creates an in-memory event store.
func NewInMemoryEventStore() *InMemoryEventStore {
	return &InMemoryEventStore{
		sessions: make(map[string]bool),
	}
}

func (s *InMemoryEventStore) Open(_ context.Context, sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[sessionID] = true
	return nil
}

func (s *InMemoryEventStore) SessionClosed(_ context.Context, sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, sessionID)
	return nil
}

// DelegatingEventStore wraps an EventStore and publishes SessionClosed events
// to a channel for the SessionClosedHandler to process. Keeps business logic
// out of the EventStore (DES-01 separation of concerns).
type DelegatingEventStore struct {
	inner        EventStore
	closedChan   chan string
	mcpToSession sync.Map // mcpSessionID -> interactiveSessionID
}

// NewDelegatingEventStore wraps the given EventStore with disconnect notification.
func NewDelegatingEventStore(inner EventStore) *DelegatingEventStore {
	return &DelegatingEventStore{
		inner:      inner,
		closedChan: make(chan string, 64),
	}
}

func (s *DelegatingEventStore) Open(ctx context.Context, sessionID string) error {
	return s.inner.Open(ctx, sessionID)
}

func (s *DelegatingEventStore) SessionClosed(ctx context.Context, sessionID string) error {
	select {
	case s.closedChan <- sessionID:
	default:
	}
	return s.inner.SessionClosed(ctx, sessionID)
}

// RegisterMCPSession maps an MCP session ID to an interactive session ID.
func (s *DelegatingEventStore) RegisterMCPSession(mcpSessionID, interactiveSessionID string) {
	s.mcpToSession.Store(mcpSessionID, interactiveSessionID)
}

// LookupInteractiveSession returns the interactive session ID for a given MCP session.
func (s *DelegatingEventStore) LookupInteractiveSession(mcpSessionID string) (string, bool) {
	val, ok := s.mcpToSession.Load(mcpSessionID)
	if !ok {
		return "", false
	}
	return val.(string), true
}

// ClosedSessions returns the channel that receives closed MCP session IDs.
func (s *DelegatingEventStore) ClosedSessions() <-chan string {
	return s.closedChan
}
