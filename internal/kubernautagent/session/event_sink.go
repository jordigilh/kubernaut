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
	"sync"
	"sync/atomic"
)

type eventSinkKey struct{}
type sessionIDKey struct{}
type actingUserKey struct{}

// lazySinkBufferCap bounds the number of events LazySink buffers before a
// channel is attached. Matches eventChannelBuffer (manager.go) so a full
// replay never overflows the newly-activated channel (#1811).
const lazySinkBufferCap = 64

// LazySink holds a channel reference that can be set after the context is
// created. This allows Subscribe to attach the event sink lazily while the
// investigation goroutine's context is already in flight.
//
// #1811: before a channel is attached, Emit buffers events (bounded,
// oldest-evicted) instead of silently dropping them. Set replays the
// buffer to the newly-activated channel so a late Subscribe (e.g. AF's
// kubernaut_investigate racing an autonomous investigation's fast
// InteractiveHold completion) still delivers everything emitted so far.
type LazySink struct {
	mu     sync.RWMutex
	ch     chan<- InvestigationEvent
	buffer []InvestigationEvent
}

// Set assigns the channel and replays any buffered events (oldest-first,
// non-blocking) accumulated while no channel was attached. Safe for
// concurrent use.
func (ls *LazySink) Set(ch chan<- InvestigationEvent) {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	ls.ch = ch
	if ch == nil || len(ls.buffer) == 0 {
		return
	}
	for _, evt := range ls.buffer {
		select {
		case ch <- evt:
		default:
			// Newly-created channel is already full (unexpected given it's
			// sized to eventChannelBuffer >= lazySinkBufferCap) — stop
			// replay rather than block session activation.
		}
	}
	ls.buffer = nil
}

// Get returns the channel, or nil if not yet set.
func (ls *LazySink) Get() chan<- InvestigationEvent {
	ls.mu.RLock()
	defer ls.mu.RUnlock()
	return ls.ch
}

// Emit sends event to the active channel (non-blocking), or buffers it
// (bounded, oldest-evicted) if no channel is attached yet. Returns true if
// the event was sent to an active channel, false if buffered or dropped
// (channel full). Callers that need to distinguish "buffered for later
// replay" from "dropped, channel full" should check Get() != nil first.
func (ls *LazySink) Emit(event InvestigationEvent) bool {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	if ls.ch == nil {
		ls.buffer = append(ls.buffer, event)
		if len(ls.buffer) > lazySinkBufferCap {
			ls.buffer = ls.buffer[len(ls.buffer)-lazySinkBufferCap:]
		}
		return false
	}
	select {
	case ls.ch <- event:
		return true
	default:
		return false
	}
}

// WithLazySink returns a derived context carrying a LazySink.
// The investigator retrieves the current channel via EventSinkFromContext.
func WithLazySink(ctx context.Context, ls *LazySink) context.Context {
	return context.WithValue(ctx, eventSinkKey{}, ls)
}

// WithEventSink returns a derived context carrying a pre-set event channel.
// Retained for backward compatibility with tests that set the sink eagerly.
func WithEventSink(ctx context.Context, ch chan<- InvestigationEvent) context.Context {
	ls := &LazySink{}
	ls.Set(ch)
	return context.WithValue(ctx, eventSinkKey{}, ls)
}

// EventSinkFromContext retrieves the event sink channel from ctx, or nil if
// none was attached (or the lazy sink has not been activated yet).
// Callers must nil-check before sending.
func EventSinkFromContext(ctx context.Context) chan<- InvestigationEvent {
	ls, ok := ctx.Value(eventSinkKey{}).(*LazySink)
	if !ok || ls == nil {
		return nil
	}
	return ls.Get()
}

// EmitEvent sends event through the LazySink attached to ctx (if any),
// buffering it for later replay if no channel is attached yet (#1811).
// Returns false if there is no LazySink on ctx, if the event was buffered,
// or if the active channel's buffer is full.
func EmitEvent(ctx context.Context, event InvestigationEvent) bool {
	ls, ok := ctx.Value(eventSinkKey{}).(*LazySink)
	if !ok || ls == nil {
		return false
	}
	return ls.Emit(event)
}

// WithSessionID returns a derived context carrying the session ID so that
// lower-level code (e.g. the investigator) can cross-reference audit events.
func WithSessionID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, sessionIDKey{}, id)
}

// SessionIDFromContext retrieves the session ID from ctx, or "" if absent.
func SessionIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(sessionIDKey{}).(string); ok {
		return v
	}
	return ""
}

// WithActingUser returns a derived context carrying the acting user identity
// for interactive sessions. Used by the MCP tool handler to propagate user
// attribution to audit events emitted by the investigator (BR-INTERACTIVE-005).
func WithActingUser(ctx context.Context, user string) context.Context {
	return context.WithValue(ctx, actingUserKey{}, user)
}

// ActingUserFromContext retrieves the acting user identity from ctx, or "" if
// absent (autonomous mode).
func ActingUserFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(actingUserKey{}).(string); ok {
		return v
	}
	return ""
}

type interactiveUpgradeKey struct{}

// WithInteractiveUpgrade returns a derived context carrying the interactive
// upgrade atomic flag. The investigator goroutine checks this flag to decide
// whether to set InteractiveHold=true on the result (BR-INTERACTIVE-004, #1390).
func WithInteractiveUpgrade(ctx context.Context, flag *atomic.Bool) context.Context {
	return context.WithValue(ctx, interactiveUpgradeKey{}, flag)
}

// InteractiveUpgradeFromContext reads the interactive upgrade flag from ctx.
// Returns false if no flag was attached or the flag is false.
func InteractiveUpgradeFromContext(ctx context.Context) bool {
	flag, ok := ctx.Value(interactiveUpgradeKey{}).(*atomic.Bool)
	if !ok || flag == nil {
		return false
	}
	return flag.Load()
}
