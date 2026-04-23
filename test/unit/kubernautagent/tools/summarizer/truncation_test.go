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

package summarizer_test

import (
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/summarizer"
)

var _ = Describe("Kubernaut Agent Tool Output Truncation — #752", func() {

	Describe("UT-KA-752-001: Hard truncation applied to oversized output", func() {
		It("should truncate output exceeding the limit and append guidance hint", func() {
			limit := 1000
			toolName := "kubectl_get_by_kind_in_cluster"
			longOutput := strings.Repeat("x", 2000)

			result := summarizer.TruncateToolOutput(longOutput, toolName, limit)

			Expect(len(result)).To(BeNumerically("<=", limit+300),
				"truncated result should not vastly exceed limit (hint overhead)")
			Expect(result).To(ContainSubstring("[TRUNCATED]"),
				"should contain truncation marker")
			Expect(result).To(ContainSubstring(toolName),
				"hint should reference the tool name")
			Expect(result).To(ContainSubstring("2000"),
				"hint should mention original output size")
		})
	})

	Describe("UT-KA-752-002: Below-limit output passes through unchanged", func() {
		It("should return input unchanged when at or below limit", func() {
			limit := 100000
			shortOutput := "metric_value=42"

			result := summarizer.TruncateToolOutput(shortOutput, "kubectl_describe", limit)

			Expect(result).To(Equal(shortOutput),
				"should pass through unchanged")
		})

		It("should return input unchanged when exactly at limit", func() {
			limit := 500
			exactOutput := strings.Repeat("a", 500)

			result := summarizer.TruncateToolOutput(exactOutput, "kubectl_describe", limit)

			Expect(result).To(Equal(exactOutput),
				"should pass through unchanged when exactly at limit")
		})
	})

	Describe("UT-KA-752-005: Truncation hint includes output size and guidance", func() {
		It("should include original character count and tool-specific guidance", func() {
			limit := 500
			toolName := "kubectl_get_by_kind_in_cluster"
			longOutput := strings.Repeat("z", 5000)

			result := summarizer.TruncateToolOutput(longOutput, toolName, limit)

			Expect(result).To(ContainSubstring(fmt.Sprintf("%d", 5000)),
				"hint should include original output length")
			Expect(result).To(ContainSubstring("kubectl_get_by_name"),
				"hint should suggest using targeted tool for cluster-wide listings")
		})

		It("should include generic guidance for non-listing tools", func() {
			limit := 500
			toolName := "kubectl_logs"
			longOutput := strings.Repeat("log", 1000)

			result := summarizer.TruncateToolOutput(longOutput, toolName, limit)

			Expect(result).To(ContainSubstring("[TRUNCATED]"),
				"should contain truncation marker")
			Expect(result).To(ContainSubstring(fmt.Sprintf("%d", 3000)),
				"hint should include original output length")
		})
	})
})
