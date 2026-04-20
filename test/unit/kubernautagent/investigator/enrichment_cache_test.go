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

package investigator_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
)

var _ = Describe("TP-764: Enrichment deduplication (#764)", func() {

	Describe("UT-KA-764-001: EnrichmentCacheKey produces unique keys", func() {
		It("should produce distinct keys for different targets", func() {
			key1 := investigator.EnrichmentCacheKey("Deployment", "web", "production")
			key2 := investigator.EnrichmentCacheKey("Deployment", "web", "staging")
			key3 := investigator.EnrichmentCacheKey("StatefulSet", "web", "production")

			Expect(key1).NotTo(Equal(key2), "different namespace -> different key")
			Expect(key1).NotTo(Equal(key3), "different kind -> different key")
		})
	})

	Describe("UT-KA-764-002: EnrichmentCacheKey produces same key for identical targets", func() {
		It("should produce the same key for same (kind, name, namespace)", func() {
			key1 := investigator.EnrichmentCacheKey("Node", "worker-1", "")
			key2 := investigator.EnrichmentCacheKey("Node", "worker-1", "")

			Expect(key1).To(Equal(key2), "same target -> same key")
		})
	})
})
