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

package datastorage

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
)

var _ = Describe("Sorted Correlation ID Extraction [BR-STORAGE-040]", func() {

	Describe("SortedCorrelationIDs", func() {

		It("UT-DS-040-001: returns correlation IDs in lexicographic order from a multi-key map", func() {
			input := map[string][]repository.AuditEvent{
				"charlie": {{CorrelationID: "charlie"}},
				"alpha":   {{CorrelationID: "alpha"}},
				"bravo":   {{CorrelationID: "bravo"}},
			}

			result := repository.SortedCorrelationIDs(input)

			Expect(result).To(HaveLen(3), "all map keys must be present")
			Expect(result).To(Equal([]string{"alpha", "bravo", "charlie"}),
				"keys must be in ascending lexicographic order for deterministic lock acquisition")
		})

		It("UT-DS-040-002: returns a single-element slice for a single-key map", func() {
			input := map[string][]repository.AuditEvent{
				"only-one": {{CorrelationID: "only-one"}},
			}

			result := repository.SortedCorrelationIDs(input)

			Expect(result).To(HaveLen(1))
			Expect(result[0]).To(Equal("only-one"))
		})
	})
})
