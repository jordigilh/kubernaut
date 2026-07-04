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

package pii_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/pii"
)

func TestPII(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "PII Redaction Utility Suite")
}

// Test Tier: UNIT ONLY
// Rationale: Pure computational utility with zero external dependencies.
// Business Requirements Enabled: SOC2/FedRAMP privacy compliance -- data
// minimization for the DataStorage audit export handler
// (pkg/datastorage/server/audit_export_handler.go), which is the sole
// production consumer of this package.
//
// These are characterization tests written before the Wave E complexity
// remediation of RedactMapByFieldNames (nestif finding), pinning current
// behavior across the whole file since it previously had zero test coverage.

var _ = Describe("Redactor", func() {
	var r *pii.Redactor

	BeforeEach(func() {
		r = pii.NewRedactor()
	})

	Describe("RedactEmail", func() {
		It("redacts the local and domain parts, keeping only the first character of each", func() {
			Expect(r.RedactEmail("user@domain.com")).To(Equal("u***@d***.c***"))
		})

		It("returns empty string for empty input", func() {
			Expect(r.RedactEmail("")).To(Equal(""))
		})

		It("fully redacts malformed emails (no single '@')", func() {
			Expect(r.RedactEmail("not-an-email")).To(Equal("***@***.***"))
			Expect(r.RedactEmail("a@b@c")).To(Equal("***@***.***"))
		})

		It("redacts every label of a multi-part domain", func() {
			Expect(r.RedactEmail("alice@mail.example.co.uk")).To(Equal("a***@m***.e***.c***.u***"))
		})
	})

	Describe("RedactIPv4", func() {
		It("keeps the first octet and redacts the rest", func() {
			Expect(r.RedactIPv4("192.168.1.1")).To(Equal("192.***.***.***"))
		})

		It("returns empty string for empty input", func() {
			Expect(r.RedactIPv4("")).To(Equal(""))
		})

		It("fully redacts malformed IPv4 addresses", func() {
			Expect(r.RedactIPv4("not-an-ip")).To(Equal("***.***.***"))
			Expect(r.RedactIPv4("1.2.3")).To(Equal("***.***.***"))
		})
	})

	Describe("RedactPhone", func() {
		It("preserves a leading country code and redacts the rest", func() {
			Expect(r.RedactPhone("+1-555-1234")).To(Equal("+1-***-****"))
		})

		It("returns empty string for empty input", func() {
			Expect(r.RedactPhone("")).To(Equal(""))
		})

		It("fully redacts numbers with no country code", func() {
			Expect(r.RedactPhone("555-555-1234")).To(Equal("***-***-****"))
		})
	})

	Describe("RedactString", func() {
		It("redacts emails, IPv4 addresses, and phone numbers found within free text", func() {
			// Note: the phone regex requires two 3-digit groups plus a 4-digit
			// group after an optional country code (e.g. "+1-555-555-1234"),
			// so a short "555-1234"-style number is intentionally not matched.
			input := "Contact user@domain.com or call +1-555-555-1234 from 192.168.1.1"
			result := r.RedactString(input)
			Expect(result).To(ContainSubstring("u***@d***.c***"))
			Expect(result).To(ContainSubstring("192.***.***.***"))
			Expect(result).To(ContainSubstring("+1-***-****"))
		})

		It("returns empty string for empty input", func() {
			Expect(r.RedactString("")).To(Equal(""))
		})

		It("returns text unchanged when it contains no PII patterns", func() {
			Expect(r.RedactString("no sensitive data here")).To(Equal("no sensitive data here"))
		})
	})

	Describe("RedactJSON", func() {
		It("redacts PII within string values", func() {
			result := r.RedactJSON("user@domain.com")
			Expect(result).To(Equal("u***@d***.c***"))
		})

		It("recursively redacts values within nested maps", func() {
			input := map[string]interface{}{
				"email": "user@domain.com",
				"nested": map[string]interface{}{
					"ip": "192.168.1.1",
				},
			}
			result := r.RedactJSON(input).(map[string]interface{})
			Expect(result["email"]).To(Equal("u***@d***.c***"))
			nested := result["nested"].(map[string]interface{})
			Expect(nested["ip"]).To(Equal("192.***.***.***"))
		})

		It("recursively redacts elements within arrays", func() {
			input := []interface{}{"user@domain.com", "no pii here"}
			result := r.RedactJSON(input).([]interface{})
			Expect(result[0]).To(Equal("u***@d***.c***"))
			Expect(result[1]).To(Equal("no pii here"))
		})

		It("returns non-string, non-composite values unchanged", func() {
			Expect(r.RedactJSON(42)).To(Equal(42))
			Expect(r.RedactJSON(true)).To(Equal(true))
			Expect(r.RedactJSON(nil)).To(BeNil())
		})
	})

	Describe("RedactJSONBytes", func() {
		It("unmarshals, redacts, and re-marshals JSON containing PII", func() {
			input := []byte(`{"email":"user@domain.com"}`)
			result, err := r.RedactJSONBytes(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(result)).To(ContainSubstring("u***@d***.c***"))
		})

		It("returns an error for malformed JSON", func() {
			_, err := r.RedactJSONBytes([]byte(`not json`))
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("RedactMapByFieldNames", func() {
		It("redacts only the named string fields, leaving others untouched", func() {
			data := map[string]interface{}{
				"email":   "user@domain.com",
				"comment": "user@domain.com mentioned in passing",
			}
			result := r.RedactMapByFieldNames(data, []string{"email"})
			Expect(result["email"]).To(Equal("u***@d***.c***"))
			Expect(result["comment"]).To(Equal("user@domain.com mentioned in passing"))
		})

		It("applies RedactJSON (not RedactString) when a named field's value is not a string", func() {
			data := map[string]interface{}{
				"email": map[string]interface{}{"raw": "user@domain.com"},
			}
			result := r.RedactMapByFieldNames(data, []string{"email"})
			nested := result["email"].(map[string]interface{})
			Expect(nested["raw"]).To(Equal("u***@d***.c***"))
		})

		It("recurses into nested maps for non-matching keys", func() {
			data := map[string]interface{}{
				"outer": map[string]interface{}{
					"email": "user@domain.com",
				},
			}
			result := r.RedactMapByFieldNames(data, []string{"email"})
			outer := result["outer"].(map[string]interface{})
			Expect(outer["email"]).To(Equal("u***@d***.c***"))
		})

		It("recurses into array elements that are maps, leaving non-map elements as-is", func() {
			data := map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{"email": "user@domain.com"},
					"plain-string-element",
				},
			}
			result := r.RedactMapByFieldNames(data, []string{"email"})
			items := result["items"].([]interface{})
			firstItem := items[0].(map[string]interface{})
			Expect(firstItem["email"]).To(Equal("u***@d***.c***"))
			Expect(items[1]).To(Equal("plain-string-element"))
		})

		It("returns non-matching, non-composite values unchanged", func() {
			data := map[string]interface{}{"count": 42}
			result := r.RedactMapByFieldNames(data, []string{"email"})
			Expect(result["count"]).To(Equal(42))
		})

		It("returns an empty map for empty input", func() {
			result := r.RedactMapByFieldNames(map[string]interface{}{}, []string{"email"})
			Expect(result).To(BeEmpty())
		})
	})
})
