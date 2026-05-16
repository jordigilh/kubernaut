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

	"github.com/go-logr/logr"
)

// SessionDrainer handles graceful shutdown of active MCP sessions.
// During SIGTERM, it notifies all connected clients, waits for in-flight
// tool executions to complete, then releases all Leases. BR-OPS-013.
type SessionDrainer struct {
	sessions *LeaseSessionManager
	notifier *SessionNotifier
	logger   logr.Logger
}

// NewSessionDrainer creates a SessionDrainer backed by the given session
// manager and notifier. Either may be nil: without a session manager no
// sessions are drained, without a notifier no shutdown notifications are sent.
func NewSessionDrainer(sessions *LeaseSessionManager, notifier *SessionNotifier, logger logr.Logger) *SessionDrainer {
	var zero logr.Logger
	if logger == zero {
		logger = logr.Discard()
	}
	return &SessionDrainer{
		sessions: sessions,
		notifier: notifier,
		logger:   logger,
	}
}

// DrainSessions notifies all connected MCP clients that the server is shutting
// down, then releases all active session Leases. Blocks until all sessions are
// released or the context is cancelled (whichever comes first).
//
// With zero active sessions, this returns immediately.
func (d *SessionDrainer) DrainSessions(ctx context.Context) {
	if d.sessions == nil {
		return
	}

	ids := d.sessions.ActiveSessionIDs()
	if len(ids) == 0 {
		return
	}

	d.logger.Info("draining interactive sessions", "count", len(ids))

	if d.notifier != nil {
		for _, id := range ids {
			d.notifier.Notify(id, "Server is shutting down. Your session will be released.")
		}
	}

	for _, id := range ids {
		select {
		case <-ctx.Done():
			d.logger.Info("drain context expired, force-releasing remaining sessions")
			d.forceReleaseAll(ids)
			return
		default:
		}
		if err := d.sessions.Release(id, "shutdown"); err != nil {
			d.logger.Error(err, "failed to release session during drain", "session_id", id)
		} else {
			d.logger.Info("session released during drain", "session_id", id)
		}
	}
}

func (d *SessionDrainer) forceReleaseAll(ids []string) {
	for _, id := range ids {
		if err := d.sessions.Release(id, "shutdown_forced"); err != nil {
			d.logger.V(1).Info("force-release skipped (already released or error)",
				"session_id", id, "error", err)
		}
	}
}
