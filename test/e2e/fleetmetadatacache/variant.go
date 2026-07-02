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

package fleetmetadatacache

import (
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/jordigilh/kubernaut/test/e2e/fleetmetadatacache/shared"
)

// kuadrantVariant is this package's shared.Variant implementation: the only
// file in this package that encodes anything Kuadrant-specific. Every
// scenario body lives in test/e2e/fleetmetadatacache/shared/ and is shared
// verbatim with the EAIGW lane (test/e2e/fleetmetadatacache/eaigw/variant.go).
type kuadrantVariant struct{}

func (kuadrantVariant) ResourcePrefix() string      { return "fmc-e2e" }
func (kuadrantVariant) ScenarioPrefix() string      { return "E2E-FMC-054" }
func (kuadrantVariant) DiscoveryLabel() string      { return "Kuadrant" }
func (kuadrantVariant) DynamicResourceKind() string { return "MCPServerRegistration" }

// mcpServerRegistrationGVK matches pkg/fleet/registry.MCPServerRegistrationGVR
// (mcp.kuadrant.io/v1alpha1, Kind MCPServerRegistration). Declared locally
// (not imported from pkg/fleet/registry) since this test only needs it to
// build an unstructured.Unstructured for k8sClient CRUD -- pulling in the
// registry package for one GVK constant isn't warranted.
func mcpServerRegistrationGVK() schema.GroupVersionKind {
	return schema.GroupVersionKind{Group: "mcp.kuadrant.io", Version: "v1alpha1", Kind: "MCPServerRegistration"}
}

// NewDynamicClusterResource builds an unstructured MCPServerRegistration
// pointing at the same kube-mcp-server-route HTTPRoute the fixed 3 clusters
// use (test/infrastructure/fleet_e2e.go DeployFleetCoreInfra Phase 3).
func (kuadrantVariant) NewDynamicClusterResource(namespace, clusterID string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(mcpServerRegistrationGVK())
	obj.SetName(clusterID)
	obj.SetNamespace(namespace)
	obj.SetLabels(map[string]string{"kubernaut.ai/managed": "true"})
	// spec.prefix is CRD-validated against ^[a-z0-9][a-z0-9_]*$ (no hyphens),
	// unlike metadata.name which allows the RFC 1123 hyphenated form.
	prefix := strings.ReplaceAll(clusterID, "-", "_") + "_"
	_ = unstructured.SetNestedField(obj.Object, prefix, "spec", "prefix")
	_ = unstructured.SetNestedMap(obj.Object, map[string]interface{}{
		"group":     "gateway.networking.k8s.io",
		"kind":      "HTTPRoute",
		"name":      "kube-mcp-server-route",
		"namespace": namespace,
	}, "spec", "targetRef")
	return obj
}

// RBACChecks asserts FMC's ClusterRole grants only read access to the
// Kuadrant discovery CRD and Gateway API objects it needs, and nothing more.
func (kuadrantVariant) RBACChecks() []shared.RBACCheck {
	return []shared.RBACCheck{
		{Verb: "get", Resource: "mcpserverregistrations.mcp.kuadrant.io", Allowed: true,
			Reason: "FMC must be able to get MCPServerRegistrations to discover fleet clusters"},
		{Verb: "list", Resource: "mcpserverregistrations.mcp.kuadrant.io", Allowed: true},
		{Verb: "watch", Resource: "mcpserverregistrations.mcp.kuadrant.io", Allowed: true},
		{Verb: "list", Resource: "gateways.gateway.networking.k8s.io", Allowed: true},
		{Verb: "list", Resource: "httproutes.gateway.networking.k8s.io", Allowed: true},
		{Verb: "create", Resource: "mcpserverregistrations.mcp.kuadrant.io", Allowed: false,
			Reason: "FMC has no business creating MCPServerRegistrations"},
		{Verb: "delete", Resource: "mcpserverregistrations.mcp.kuadrant.io", Allowed: false},
	}
}
