/*
Copyright 2025 Jordi Gil.

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

package scope

import "k8s.io/apimachinery/pkg/runtime/schema"

// ScopeGVKs returns the GroupVersionKinds that the scope manager needs
// cached for resource-level label lookups. Both Gateway and RO should
// include these GVKs in their RBAC ClusterRole (list+watch permissions)
// to enable metadata-only informers.
//
// This function exports the static mapping from kindToGroup so that
// RBAC configuration, cache setup, and documentation remain consistent
// across services.
//
// Reference: ADR-053 (Resource Scope Management Architecture)
func ScopeGVKs() []schema.GroupVersionKind {
	gvks := make([]schema.GroupVersionKind, 0, len(kindToGroup))
	for kind, group := range kindToGroup {
		gvks = append(gvks, schema.GroupVersionKind{
			Group:   group,
			Version: "v1",
			Kind:    kind,
		})
	}
	// Add Namespace GVK (used for namespace-level label checks)
	gvks = append(gvks, namespaceGVK)
	return gvks
}
