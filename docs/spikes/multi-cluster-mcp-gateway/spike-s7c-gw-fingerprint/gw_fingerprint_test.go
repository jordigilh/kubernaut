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

// Package spike_s7c validates the backward-compatible cluster-aware fingerprint algorithm.
// This code is NOT production code — it lives under docs/spikes/ per project convention.
package spike_s7c

import (
	"crypto/sha256"
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestSpikeS7c(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Spike S7c — GW Cluster-Aware Fingerprint")
}

// CalculateOwnerFingerprintCurrent is the CURRENT algorithm (no cluster awareness).
// This is the baseline we must remain backward-compatible with.
func CalculateOwnerFingerprintCurrent(namespace, kind, name string) string {
	input := fmt.Sprintf("%s:%s:%s", namespace, kind, name)
	hash := sha256.Sum256([]byte(input))
	return fmt.Sprintf("%x", hash)
}

// CalculateOwnerFingerprintNew is the NEW algorithm (cluster-aware).
// When clusterID is empty, it MUST produce the same result as the current algorithm.
func CalculateOwnerFingerprintNew(clusterID, namespace, kind, name string) string {
	var input string
	if clusterID != "" {
		input = fmt.Sprintf("%s:%s:%s:%s", clusterID, namespace, kind, name)
	} else {
		input = fmt.Sprintf("%s:%s:%s", namespace, kind, name)
	}
	hash := sha256.Sum256([]byte(input))
	return fmt.Sprintf("%x", hash)
}

var _ = Describe("Spike S7c — GW Cluster-Aware Fingerprint", func() {
	Describe("Backward compatibility", func() {
		It("S7c-001: empty clusterID produces same fingerprint as current algorithm", func() {
			current := CalculateOwnerFingerprintCurrent("default", "Deployment", "nginx")
			new := CalculateOwnerFingerprintNew("", "default", "Deployment", "nginx")
			Expect(new).To(Equal(current))
		})

		It("S7c-002: backward compatible across multiple resources", func() {
			testCases := []struct {
				namespace string
				kind      string
				name      string
			}{
				{"default", "Deployment", "nginx"},
				{"prod", "StatefulSet", "postgres"},
				{"kube-system", "DaemonSet", "fluentd"},
				{"monitoring", "Pod", "prometheus-0"},
				{"", "Node", "worker-1"},
			}

			for _, tc := range testCases {
				current := CalculateOwnerFingerprintCurrent(tc.namespace, tc.kind, tc.name)
				new := CalculateOwnerFingerprintNew("", tc.namespace, tc.kind, tc.name)
				Expect(new).To(Equal(current), fmt.Sprintf("Mismatch for %s/%s/%s", tc.namespace, tc.kind, tc.name))
			}
		})
	})

	Describe("Cluster-aware deduplication", func() {
		It("S7c-003: same resource on different clusters produces DIFFERENT fingerprints", func() {
			fpEast := CalculateOwnerFingerprintNew("prod-east", "default", "Deployment", "nginx")
			fpWest := CalculateOwnerFingerprintNew("prod-west", "default", "Deployment", "nginx")
			fpHub := CalculateOwnerFingerprintNew("", "default", "Deployment", "nginx")

			Expect(fpEast).ToNot(Equal(fpWest))
			Expect(fpEast).ToNot(Equal(fpHub))
			Expect(fpWest).ToNot(Equal(fpHub))
		})

		It("S7c-004: same cluster same resource produces same fingerprint (deterministic)", func() {
			fp1 := CalculateOwnerFingerprintNew("prod-east", "default", "Deployment", "nginx")
			fp2 := CalculateOwnerFingerprintNew("prod-east", "default", "Deployment", "nginx")
			Expect(fp1).To(Equal(fp2))
		})

		It("S7c-005: fingerprint is full 64-char SHA256 hex", func() {
			fp := CalculateOwnerFingerprintNew("prod-east", "default", "Deployment", "nginx")
			Expect(fp).To(HaveLen(64))
			Expect(fp).To(MatchRegexp("^[0-9a-f]{64}$"))
		})
	})

	Describe("AF compatibility check", func() {
		// AF uses a DIFFERENT separator (/) and DIFFERENT field order:
		//   rrFingerprintWithCluster: clusterID + "/" + namespace + "/" + kind + "/" + name
		// GW uses:
		//   CalculateOwnerFingerprint: namespace + ":" + kind + ":" + name
		//
		// These are INTENTIONALLY different because:
		// - AF creates RRs from user requests (different input shape)
		// - GW creates RRs from alerts (owner-resolved)
		// - They store fingerprints in the same spec.signalFingerprint field
		// - Dedup happens per-fingerprint, so different algorithms = no cross-service collision
		//
		// CONCLUSION: It's OK that AF and GW have different fingerprint formats.
		// They both independently get cluster-awareness. No shared helper needed.
		It("S7c-006: AF and GW fingerprints are different for same input (by design)", func() {
			// AF algorithm (from pkg/apifrontend/tools/af_create_rr.go)
			afFingerprint := func(clusterID, namespace, kind, name string) string {
				input := namespace + "/" + kind + "/" + name
				if clusterID != "" {
					input = clusterID + "/" + input
				}
				h := sha256.Sum256([]byte(input))
				return fmt.Sprintf("%x", h)
			}

			fpAF := afFingerprint("prod-east", "default", "Deployment", "nginx")
			fpGW := CalculateOwnerFingerprintNew("prod-east", "default", "Deployment", "nginx")

			// They should be DIFFERENT (different algorithms, different sources)
			Expect(fpAF).ToNot(Equal(fpGW))
		})
	})

	Describe("Integration with ResolveFingerprint", func() {
		// ResolveFingerprint currently calls CalculateOwnerFingerprint(owner ResourceIdentifier)
		// After the change, it needs to also accept clusterID.
		// Option A: Add clusterID to ResourceIdentifier struct
		// Option B: Add clusterID parameter to ResolveFingerprint
		// Option C: Add clusterID to NormalizedSignal and pass through
		//
		// CONCLUSION: Option C is cleanest — clusterID lives on NormalizedSignal,
		// passed to ResolveFingerprint, which passes it to CalculateOwnerFingerprint.
		It("S7c-007: ResolveFingerprint integration design - clusterID from signal", func() {
			type ResourceIdentifier struct {
				Namespace string
				Kind      string
				Name      string
			}

			// Simulated new ResolveFingerprint that accepts clusterID from signal
			resolveFingerprint := func(clusterID string, resource ResourceIdentifier) string {
				return CalculateOwnerFingerprintNew(clusterID, resource.Namespace, resource.Kind, resource.Name)
			}

			resource := ResourceIdentifier{Namespace: "prod", Kind: "Deployment", Name: "api"}

			fpLocal := resolveFingerprint("", resource)
			fpRemote := resolveFingerprint("cluster-east", resource)

			Expect(fpLocal).ToNot(Equal(fpRemote))
			Expect(fpLocal).To(Equal(CalculateOwnerFingerprintCurrent("prod", "Deployment", "api")))
		})
	})
})
