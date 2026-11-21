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

// Package-internal spec: it proves the event adapter normalizes a discovered
// cluster-scoped CRD's namespace before fingerprint resolution, using the same
// registry the scope gate uses (#2670). Ginkgo registers it in the same binary
// as the adapters_test specs, which own RunSpecs.
package adapters

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/gateway/types"
)

// clusterScopedCRDEvent is a Warning event whose involvedObject claims a
// namespace even though its Kind is cluster-scoped.
const clusterScopedCRDEvent = `{
  "type": "Warning",
  "reason": "VolumeDegraded",
  "involvedObject": {
    "kind": "ClusterVolume",
    "namespace": "monitoring",
    "name": "vol-1",
    "apiVersion": "storage.example.com/v1"
  },
  "firstTimestamp": "2026-01-01T00:00:00Z"
}`

var _ = Describe("Kubernetes event adapter target scope normalization (#2670)", func() {

	It("UT-GW-2670-024: a discovered cluster-scoped CRD event hashes without the claimed namespace", func() {
		registry, err := NewAPIResourceRegistry(discoveryWithClusterScopedCRD())
		Expect(err).ToNot(HaveOccurred())

		adapter := NewKubernetesEventAdapter()
		adapter.SetRegistry(registry)

		signal, err := adapter.Parse(context.Background(), []byte(clusterScopedCRDEvent))
		Expect(err).ToNot(HaveOccurred())
		Expect(signal.Resource.Namespace).To(BeEmpty(),
			"the RR target is cluster-scoped, so the event lane must not keep the claimed namespace")
		Expect(signal.Fingerprint).To(Equal(types.CalculateClusterAwareFingerprint("", types.ResourceIdentifier{
			Kind: "ClusterVolume", Name: "vol-1", APIVersion: "storage.example.com/v1",
		})), "dedup key and RR target must agree")
	})

	It("UT-GW-2670-025: without a registry only static cluster-scoped kinds are normalized", func() {
		adapter := NewKubernetesEventAdapter()

		signal, err := adapter.Parse(context.Background(), []byte(clusterScopedCRDEvent))
		Expect(err).ToNot(HaveOccurred())
		Expect(signal.Resource.Namespace).To(Equal("monitoring"),
			"a nil registry cannot classify a CRD; the event keeps what it claims")
	})
})
