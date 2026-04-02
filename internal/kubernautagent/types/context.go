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

package types

import "context"

type signalContextKey struct{}

// WithSignalContext returns a new context carrying the given SignalContext.
func WithSignalContext(ctx context.Context, signal SignalContext) context.Context {
	return context.WithValue(ctx, signalContextKey{}, signal)
}

// SignalContextFromContext extracts the SignalContext from the context.
// Returns the signal and true if found, zero value and false otherwise.
func SignalContextFromContext(ctx context.Context) (SignalContext, bool) {
	signal, ok := ctx.Value(signalContextKey{}).(SignalContext)
	return signal, ok
}
