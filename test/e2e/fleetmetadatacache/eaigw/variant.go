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

package eaigw

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/jordigilh/kubernaut/test/e2e/fleetmetadatacache/shared"
)

// eaigwVariant is this package's shared.Variant implementation: the only
// file in this package that encodes anything Envoy-AI-Gateway-specific.
// Every scenario body lives in test/e2e/fleetmetadatacache/shared/ and is
// shared verbatim with the Kuadrant lane
// (test/e2e/fleetmetadatacache/variant.go).
type eaigwVariant struct{}

func (eaigwVariant) ResourcePrefix() string      { return "fmc-eaigw-e2e" }
func (eaigwVariant) ScenarioPrefix() string      { return "E2E-FMC-EAIGW-054" }
func (eaigwVariant) DiscoveryLabel() string      { return "EnvoyAIGateway" }
func (eaigwVariant) DynamicResourceKind() string { return "Backend" }

// backendGVK matches pkg/fleet/registry.BackendGVR (gateway.envoyproxy.io/
// v1alpha1, Kind Backend) -- EAIGWRegistry watches this CRD directly (no
// separate broker component, unlike Kuadrant's MCPServerRegistration).
// Declared locally (not imported from pkg/fleet/registry) since this test
// only needs it to build an unstructured.Unstructured for k8sClient CRUD.
func backendGVK() schema.GroupVersionKind {
	return schema.GroupVersionKind{Group: "gateway.envoyproxy.io", Version: "v1alpha1", Kind: "Backend"}
}

// NewDynamicClusterResource builds an unstructured Backend pointing at the
// same kube-mcp-server Service the fixed 3 clusters use
// (test/infrastructure/fleet_e2e.go deployEnvoyAIGatewayRegistrations). Note
// this Backend is standalone -- it is NOT wired into the shared MCPRoute's
// backendRefs, so FMC's registry (which watches Backend directly, not
// MCPRoute) still picks it up as a candidate cluster the moment it's
// created, exactly mirroring how the Kuadrant lane's dynamic
// MCPServerRegistration creates a registration without needing broker
// reconfiguration.
func (eaigwVariant) NewDynamicClusterResource(namespace, clusterID string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(backendGVK())
	obj.SetName(clusterID)
	obj.SetNamespace(namespace)
	obj.SetLabels(map[string]string{"kubernaut.ai/managed": "true"})
	_ = unstructured.SetNestedSlice(obj.Object, []interface{}{
		map[string]interface{}{
			"fqdn": map[string]interface{}{
				"hostname": fmt.Sprintf("kube-mcp-server.%s.svc.cluster.local", namespace),
				"port":     int64(8080),
			},
		},
	}, "spec", "endpoints")
	return obj
}

// RBACChecks asserts FMC's ClusterRole grants only read access to the
// EAIGW discovery CRD (Backend) it needs, and explicitly nothing from the
// Kuadrant lane's CRD group (the two ClusterRoles are gateway-specific, not
// supersets of each other).
func (eaigwVariant) RBACChecks() []shared.RBACCheck {
	return []shared.RBACCheck{
		{Verb: "get", Resource: "backends.gateway.envoyproxy.io", Allowed: true,
			Reason: "FMC must be able to get Backends to discover fleet clusters"},
		{Verb: "list", Resource: "backends.gateway.envoyproxy.io", Allowed: true},
		{Verb: "watch", Resource: "backends.gateway.envoyproxy.io", Allowed: true},
		{Verb: "create", Resource: "backends.gateway.envoyproxy.io", Allowed: false,
			Reason: "FMC has no business creating Backends"},
		{Verb: "delete", Resource: "backends.gateway.envoyproxy.io", Allowed: false},
		{Verb: "list", Resource: "mcpserverregistrations.mcp.kuadrant.io", Allowed: false,
			Reason: "FMC's EAIGW-mode ClusterRole must not also grant Kuadrant CRD access"},
	}
}
