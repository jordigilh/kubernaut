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

package gateway

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/gateway/adapters"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var _ = Describe("Gateway cache configuration (#270)", func() {

	Describe("OwnerChainCacheObjects includes all kindToGroup GVKs (BR-GATEWAY-004)", func() {
		It("should return PartialObjectMetadata entries for every kind in KindToGroup", func() {
			entries := adapters.OwnerChainCacheObjects()
			kindToGroup := adapters.KindToGroup()

			Expect(entries).ToNot(BeEmpty(), "OwnerChainCacheObjects must return entries")
			Expect(entries).To(HaveLen(len(kindToGroup)),
				"OwnerChainCacheObjects must return exactly one entry per kindToGroup kind")

			for kind, group := range kindToGroup {
				version := "v1"
				expectedGVK := schema.GroupVersionKind{Group: group, Version: version, Kind: kind}

				found := false
				for obj := range entries {
					pom, ok := obj.(*metav1.PartialObjectMetadata)
					if !ok {
						continue
					}
					if pom.GroupVersionKind() == expectedGVK {
						found = true
						break
					}
				}
				Expect(found).To(BeTrue(),
					"Cache must include PartialObjectMetadata entry for %s (GVK: %s)", kind, expectedGVK)
			}
		})
	})

	Describe("KindToGroup is the single source of truth", func() {
		It("should include all core, apps, and batch kinds needed for owner resolution", func() {
			kindToGroup := adapters.KindToGroup()

			expectedKinds := []string{
				"Pod", "Node", "Service", "ConfigMap", "Secret", "Namespace", "PersistentVolume",
				"ReplicaSet", "Deployment", "StatefulSet", "DaemonSet",
				"Job", "CronJob",
			}
			for _, kind := range expectedKinds {
				Expect(kindToGroup).To(HaveKey(kind),
					"KindToGroup must include %s for owner chain traversal", kind)
			}
		})
	})
})
