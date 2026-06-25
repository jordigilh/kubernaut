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

package acm

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

// UT-ACM-054-FILTER: buildFilters unit tests
// Authority: ADR-068 (Fleet Federation Architecture)
// FedRAMP: SI-10 (Information Input Validation) -- filter construction
var _ = Describe("UT-ACM-054-FILTER: buildFilters", func() {
	It("UT-ACM-054-FILTER-001: should include all fields when fully populated", func() {
		filters := buildFilters(scope.ResourceIdentity{
			ClusterID: "prod-east",
			Group:     "apps",
			Kind:      "Deployment",
			Namespace: "default",
			Name:      "nginx",
		})
		Expect(filters).To(HaveLen(5))

		propertyMap := make(map[string]string)
		for _, f := range filters {
			if len(f.Values) > 0 {
				propertyMap[f.Property] = f.Values[0]
			}
		}
		Expect(propertyMap).To(HaveKeyWithValue("kind", "Deployment"))
		Expect(propertyMap).To(HaveKeyWithValue("name", "nginx"))
		Expect(propertyMap).To(HaveKeyWithValue("namespace", "default"))
		Expect(propertyMap).To(HaveKeyWithValue("cluster", "prod-east"))
		Expect(propertyMap).To(HaveKeyWithValue("apigroup", "apps"))
	})

	It("UT-ACM-054-FILTER-002: should omit optional fields when empty", func() {
		filters := buildFilters(scope.ResourceIdentity{
			Kind: "Node",
			Name: "worker-1",
		})

		properties := make([]string, 0, len(filters))
		for _, f := range filters {
			properties = append(properties, f.Property)
		}
		Expect(properties).To(ContainElement("kind"))
		Expect(properties).To(ContainElement("name"))
		Expect(properties).ToNot(ContainElement("namespace"))
		Expect(properties).ToNot(ContainElement("cluster"))
		Expect(properties).ToNot(ContainElement("apigroup"))
	})
})
