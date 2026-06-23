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

package mcpclient

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ResourceWriter provides K8s-compatible write access to resources on a remote
// cluster via the MCP Gateway. It mirrors ResourceClient (which embeds
// client.Reader) but for write operations (Create, Delete, Update).
//
// This is a narrower interface than client.Writer because Patch, DeleteAllOf,
// and Apply are not supported via MCP. Executors only need Create/Delete/Update.
//
// Write operations are isolated in a separate type so that read-only consumers
// (SP ReaderFactory, FMC) never gain accidental write access.
//
// Authority: Issue #54, Fleet Federation Roadmap Phase 3 (BR-FLEET-054)
type ResourceWriter interface {
	Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error
	Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error
	Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error
	Close() error
}
