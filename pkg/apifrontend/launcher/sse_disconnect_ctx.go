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
package launcher

import "context"

type sseDisconnectCtxKey struct{}

// WithSSEDisconnectCtx stores the HTTP request context as a value so that
// downstream components running in a detached context (a2a-go uses
// context.WithoutCancel for execution goroutines) can still detect SSE
// client disconnects. Context values survive WithoutCancel, while
// cancellation signals do not.
//
// Go's net/http cancels r.Context() when the client's TCP connection closes
// (before ServeHTTP returns), making it a reliable disconnect indicator even
// from within a detached execution goroutine — as long as the reference is
// preserved as a value.
func WithSSEDisconnectCtx(parent context.Context) context.Context {
	return context.WithValue(parent, sseDisconnectCtxKey{}, parent)
}

// SSEDisconnectCtxFromContext retrieves the stored HTTP request context.
// Returns nil if not set.
func SSEDisconnectCtxFromContext(ctx context.Context) context.Context {
	v, _ := ctx.Value(sseDisconnectCtxKey{}).(context.Context)
	return v
}
