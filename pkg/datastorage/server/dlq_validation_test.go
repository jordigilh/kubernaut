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

package server

import (
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/dlq"
)

var _ = Describe("DLQ EventData Validation (#1048 Phase 4 / SC-5, SI-10)", func() {

	Describe("dlq.ValidateEventData", func() {
		It("UT-DS-1048-ED-001: should accept valid small EventData", func() {
			data := []byte(`{"key":"value","nested":{"a":1}}`)
			Expect(dlq.ValidateEventData(data)).To(Succeed())
		})

		It("UT-DS-1048-ED-002: should accept empty EventData", func() {
			Expect(dlq.ValidateEventData(nil)).To(Succeed())
			Expect(dlq.ValidateEventData([]byte{})).To(Succeed())
		})

		It("UT-DS-1048-ED-003: should reject EventData exceeding 256 KB", func() {
			data := []byte(`{"data":"` + strings.Repeat("x", dlq.MaxEventDataSize+1) + `"}`)
			err := dlq.ValidateEventData(data)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("exceeds maximum size"))
		})

		It("UT-DS-1048-ED-004: should accept EventData at exactly 256 KB boundary", func() {
			framing := len(`{"d":"` + `"}`)
			fill := dlq.MaxEventDataSize - framing
			data := []byte(`{"d":"` + strings.Repeat("x", fill) + `"}`)
			Expect(len(data)).To(Equal(dlq.MaxEventDataSize), "test payload should be exactly MaxEventDataSize")
			Expect(dlq.ValidateEventData(data)).To(Succeed())
		})
	})

	Describe("dlq.ValidateJSONDepth", func() {
		It("UT-DS-1048-JD-001: should accept flat JSON", func() {
			data := []byte(`{"a":1,"b":"two","c":true}`)
			Expect(dlq.ValidateJSONDepth(data, 10)).To(Succeed())
		})

		It("UT-DS-1048-JD-002: should accept JSON within depth limit", func() {
			data := []byte(`{"a":{"b":{"c":{"d":"value"}}}}`)
			Expect(dlq.ValidateJSONDepth(data, 10)).To(Succeed())
		})

		It("UT-DS-1048-JD-003: should reject JSON exceeding depth limit", func() {
			deep := buildDeepJSON(15)
			err := dlq.ValidateJSONDepth([]byte(deep), 10)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("nesting depth exceeds maximum"))
		})

		It("UT-DS-1048-JD-004: should handle arrays in depth counting", func() {
			data := []byte(`{"a":[[[[[[[[[[[1]]]]]]]]]]]}}`)
			err := dlq.ValidateJSONDepth(data, 5)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("nesting depth exceeds maximum"))
		})

		It("UT-DS-1048-JD-005: should accept JSON at exactly the depth limit", func() {
			deep := buildDeepJSON(10)
			Expect(dlq.ValidateJSONDepth([]byte(deep), 10)).To(Succeed())
		})

		It("UT-DS-1048-JD-006: should handle invalid JSON", func() {
			data := []byte(`{"broken": !!!}`)
			err := dlq.ValidateJSONDepth(data, 10)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid JSON"))
		})
	})
})

func buildDeepJSON(depth int) string {
	var sb strings.Builder
	for i := 0; i < depth; i++ {
		sb.WriteString(fmt.Sprintf(`{"l%d":`, i))
	}
	sb.WriteString(`"leaf"`)
	for i := 0; i < depth; i++ {
		sb.WriteString("}")
	}
	return sb.String()
}
