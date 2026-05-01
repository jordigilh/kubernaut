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

import "sync"

// SessionNotifier is a thread-safe registry that maps interactive session IDs
// to notification callbacks. Used to bridge TimeoutManager warnings to MCP
// client sessions via ServerSession.Log. BR-INTERACTIVE-005, UX-01/02.
type SessionNotifier struct {
	notifiers sync.Map // sessionID -> func(msg string)
}

// NewSessionNotifier creates an empty notifier registry.
func NewSessionNotifier() *SessionNotifier {
	return &SessionNotifier{}
}

// Register associates a notification callback with a session. The callback
// is typically a closure that calls ServerSession.Log to deliver the message.
func (n *SessionNotifier) Register(sessionID string, fn func(msg string)) {
	if fn != nil {
		n.notifiers.Store(sessionID, fn)
	}
}

// Notify delivers a message to the registered callback for the session.
// No-op if the session has no registered notifier (already deregistered).
func (n *SessionNotifier) Notify(sessionID, msg string) {
	if raw, ok := n.notifiers.Load(sessionID); ok {
		fn := raw.(func(msg string))
		fn(msg)
	}
}

// Deregister removes the notification callback for a session.
func (n *SessionNotifier) Deregister(sessionID string) {
	n.notifiers.Delete(sessionID)
}
