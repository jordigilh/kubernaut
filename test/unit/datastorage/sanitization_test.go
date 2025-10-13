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
	"go.uber.org/zap"

	"github.com/jordigilh/kubernaut/pkg/datastorage/validation"
)

var _ = Describe("BR-STORAGE-011: Input Sanitization", func() {
	var validator *validation.Validator

	BeforeEach(func() {
		logger, _ := zap.NewDevelopment()
		validator = validation.NewValidator(logger)
	})

	// ⭐ TABLE-DRIVEN: Sanitization test cases
	DescribeTable("should sanitize malicious input",
		func(input string, shouldContainMalicious bool, description string) {
			result := validator.SanitizeString(input)

			if shouldContainMalicious {
				Expect(result).To(ContainSubstring("<script>"), description)
			} else {
				Expect(result).ToNot(ContainSubstring("<script>"), description)
				Expect(result).ToNot(ContainSubstring("</script>"), description)
			}
		},

		// XSS patterns
		Entry("BR-STORAGE-011.1: Basic script tag",
			"<script>alert('xss')</script>",
			false, "script tags should be removed"),

		Entry("BR-STORAGE-011.2: Script with attributes",
			"<script src='evil.js'>alert(1)</script>",
			false, "script with attributes should be removed"),

		Entry("BR-STORAGE-011.3: Nested script tags",
			"<script><script>alert('xss')</script></script>",
			false, "nested scripts should be removed"),

		Entry("BR-STORAGE-011.4: iframe injection",
			"<iframe src='evil.com'></iframe>",
			false, "iframe tags should be removed"),

		Entry("BR-STORAGE-011.5: img onerror",
			"<img src=x onerror='alert(1)'>",
			false, "img onerror should be removed"),
	)

	DescribeTable("should handle SQL injection patterns",
		func(input, expectedNotToContain, description string) {
			result := validator.SanitizeString(input)
			Expect(result).ToNot(ContainSubstring(expectedNotToContain), description)
		},

		// SQL injection patterns
		Entry("BR-STORAGE-011.6: SQL comment",
			"test'; DROP TABLE users; --", ";", "semicolon should be removed"),

		Entry("BR-STORAGE-011.7: SQL UNION attack - semicolon removed",
			"test' UNION SELECT * FROM passwords", ";", "handled safely"),

		Entry("BR-STORAGE-011.8: Multiple semicolons",
			"test;;; DROP TABLE users;;;", ";", "all semicolons removed"),
	)

	DescribeTable("should preserve safe content",
		func(input, expectedOutput, description string) {
			result := validator.SanitizeString(input)
			Expect(result).To(Equal(expectedOutput), description)
		},

		// Unicode and special characters that should be preserved
		Entry("BR-STORAGE-011.9: Unicode characters preserved",
			"用户-τεστ-مستخدم", "用户-τεστ-مستخدم", "unicode should be preserved"),

		Entry("BR-STORAGE-011.10: Normal punctuation preserved (except semicolon)",
			"test@example.com, user#123", "test@example.com, user#123", "normal punctuation preserved"),

		// Edge cases
		Entry("BR-STORAGE-011.11: Empty string",
			"", "", "empty string handled"),

		Entry("BR-STORAGE-011.12: Whitespace preserved",
			"hello   world", "hello   world", "whitespace preserved"),
	)

	Context("comprehensive XSS protection", func() {
		It("should remove all HTML tags", func() {
			input := "<div><p>Hello <strong>World</strong></p></div>"
			result := validator.SanitizeString(input)

			Expect(result).ToNot(ContainSubstring("<div>"))
			Expect(result).ToNot(ContainSubstring("<p>"))
			Expect(result).ToNot(ContainSubstring("<strong>"))
			Expect(result).To(ContainSubstring("Hello"))
			Expect(result).To(ContainSubstring("World"))
		})

		It("should handle mixed content", func() {
			input := "Safe text <script>evil()</script> more safe text"
			result := validator.SanitizeString(input)

			Expect(result).To(ContainSubstring("Safe text"))
			Expect(result).To(ContainSubstring("more safe text"))
			Expect(result).ToNot(ContainSubstring("<script>"))
			Expect(result).ToNot(ContainSubstring("evil()"))
		})
	})

	Context("SQL injection protection", func() {
		It("should remove semicolons from all positions", func() {
			inputs := []string{
				";DROP TABLE users",
				"DROP TABLE users;",
				"DROP; TABLE; users;",
			}

			for _, input := range inputs {
				result := validator.SanitizeString(input)
				Expect(result).ToNot(ContainSubstring(";"), "input: %s", input)
			}
		})
	})
})
