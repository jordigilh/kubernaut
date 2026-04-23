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

package parser_test

import (
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
)

var _ = Describe("Typed Parse Errors — #760", func() {

	Describe("UT-KA-760-001: Parse empty content returns ErrEmptyContent", func() {
		It("should return ErrEmptyContent identifiable via errors.As", func() {
			p := parser.NewResultParser()
			result, err := p.Parse("")
			Expect(err).To(HaveOccurred())
			Expect(result).To(BeNil())

			var emptyErr *parser.ErrEmptyContent
			Expect(errors.As(err, &emptyErr)).To(BeTrue(),
				"empty content must return ErrEmptyContent")
		})
	})

	Describe("UT-KA-760-002: Parse free text returns ErrNoJSON", func() {
		It("should return ErrNoJSON identifiable via errors.As", func() {
			p := parser.NewResultParser()
			content := "After reviewing the 21 registered workflows, none can adjust namespace quotas."
			result, err := p.Parse(content)
			Expect(err).To(HaveOccurred())
			Expect(result).To(BeNil())

			var noJSON *parser.ErrNoJSON
			Expect(errors.As(err, &noJSON)).To(BeTrue(),
				"free text must return ErrNoJSON")
		})
	})

	Describe("UT-KA-760-003: Parse garbage JSON returns ErrNoRecognizedFields", func() {
		It("should return ErrNoRecognizedFields identifiable via errors.As", func() {
			p := parser.NewResultParser()
			content := `{"foo": "bar", "baz": 42}`
			result, err := p.Parse(content)
			Expect(err).To(HaveOccurred())
			Expect(result).To(BeNil())

			var noFields *parser.ErrNoRecognizedFields
			Expect(errors.As(err, &noFields)).To(BeTrue(),
				"garbage JSON must return ErrNoRecognizedFields")
		})
	})

	Describe("UT-KA-760-004: Typed errors are distinct (no cross-match)", func() {
		It("should not cross-match between different typed errors", func() {
			p := parser.NewResultParser()

			_, errNoJSON := p.Parse("plain text, no json here")
			Expect(errNoJSON).To(HaveOccurred())

			var noFields *parser.ErrNoRecognizedFields
			Expect(errors.As(errNoJSON, &noFields)).To(BeFalse(),
				"ErrNoJSON must NOT match ErrNoRecognizedFields")

			var emptyErr *parser.ErrEmptyContent
			Expect(errors.As(errNoJSON, &emptyErr)).To(BeFalse(),
				"ErrNoJSON must NOT match ErrEmptyContent")

			_, errEmpty := p.Parse("")
			Expect(errEmpty).To(HaveOccurred())

			var noJSON *parser.ErrNoJSON
			Expect(errors.As(errEmpty, &noJSON)).To(BeFalse(),
				"ErrEmptyContent must NOT match ErrNoJSON")
		})
	})

	Describe("UT-KA-760-005: Error() text preserves backward-compatible substrings", func() {
		It("should contain expected substrings for each error type", func() {
			p := parser.NewResultParser()

			_, errEmpty := p.Parse("")
			Expect(errEmpty.Error()).To(ContainSubstring("empty"),
				"ErrEmptyContent must contain 'empty'")

			_, errNoJSON := p.Parse("no json here at all")
			Expect(errNoJSON.Error()).To(ContainSubstring("no JSON found"),
				"ErrNoJSON must contain 'no JSON found'")

			_, errNoFields := p.Parse(`{"foo": "bar", "baz": 42}`)
			Expect(errNoFields.Error()).To(ContainSubstring("no recognized fields"),
				"ErrNoRecognizedFields must contain 'no recognized fields'")
		})
	})

	Describe("UT-KA-760-006: ErrNoJSON.Content preserves raw LLM text", func() {
		It("should capture the original input text in ErrNoJSON.Content", func() {
			p := parser.NewResultParser()
			content := "No workflow can address quota exhaustion. Manual intervention required."
			_, err := p.Parse(content)
			Expect(err).To(HaveOccurred())

			var noJSON *parser.ErrNoJSON
			Expect(errors.As(err, &noJSON)).To(BeTrue())
			Expect(noJSON.Content).To(Equal(content),
				"ErrNoJSON.Content must preserve the raw LLM text verbatim")
		})
	})
})
