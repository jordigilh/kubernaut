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

package infrastructure

import (
	"regexp"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Scheduling and transport-identity guards are not extra business scenarios.
var _ = Describe("Isolated history journey scheduling [BR-KA-016]", func() {
	It("UT-PLATFORM-2496-PARALLEL-001: independent history journeys use the normal parallel scheduler", func() {
		source := readSecurityFixtureSource("test/e2e/fullpipeline/history_selection_2496_test.go")
		serialDeclaration := regexp.MustCompile(`Describe\("History-aware selection journeys[^\n]*\bSerial\b`).FindString(source)
		Expect(serialDeclaration).To(BeEmpty())
	})

	It("UT-PLATFORM-2496-PARALLEL-002: A2A message and context identities are scoped to the unique RR", func() {
		source := readSecurityFixtureSource("test/e2e/fullpipeline/history_selection_2496_test.go")
		call := regexp.MustCompile(`fpA2AMessageStreamWithContext\(([^,\n]+),([^,\n]+),`).FindStringSubmatch(source)
		Expect(call).To(HaveLen(3))
		Expect(call[1]).To(ContainSubstring("rrName"), "JSON-RPC/message ID must not be shared between tests")
		Expect(call[2]).To(ContainSubstring("rrName"), "A2A context must remain RR-specific")
	})
})
