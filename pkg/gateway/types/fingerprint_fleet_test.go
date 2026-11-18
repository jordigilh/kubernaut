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

package types_test

import (
	"context"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/gateway/types"
)

var _ = Describe("Cluster-Aware Fingerprint (BR-INTEGRATION-065)", func() {
	resource := types.ResourceIdentifier{
		Namespace: "production",
		Kind:      "Deployment",
		Name:      "payment-api",
	}

	Describe("Backward Compatibility", func() {
		It("UT-GW-FLEET-001: empty clusterID produces same hash as legacy CalculateOwnerFingerprint", func() {
			legacy := types.CalculateOwnerFingerprint(resource)
			clusterAware := types.CalculateClusterAwareFingerprint("", resource)

			Expect(clusterAware).To(Equal(legacy),
				"Empty clusterID must produce identical fingerprint for backward compatibility")
		})
	})

	Describe("Cluster Dimension", func() {
		It("UT-GW-FLEET-002: non-empty clusterID produces different hash than empty clusterID", func() {
			withoutCluster := types.CalculateClusterAwareFingerprint("", resource)
			withCluster := types.CalculateClusterAwareFingerprint("prod-east", resource)

			Expect(withCluster).NotTo(Equal(withoutCluster),
				"Cluster dimension must change fingerprint to prevent cross-cluster deduplication")
		})

		It("UT-GW-FLEET-003: different clusterIDs produce different hashes for same resource", func() {
			east := types.CalculateClusterAwareFingerprint("prod-east", resource)
			west := types.CalculateClusterAwareFingerprint("prod-west", resource)

			Expect(east).NotTo(Equal(west),
				"Same resource on different clusters must have distinct fingerprints")
		})
	})

	Describe("Determinism", func() {
		It("UT-GW-FLEET-004: same inputs produce same hash (deterministic)", func() {
			fp1 := types.CalculateClusterAwareFingerprint("prod-east", resource)
			fp2 := types.CalculateClusterAwareFingerprint("prod-east", resource)

			Expect(fp1).To(Equal(fp2),
				"Fingerprint must be deterministic for deduplication correctness")
		})
	})

	Describe("ClusterLabelKey constant", func() {
		It("UT-GW-FLEET-005: ClusterLabelKey equals 'cluster' (Thanos convention)", func() {
			Expect(types.ClusterLabelKey).To(Equal("cluster"),
				"Must match Thanos external label convention")
		})
	})

	Describe("ResolveFingerprintWithCluster", func() {
		It("UT-GW-FLEET-006: ResolveFingerprint delegates to ResolveFingerprintWithCluster with empty clusterID", func() {
			legacyFP := types.CalculateOwnerFingerprint(resource)
			clusterAwareFP := types.CalculateClusterAwareFingerprint("", resource)

			Expect(legacyFP).To(Equal(clusterAwareFP),
				"ResolveFingerprint must delegate to cluster-aware variant with empty clusterID")
		})

		// #2592: owner resolution must preserve the resolved apiVersion —
		// discovered live on SNO-a: every production Pod alert lost its GVK
		// here because the owner ResourceIdentifier was rebuilt without it,
		// and the strict scope gate then (correctly) rejected the signal.
		It("UT-GW-2592-FP-001 [BR-SCOPE-001, AC-4, SI-10]: owner resolution preserves the resolved apiVersion", func() {
			resolver := &apiVersionAwareFakeResolver{}
			pod := types.ResourceIdentifier{Namespace: "production", Kind: "Pod", Name: "pod-abc", APIVersion: "v1"}

			fp, resolved, err := types.ResolveFingerprintWithCluster(context.Background(), "", resolver, pod, logr.Discard())

			Expect(err).NotTo(HaveOccurred())
			Expect(fp).NotTo(BeEmpty())
			Expect(resolved.Kind).To(Equal("Deployment"))
			Expect(resolved.Name).To(Equal("payment-api"))
			Expect(resolved.APIVersion).To(Equal("apps/v1"),
				"the owner's apiVersion from the resolver must survive into the signal target (strict-GVK contract)")
		})

		It("UT-GW-2592-FP-002 [BR-SCOPE-001, AC-4, SI-10]: legacy resolvers without apiVersion leave it empty, never guessed", func() {
			resolver := &legacyFakeResolver{}
			pod := types.ResourceIdentifier{Namespace: "production", Kind: "Pod", Name: "pod-abc", APIVersion: "v1"}

			_, resolved, err := types.ResolveFingerprintWithCluster(context.Background(), "", resolver, pod, logr.Discard())

			Expect(err).NotTo(HaveOccurred())
			Expect(resolved.APIVersion).To(BeEmpty(),
				"a legacy resolver cannot supply the owner GVK; the identity stays empty so the strict gate rejects it (fail-closed)")
		})

		// #2649: the input signal's apiVersion must be threaded into owner
		// resolution so the resolver seeds the chain's initial GVK from it
		// instead of guessing the group from the kind name alone (#2593's
		// ResolveTopLevelOwnerIdentity carries it in the entry).
		It("UT-GW-2649-FP-001 [BR-GATEWAY-004, BR-SCOPE-001, AC-4, SI-10]: passes the input resource's apiVersion into the resolver entry", func() {
			resolver := &capturingIdentityResolver{}
			pod := types.ResourceIdentifier{Namespace: "production", Kind: "Pod", Name: "pod-abc", APIVersion: "v1"}

			_, resolved, err := types.ResolveFingerprintWithCluster(context.Background(), "", resolver, pod, logr.Discard())

			Expect(err).NotTo(HaveOccurred())
			Expect(resolver.calls).To(Equal(1))
			Expect(resolver.gotAPIVersion).To(Equal("v1"),
				"the input signal's apiVersion must reach the resolver entry so it can seed the initial GVK (#2649)")
			Expect(resolver.gotKind).To(Equal("Pod"))
			Expect(resolver.gotName).To(Equal("pod-abc"))
			Expect(resolved.Kind).To(Equal("Deployment"))
			Expect(resolved.APIVersion).To(Equal("apps/v1"))
		})

		// A Service-kind signal carries an apiVersion the resolver must
		// receive so the Service early-return can verify its passthrough
		// against the observed group instead of fabricating "v1". This spec
		// pins the fingerprint path's threading contract for the Service kind
		// specifically (the kind whose resolver branch changes behavior based
		// on the identity).
		It("UT-GW-2649-FP-004 [BR-GATEWAY-004, BR-SCOPE-001, AC-4, SI-10]: threads a Service resource's apiVersion into the resolver entry", func() {
			resolver := &capturingIdentityResolver{}
			svc := types.ResourceIdentifier{Namespace: "production", Kind: "Service", Name: "edge-svc", APIVersion: "custom.example/v1"}

			_, resolved, err := types.ResolveFingerprintWithCluster(context.Background(), "prod-east", resolver, svc, logr.Discard())

			Expect(err).NotTo(HaveOccurred())
			Expect(resolver.calls).To(Equal(1))
			Expect(resolver.gotAPIVersion).To(Equal("custom.example/v1"),
				"a Service signal's apiVersion must reach the resolver so the passthrough is verified, not fabricated (#2649)")
			Expect(resolver.gotKind).To(Equal("Service"))
			Expect(resolved.Kind).To(Equal("Deployment"))
			Expect(resolved.APIVersion).To(Equal("apps/v1"))
		})
	})
})

