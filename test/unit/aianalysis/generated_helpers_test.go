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

// BR-AI-001: Generated helper functions for extracting values from ogen types
package aianalysis

import (
	"encoding/json"

	"github.com/go-faster/jx"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/agentclient"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
)

var _ = Describe("UT-AA-668-002: Generated Helper Functions", func() {

	Describe("GetOptNilStringValue", func() {
		DescribeTable("BR-AI-008: should extract string from OptNilString correctly",
			func(opt agentclient.OptNilString, expected string) {
				result := handlers.GetOptNilStringValue(opt)
				Expect(result).To(Equal(expected))
			},
			Entry("set, non-null value",
				agentclient.OptNilString{Value: "hello", Set: true, Null: false},
				"hello"),
			Entry("set but null",
				agentclient.OptNilString{Value: "ignored", Set: true, Null: true},
				""),
			Entry("not set",
				agentclient.OptNilString{Value: "", Set: false, Null: false},
				""),
			Entry("not set and null",
				agentclient.OptNilString{Set: false, Null: true},
				""),
		)
	})

	Describe("GetMapFromMapSafe", func() {
		It("BR-AI-008: should extract nested map when key exists", func() {
			m := map[string]interface{}{
				"nested": map[string]interface{}{"inner": "value"},
			}
			result := handlers.GetMapFromMapSafe(m, "nested")
			Expect(result).To(HaveKeyWithValue("inner", "value"))
		})

		It("BR-AI-008: should return nil when key does not exist", func() {
			m := map[string]interface{}{"other": "data"}
			result := handlers.GetMapFromMapSafe(m, "missing")
			Expect(result).To(BeNil())
		})

		It("BR-AI-008: should return nil when value is not a map", func() {
			m := map[string]interface{}{"key": "string-value"}
			result := handlers.GetMapFromMapSafe(m, "key")
			Expect(result).To(BeNil())
		})

		It("BR-AI-008: should return nil for nil input map", func() {
			result := handlers.GetMapFromMapSafe(nil, "any")
			Expect(result).To(BeNil())
		})
	})

	Describe("GetMapFromJxRaw", func() {
		It("BR-AI-008: should extract map from valid JSON raw bytes", func() {
			data := map[string]interface{}{"key": "value", "num": float64(42)}
			bytes, err := json.Marshal(data)
			Expect(err).NotTo(HaveOccurred())

			result, err := handlers.GetMapFromJxRaw(jx.Raw(bytes))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(HaveKeyWithValue("key", "value"))
			Expect(result).To(HaveKeyWithValue("num", float64(42)))
		})

		It("BR-AI-008: should return error for invalid JSON", func() {
			_, err := handlers.GetMapFromJxRaw(jx.Raw([]byte("not-json")))
			Expect(err).To(HaveOccurred())
		})

		It("BR-AI-008: should handle empty JSON object", func() {
			result, err := handlers.GetMapFromJxRaw(jx.Raw([]byte("{}")))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeEmpty())
		})
	})
})
