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
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
)

// SSE proxy header constants prevent reverse proxies (nginx, envoy) from
// buffering the SSE stream. These MUST be set on every SSE response.
const (
	SSEContentType         = "text/event-stream"
	SSECacheControl        = "no-cache"
	SSEConnection          = "keep-alive"
	SSEAccelBufferingKey   = "X-Accel-Buffering"
	SSEAccelBufferingValue = "no"
)

// SSEEvent represents a Server-Sent Event.
type SSEEvent struct {
	ID    string
	Event string
	Data  string
}

type bufferedEvent struct {
	event     SSEEvent
	timestamp time.Time
}

// SSEWriter manages SSE event writing with incrementing IDs and a ring buffer
// for reconnection support.
type SSEWriter struct {
	mu        sync.Mutex
	nextID    int
	buffer    []bufferedEvent
	bufferTTL time.Duration
}

// NewSSEWriter creates an SSE writer with a ring buffer for reconnection.
func NewSSEWriter(bufferTTL time.Duration) *SSEWriter {
	return &SSEWriter{
		nextID:    1,
		bufferTTL: bufferTTL,
	}
}

// WriteEvent creates an SSE event with an auto-incrementing ID and buffers it.
func (w *SSEWriter) WriteEvent(eventType, data string) *SSEEvent {
	w.mu.Lock()
	defer w.mu.Unlock()

	evt := SSEEvent{
		ID:    strconv.Itoa(w.nextID),
		Event: eventType,
		Data:  data,
	}
	w.nextID++
	w.buffer = append(w.buffer, bufferedEvent{event: evt, timestamp: time.Now()})
	return &evt
}

// ReplayFrom replays events from the given event ID (for Last-Event-ID reconnection).
// It returns all buffered events with IDs strictly after lastEventID.
func (w *SSEWriter) ReplayFrom(lastEventID string) []SSEEvent {
	w.mu.Lock()
	defer w.mu.Unlock()

	fromID, _ := strconv.Atoi(lastEventID)
	var result []SSEEvent
	cutoff := time.Now().Add(-w.bufferTTL)
	for _, be := range w.buffer {
		evtID, _ := strconv.Atoi(be.event.ID)
		if evtID > fromID && be.timestamp.After(cutoff) {
			result = append(result, be.event)
		}
	}
	return result
}

// TurnAuditor emits audit events for conversation turns.
type TurnAuditor struct {
	store auditStore
}

type auditStore interface {
	StoreAudit(ctx context.Context, event *auditEvent) error
}

type auditEvent = audit.AuditEvent

// NewTurnAuditor creates a turn auditor backed by the given store.
func NewTurnAuditor(store auditStore) *TurnAuditor {
	return &TurnAuditor{store: store}
}

// EmitTurn records a conversation turn as an audit event.
func (t *TurnAuditor) EmitTurn(ctx context.Context, sessionID, userID, correlationID, question, answer string) {
	evt := audit.NewEvent(audit.EventTypeConversationTurn, correlationID)
	evt.EventAction = "conversation_turn"
	evt.EventOutcome = audit.OutcomeSuccess
	evt.Data["session_id"] = sessionID
	evt.Data["user_id"] = userID
	evt.Data["question"] = question
	evt.Data["answer"] = answer
	_ = t.store.StoreAudit(ctx, evt)
}