// capturingIdentityResolver records the OwnerChainEntry it was called with,
// proving ResolveFingerprintWithCluster threads the signal's apiVersion into
// owner resolution (#2649) through #2593's single identity-carrying method.
type capturingIdentityResolver struct {
	calls         int
	gotKind       string
	gotName       string
	gotAPIVersion string
}

func (f *capturingIdentityResolver) ResolveTopLevelOwnerIdentity(_ context.Context, resource sharedtypes.OwnerChainEntry) (sharedtypes.OwnerChainEntry, error) {
	f.calls++
	f.gotKind, f.gotName, f.gotAPIVersion = resource.Kind, resource.Name, resource.APIVersion
	return sharedtypes.OwnerChainEntry{Kind: "Deployment", Name: "payment-api", APIVersion: "apps/v1"}, nil
}

// apiVersionAwareFakeResolver implements the APIVersion-carrying owner
// resolution the production K8sOwnerResolver provides (DD-FLEET-014).
type apiVersionAwareFakeResolver struct{}

func (f *apiVersionAwareFakeResolver) ResolveTopLevelOwnerIdentity(_ context.Context, _ sharedtypes.OwnerChainEntry) (sharedtypes.OwnerChainEntry, error) {
	return sharedtypes.OwnerChainEntry{Kind: "Deployment", Name: "payment-api", APIVersion: "apps/v1"}, nil
}

// legacyFakeResolver supplies an owner entry without an apiVersion: the
// identity stays incomplete so the strict scope gate rejects it (fail-closed).
type legacyFakeResolver struct{}

func (f *legacyFakeResolver) ResolveTopLevelOwnerIdentity(_ context.Context, _ sharedtypes.OwnerChainEntry) (sharedtypes.OwnerChainEntry, error) {
	return sharedtypes.OwnerChainEntry{Kind: "Deployment", Name: "payment-api"}, nil
}
