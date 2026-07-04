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
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ensureGVK returns obj's GroupVersionKind, inferring it from scheme when the
// caller didn't set one explicitly.
//
// Unlike the cached controller-runtime client (which always has a scheme
// handy to resolve typed objects), this package talks to the K8s MCP Server
// over JSON-RPC and needs Kind/apiVersion as explicit wire values -- there is
// no informer/RESTMapper in the loop to infer them from. Historically every
// caller was required to call SetGroupVersionKind itself before Get/List/
// Create/Update/Delete; that footgun caused the same class of bug in three
// unrelated packages (WorkflowExecution's Job/Tekton executors, and the
// EffectivenessMonitor controller's cross-cluster health checks -- Issue
// #1542 follow-up). Falling back to scheme-based inference here, once, makes
// the client behave like every other client.Reader/client.Writer implementation
// callers already assume it does.
//
// An explicit GVK (Kind != "") always wins -- this only fills in what's
// missing, it never overrides a caller's choice (relevant for APIVersion
// disambiguation across API groups, e.g. Route in multiple groups).
func ensureGVK(obj runtime.Object, scheme *runtime.Scheme) (schema.GroupVersionKind, error) {
	gvk := obj.GetObjectKind().GroupVersionKind()
	if gvk.Kind != "" {
		return gvk, nil
	}

	gvks, _, err := scheme.ObjectKinds(obj)
	if err != nil || len(gvks) == 0 {
		return schema.GroupVersionKind{}, fmt.Errorf(
			"object GVK Kind must be set before calling this method (scheme could not infer it for %T: %w)", obj, err)
	}

	// Multiple registered GVKs (e.g. an internal + external version) are rare
	// for the built-in types this client exchanges; take the first and let
	// the caller override via explicit SetGroupVersionKind if ambiguous.
	resolved := gvks[0]
	obj.GetObjectKind().SetGroupVersionKind(resolved)
	return resolved, nil
}
