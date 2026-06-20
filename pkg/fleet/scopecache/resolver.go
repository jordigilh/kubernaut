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

package scopecache

import "context"

// RemoteScopeResolver checks whether a resource on a remote cluster is managed
// by Kubernaut (has the kubernaut.ai/managed=true label). This is the pluggable
// interface that allows different backend implementations:
//
//   - Valkey-backed Client (FMC syncs labels to Valkey) — for environments
//     without a federated control plane (GitOps, manual cluster management)
//   - ACM Search GraphQL — for environments with ACM deployed, querying managed
//     labels directly without needing FMC or Valkey
//
// The FederatedScopeChecker accepts any RemoteScopeResolver, allowing operators
// to choose the implementation that matches their environment.
type RemoteScopeResolver interface {
	// IsManaged returns true if the specified resource on the given cluster
	// is labeled kubernaut.ai/managed=true.
	IsManaged(ctx context.Context, clusterID, group, version, kind, namespace, name string) (bool, error)
}

// Compile-time verification that Client satisfies RemoteScopeResolver.
var _ RemoteScopeResolver = (*Client)(nil)
