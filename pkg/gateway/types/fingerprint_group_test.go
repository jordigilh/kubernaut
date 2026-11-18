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

package types_test

import (
	"context"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/gateway/types"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// Issue #2650: the fingerprint hash gains an API-group dimension so the same
// namespace/kind/name in two API groups (e.g. route.openshift.io Route vs
// serving.knative.dev Route) produces distinct dedup fingerprints
// (BR-GATEWAY-004). Group extraction uses scope.ParseAPIVersion's semantics:
// empty apiVersion -> empty group segment (legitimate prometheus
// ambiguous-registry state, residual #2066 gap); a malformed non-empty
// apiVersion must NOT collapse onto the empty-group hash (raw-string
// fallback). The version within a group is deliberately NOT part of the
// dimension: apps/v1 and apps/v2 are the same dedup target.
var _ = Describe("Fingerprint group dimension (#2650, BR-GATEWAY-004)", func() {
	route := func(apiVersion string) types.ResourceIdentifier {
		return types.ResourceIdentifier{
			Namespace:  "production",
			Kind:       "Route",
			Name:       "shared-route",
			APIVersion: apiVersion,
		}
	}

	Describe("Group distinctness", func() {
		It("UT-GW-2650-001: same ns/kind/name in two API groups produces distinct fingerprints", func() {
			openshift := types.CalculateClusterAwareFingerprint("", route("route.openshift.io/v1"))
			knative := types.CalculateClusterAwareFingerprint("", route("serving.knative.dev/v1"))

			Expect(openshift).NotTo(Equal(knative),
				"BR-GATEWAY-004: route.openshift.io Route and serving.knative.dev Route are different resources and must not share a dedup fingerprint")
		})

		It("UT-GW-2650-002: same resource (same group) produces a deterministic identical hash", func() {
			fp1 := types.CalculateClusterAwareFingerprint("", route("route.openshift.io/v1"))
			fp2 := types.CalculateClusterAwareFingerprint("", route("route.openshift.io/v1"))

			Expect(fp1).To(Equal(fp2),
				"determinism: identical inputs must produce identical fingerprints")
		})

		It("UT-GW-2650-003: cluster and group dimensions combine independently", func() {
			base := types.CalculateClusterAwareFingerprint("cluster-a", route("route.openshift.io/v1"))
			otherCluster := types.CalculateClusterAwareFingerprint("cluster-b", route("route.openshift.io/v1"))
			otherGroup := types.CalculateClusterAwareFingerprint("cluster-a", route("serving.knative.dev/v1"))

			Expect(base).NotTo(Equal(otherCluster),
				"different cluster, same group must be distinct")
			Expect(base).NotTo(Equal(otherGroup),
				"different group, same cluster must be distinct")
			Expect(otherCluster).NotTo(Equal(otherGroup),
				"both dimensions differing must also be distinct from each other")
		})
	})

	Describe("Empty group segment", func() {
		It("UT-GW-2650-004: empty apiVersion is stable and distinct from any non-empty group", func() {
			empty1 := types.CalculateClusterAwareFingerprint("", route(""))
			empty2 := types.CalculateClusterAwareFingerprint("", route(""))
			core := types.CalculateClusterAwareFingerprint("", route("v1"))
			grouped := types.CalculateClusterAwareFingerprint("", route("route.openshift.io/v1"))

			Expect(empty1).To(Equal(empty2),
				"empty apiVersion (prometheus ambiguous-registry state) must hash deterministically")
			// Core "v1" IS the empty group — the ambiguous case shares its
			// segment by design. No real collision exists: ambiguity only
			// arises for kinds served in >1 group, which core never is.
			Expect(empty1).To(Equal(core),
				"core (empty-group) and ambiguous (empty apiVersion) share the empty group segment by design")
			Expect(empty1).NotTo(Equal(grouped),
				"empty group (ambiguous) must not collide with a named group")
		})
	})

	Describe("Malformed apiVersion raw-string fallback", func() {
		It("UT-GW-2650-005: malformed non-empty apiVersion does not collapse to the empty-group hash", func() {
			// "apps" is a slash-less group name: schema.ParseGroupVersion silently
			// accepts it as {Group:"", Version:"apps"} — the exact bug class the
			// canonical scope.ParseAPIVersion parser rejects. The fingerprint must
			// use the raw string as its segment instead of collapsing onto the
			// empty-group hash.
			slashlessGroup := types.CalculateClusterAwareFingerprint("", route("apps"))
			multiSlash := types.CalculateClusterAwareFingerprint("", route("a/b/c"))
			empty := types.CalculateClusterAwareFingerprint("", route(""))

			Expect(slashlessGroup).NotTo(Equal(empty),
				"a slash-less group name must not be treated as the empty group")
			Expect(multiSlash).NotTo(Equal(empty),
				"a multi-slash apiVersion must not be treated as the empty group")
			Expect(slashlessGroup).NotTo(Equal(multiSlash),
				"distinct malformed inputs must remain distinct (raw-string fallback)")
		})
	})

	Describe("Group-only dimension", func() {
		It("UT-GW-2650-006: apps/v1 and apps/v2 (same group) produce the SAME fingerprint", func() {
			v1 := types.CalculateClusterAwareFingerprint("", types.ResourceIdentifier{
				Namespace: "production", Kind: "Deployment", Name: "payment-api", APIVersion: "apps/v1",
			})
			v2 := types.CalculateClusterAwareFingerprint("", types.ResourceIdentifier{
				Namespace: "production", Kind: "Deployment", Name: "payment-api", APIVersion: "apps/v2",
			})

			Expect(v1).To(Equal(v2),
				"#2650 decision: the group is the dimension, not the version — same-group versions are the same dedup target")
		})
	})

	Describe("Owner-path group seeding (M1)", func() {
		// registryMissResolver simulates the discovery gap: an
		// OwnerResolver whose identity resolution returns the standalone
		// resource without an apiVersion (registry miss, nil registry —
		// post-#2593 the production resolver fails closed here instead,
		// so the gateway-level seed is defense in depth for resolvers
		// that return a versionless identity without erroring).
		resolver := &registryMissFakeResolver{}

		It("UT-GW-2650-007: resource apiVersion known, resolution returns empty apiVersion -> fingerprint carries the resource's group", func() {
			standalone := types.ResourceIdentifier{
				Namespace: "production", Kind: "Route", Name: "shared-route", APIVersion: "route.openshift.io/v1",
			}

			fp, resolved, err := types.ResolveFingerprintWithCluster(context.Background(), "", resolver, standalone, logr.Discard())

			Expect(err).NotTo(HaveOccurred())
			Expect(resolved.APIVersion).To(Equal("route.openshift.io/v1"),
				"the input resource's apiVersion must seed the resolved identity when resolution cannot supply one")
			expected := types.CalculateClusterAwareFingerprint("", types.ResourceIdentifier{
				Namespace: "production", Kind: "Route", Name: "shared-route", APIVersion: "route.openshift.io/v1",
			})
			Expect(fp).To(Equal(expected),
				"the seeded group must reach the fingerprint hash (M1: registry-miss standalone resource)")
		})

		It("UT-GW-2650-008: a DIFFERENT owner with empty apiVersion is not seeded from the child resource (fail-closed)", func() {
			// Degenerate controllerRef without apiVersion: the resolved owner
			// is another resource and carries no version. The child's group
			// must NOT be attributed to the owner — the owner keeps its empty
			// version and the #2592 strict identity gate rejects it
			// downstream rather than guessing.
			differentOwnerResolver := &differentOwnerNoVersionResolver{}
			pod := types.ResourceIdentifier{
				Namespace: "production", Kind: "Pod", Name: "shared-route-abc", APIVersion: "v1",
			}

			fp, resolved, err := types.ResolveFingerprintWithCluster(context.Background(), "", differentOwnerResolver, pod, logr.Discard())

			Expect(err).NotTo(HaveOccurred())
			Expect(resolved.APIVersion).To(BeEmpty(),
				"the child's apiVersion must not be attributed to a different owner")
			Expect(resolved.Kind).To(Equal("StatefulSet"))
			expected := types.CalculateClusterAwareFingerprint("", types.ResourceIdentifier{
				Namespace: "production", Kind: "StatefulSet", Name: "shared-route",
			})
			Expect(fp).To(Equal(expected),
				"the fingerprint must hash the owner with the empty group segment, not the child's group")
		})
	})
})

// differentOwnerNoVersionResolver resolves to a different owner (StatefulSet)
// with an empty APIVersion, reproducing a controllerRef that carries no
// apiVersion (real Kubernetes always populates it).
type differentOwnerNoVersionResolver struct{}

func (f *differentOwnerNoVersionResolver) ResolveTopLevelOwnerIdentity(_ context.Context, _ sharedtypes.OwnerChainEntry) (sharedtypes.OwnerChainEntry, error) {
	return sharedtypes.OwnerChainEntry{Namespace: "production", Kind: "StatefulSet", Name: "shared-route"}, nil
}

// registryMissFakeResolver implements the OwnerResolver interface but
// returns the standalone identity with an empty APIVersion, reproducing the
// registry-miss/nil-registry gap where the GVK lookup cannot map a grouped
// kind (a resolver that does not fail closed on a versionless source).
type registryMissFakeResolver struct{}

func (f *registryMissFakeResolver) ResolveTopLevelOwnerIdentity(_ context.Context, resource sharedtypes.OwnerChainEntry) (sharedtypes.OwnerChainEntry, error) {
	return sharedtypes.OwnerChainEntry{Namespace: resource.Namespace, Kind: resource.Kind, Name: resource.Name}, nil
}
