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

import "context"

type eventSinkKey struct{}

// WithEventSink returns a derived context carrying the given event channel.
// The investigator retrieves it via EventSinkFromContext to emit turn-level
// events without importing the session package directly.
func WithEventSink(ctx context.Context, ch chan<- InvestigationEvent) context.Context {
	return context.WithValue(ctx, eventSinkKey{}, ch)
}

// EventSinkFromContext retrieves the event sink channel from ctx, or nil if
// none was attached. Callers must nil-check before sending.
func EventSinkFromContext(ctx context.Context) chan<- InvestigationEvent {
	ch, _ := ctx.Value(eventSinkKey{}).(chan<- InvestigationEvent)
	return ch
}
