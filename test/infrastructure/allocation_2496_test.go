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

// These source guards prevent recurrence of the scoped CodeQL allocation
// findings. They are not additional business scenarios or runtime overflow
// simulations: the existing behavior specs prove history and dispatch outcomes.
var _ = Describe("Bounded history and phase-tool allocations [BR-KA-016, BR-KA-017]", func() {
	DescribeTable("avoids unchecked length sums in allocation capacity", func(path, name string) {
		source := readSecurityFixtureSource(path)
		declaration := regexp.MustCompile(`(?ms)^func (?:\([^\n]*\) )?` + regexp.QuoteMeta(name) + `\([^\n]*\).*?^}`).FindString(source)
		Expect(declaration).NotTo(BeEmpty(), "production function %s must be found", name)
		Expect(declaration).NotTo(MatchRegexp(`make\([^\n]*,\s*len\([^\n]*\)\s*\+`),
			"CodeQL go/allocation-size-overflow: append must grow capacity without unchecked length addition")
	},
		Entry("UT-KA-2496-ALLOC-001: frozen prior attempts", "internal/kubernautagent/investigator/evidence_handoff.go", "projectHandoffHistory"),
		Entry("UT-KA-2496-ALLOC-002: chronological history", "internal/kubernautagent/prompt/history.go", "renderChronologicalHistory"),
		Entry("UT-KA-2496-ALLOC-003: existing history projection", "internal/kubernautagent/prompt/history.go", "toHistoryEntries"),
		Entry("UT-KA-2498-ALLOC-001: phase-scoped tool definitions", "internal/kubernautagent/investigator/investigator_tools.go", "toolDefinitionsForPhase"),
	)
})
