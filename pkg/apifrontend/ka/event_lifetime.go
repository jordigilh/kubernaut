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

import "context"

type eventLifetimeKey struct{}

// WithEventLifetime associates the context that owns a live A2A event stream
// with a shorter-lived pooled MCP call. Pooled session notifications can arrive
// after the MCP result has been returned, so the subscription must live until
// the enclosing stream ends (#1637, DD-AF-015).
func WithEventLifetime(callCtx, lifetimeCtx context.Context) context.Context {
	return context.WithValue(callCtx, eventLifetimeKey{}, lifetimeCtx)
}

// EventLifetimeFromContext returns the enclosing event-stream context, if one
// was attached by the caller.
func EventLifetimeFromContext(ctx context.Context) context.Context {
	lifetimeCtx, _ := ctx.Value(eventLifetimeKey{}).(context.Context)
	return lifetimeCtx
}
