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

package main

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	kaconfig "github.com/jordigilh/kubernaut/internal/kubernautagent/config"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"k8s.io/utils/ptr"
)

// mergeLLMConfig's Temperature handling — #1749: the previous
// `if rt.Temperature != 0` check conflated "not configured" with
// "explicitly configured as 0", violating BR-KA-266 and forcing a
// numeric temperature onto models (e.g. claude-opus-4-8) that reject the
// parameter outright with a 400. Temperature is now a *float64 all the
// way through, so mergeLLMConfig copies the pointer directly instead of
// checking its value.
var _ = Describe("mergeLLMConfig — temperature pointer semantics (#1749)", func() {

	Describe("UT-KA-1749-007: omits temperature from the merged LLMConfig when not configured", func() {
		It("should leave merged.Temperature nil when LLMRuntimeConfig.Temperature is nil", func() {
			merged := mergeLLMConfig(types.LLMConfig{Provider: "anthropic"}, &kaconfig.LLMRuntimeConfig{
				Model: "claude-opus-4-8",
			})

			Expect(merged.Temperature).To(BeNil(),
				"temperature must be omitted from the merged config when not explicitly configured "+
					"(fixes claude-opus-4-8 400 'temperature is deprecated for this model')")
		})
	})

	Describe("UT-KA-1749-008 (BR-KA-266): preserves an explicit temperature of 0", func() {
		It("should set merged.Temperature to 0 when LLMRuntimeConfig.Temperature is an explicit pointer to 0.0", func() {
			merged := mergeLLMConfig(types.LLMConfig{Provider: "openai"}, &kaconfig.LLMRuntimeConfig{
				Model:       "gpt-4o",
				Temperature: ptr.To(0.0),
			})

			Expect(merged.Temperature).NotTo(BeNil(),
				"an explicit temperature of 0 must survive the merge — BR-KA-266 requires "+
					"deterministic output to be an explicit configuration, not confused with 'unset'")
			Expect(*merged.Temperature).To(Equal(0.0))
		})
	})

	Describe("UT-KA-1749-009: preserves a non-zero explicit temperature", func() {
		It("should set merged.Temperature to the configured value", func() {
			merged := mergeLLMConfig(types.LLMConfig{Provider: "openai"}, &kaconfig.LLMRuntimeConfig{
				Model:       "gpt-4o",
				Temperature: ptr.To(0.7),
			})

			Expect(merged.Temperature).NotTo(BeNil())
			Expect(*merged.Temperature).To(Equal(0.7))
		})
	})
})
