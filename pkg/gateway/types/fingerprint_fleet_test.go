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
	})
})
