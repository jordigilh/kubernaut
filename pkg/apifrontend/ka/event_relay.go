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

package ka

import (
	"context"
	"sync"
)

// EventRelay lets a pooled session's background event-watcher goroutine
// (see WatchTerminalEvents in pkg/apifrontend/tools) discover, for the
// duration of a specific pooled MCP call, which A2A call's context (and
// therefore EventBridge, via launcher.EventBridgeFromContext) should
// receive KA events arriving mid-call — e.g. reasoning_content_delta
// emitted while KA processes a synchronous kubernaut_message turn.
//
// Idle (no in-flight call): Current() returns nil, and the watcher falls
// back to its original terminal-event-only behavior, unchanged from #1438.
//
// See DD-AF-009 for the design rationale and rejected alternatives.
//
//nolint:containedctx // intentional: this ctx is a cross-goroutine handoff
// pointer (the watcher goroutine reads what the pooled-call goroutine
// attaches), not a ctx threaded through a call chain — the anti-pattern this
// lint guards against does not apply. See DD-AF-009 Alternative D.
type EventRelay struct {
	mu  sync.Mutex
	ctx context.Context
}

// Attach records ctx as the context of the pooled call currently in
// flight. Returns a detach func that must be deferred by the caller; it
// only clears the field if it still points at this exact ctx, so a stale
// detach from an outer call can never clobber a more recent (inner) one.
func (r *EventRelay) Attach(ctx context.Context) (detach func()) {
	r.mu.Lock()
	r.ctx = ctx
	r.mu.Unlock()
	return func() {
		r.mu.Lock()
		if r.ctx == ctx {
			r.ctx = nil
		}
		r.mu.Unlock()
	}
}

// Current returns the ctx of the pooled call currently in flight, or nil
// when idle.
func (r *EventRelay) Current() context.Context {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ctx
}
