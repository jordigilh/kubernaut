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

package shared

import "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

// Variant captures the only things that legitimately differ between the
// Kuadrant and Envoy AI Gateway FMC E2E lanes: which CRD fronts dynamic
// cluster registration, and what RBAC surface each gateway's ClusterRole
// grants FMC. Everything else (scope-check semantics, resync/TTL behavior,
// resilience, token exchange) is gateway-agnostic and lives directly in the
// shared scenario functions.
type Variant interface {
	// ResourcePrefix names test-created Kubernetes objects (e.g.
	// "fmc-e2e" / "fmc-eaigw-e2e") so concurrently-run lanes never collide
	// on resource names even though they may target the same cluster type.
	ResourcePrefix() string

	// ScenarioPrefix is the BR test-ID prefix used in Describe/It text
	// (e.g. "E2E-FMC-054" / "E2E-FMC-EAIGW-054"), matching
	// docs/testing/BR-INTEGRATION-054/TEST_PLAN.md.
	ScenarioPrefix() string

	// DiscoveryLabel names the gateway + discovery CRD in human-readable
	// Describe/It text (e.g. "Kuadrant+MCPServerRegistration" /
	// "EnvoyAIGateway+Backend").
	DiscoveryLabel() string

	// DynamicResourceKind names the CRD Kind dynamic-registration creates
	// (e.g. "MCPServerRegistration" / "Backend"), used in assertion
	// failure messages.
	DynamicResourceKind() string

	// NewDynamicClusterResource builds an unstructured object of this
	// variant's discovery CRD, registered under the given namespace,
	// representing a 4th (dynamically-added) cluster named clusterID.
	NewDynamicClusterResource(namespace, clusterID string) *unstructured.Unstructured

	// RBACChecks returns the allow/deny `kubectl auth can-i` assertions
	// specific to this gateway's ClusterRole for the FMC ServiceAccount
	// (least-privilege scenario). Checks that apply regardless of gateway
	// (e.g. denying Pods/Secrets/Deployments) are NOT variant-specific and
	// are asserted directly in the shared LeastPrivilege scenario instead.
	RBACChecks() []RBACCheck
}

// RBACCheck is one `kubectl auth can-i <Verb> <Resource> --as <SA>`
// assertion: Allowed is the expected boolean result, Reason is the Gomega
// failure message explaining why.
type RBACCheck struct {
	Verb, Resource string
	Allowed        bool
	Reason         string
}
