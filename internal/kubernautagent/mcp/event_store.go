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
	"iter"
	"sync"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// DelegatingEventStore implements the MCP SDK's mcp.EventStore interface,
// delegating stream resumption to MemoryEventStore while intercepting
// SessionClosed to notify the SessionClosedHandler via a buffered channel.
// DD-INTERACTIVE-002: session lifecycle detection feeds disconnect handling.
type DelegatingEventStore struct {
	inner        *mcpsdk.MemoryEventStore
	closedChan   chan string
	mcpToSession sync.Map // mcpSessionID -> interactiveSessionID
}

// NewDelegatingEventStore wraps the SDK's MemoryEventStore with disconnect
// notification. The closedChan has capacity 64 to avoid blocking the SDK.
func NewDelegatingEventStore() *DelegatingEventStore {
	return &DelegatingEventStore{
		inner:      mcpsdk.NewMemoryEventStore(nil),
		closedChan: make(chan string, 64),
	}
}

func (s *DelegatingEventStore) Open(ctx context.Context, sessionID, streamID string) error {
	return s.inner.Open(ctx, sessionID, streamID)
}

func (s *DelegatingEventStore) Append(ctx context.Context, sessionID, streamID string, data []byte) error {
	return s.inner.Append(ctx, sessionID, streamID, data)
}

func (s *DelegatingEventStore) After(ctx context.Context, sessionID, streamID string, index int) iter.Seq2[[]byte, error] {
	return s.inner.After(ctx, sessionID, streamID, index)
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

// DeleteMCPSession removes the MCP-to-interactive mapping after the disconnect
// handler has finished processing. Must be called by the handler callback, not
// by SessionClosed, to avoid a race where LookupInteractiveSession returns false
// because the mapping was deleted before the handler consumed the channel event.
func (s *DelegatingEventStore) DeleteMCPSession(mcpSessionID string) {
	s.mcpToSession.Delete(mcpSessionID)
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
